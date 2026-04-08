package grpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jhump/protoreflect/desc" //nolint:staticcheck
	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/logger"
	"github.com/ridakaddir/apitwin/internal/persist"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// configLoader abstracts config.Loader for gRPC — mirrors the interface in proxy/handler.go.
type configLoader interface {
	Get() *config.Config
	ConfigDir() string
}

// handler dispatches incoming gRPC calls to mock cases or proxies them upstream.
type handler struct {
	loader      configLoader
	registry    *Registry
	transitions *grpcTransitionState
	target      string // upstream gRPC address, empty if mock-only
}

func newHandler(loader configLoader, registry *Registry, target string) *handler {
	return &handler{
		loader:      loader,
		registry:    registry,
		transitions: newGRPCTransitionState(),
		target:      target,
	}
}

// resetTransitions clears all transition timers (called on config hot-reload).
func (h *handler) resetTransitions() {
	h.transitions.Reset()
}

// serve is the unknown-service handler registered with grpc.Server.
// It intercepts every unrecognised method and routes it through the mock pipeline.
func (h *handler) serve(srv interface{}, stream grpc.ServerStream) error {
	start := time.Now()

	// Extract the full method path from the stream context.
	fullMethod, ok := grpc.MethodFromServerStream(stream)
	if !ok {
		return status.Error(codes.Internal, "could not determine gRPC method")
	}

	cfg := h.loader.Get()

	// Receive the raw request bytes from the client.
	var rawReq []byte
	if err := stream.RecvMsg(&rawReq); err != nil {
		return status.Errorf(codes.Internal, "recv: %v", err)
	}

	// Find the method descriptor so we can decode the request body.
	md, err := h.registry.FindMethod(fullMethod)
	if err != nil {
		return status.Errorf(codes.Internal, "registry lookup: %v", err)
	}

	// Guard against streaming RPCs — only unary is supported.
	if md != nil && (md.IsClientStreaming() || md.IsServerStreaming()) {
		return status.Errorf(codes.Unimplemented,
			"apitwin does not yet support streaming RPCs (%s)", fullMethod)
	}

	// Decode request to a map for condition evaluation.
	var reqMap map[string]interface{}
	if md != nil && len(rawReq) > 0 {
		var decodeErr error
		reqMap, decodeErr = h.registry.DecodeRequest(md, rawReq)
		if decodeErr != nil {
			logger.Warn("grpc decode request failed — conditions will use empty body",
				"err", decodeErr, "method", fullMethod)
		}
	}

	// Match a gRPC route.
	for i := range cfg.GRPCRoutes {
		route := &cfg.GRPCRoutes[i]

		if !route.IsEnabled() {
			continue
		}
		if !grpcMatchPath(route.Match, fullMethod) {
			continue
		}

		// Require a valid method descriptor to transcode request/response.
		// Without one we cannot decode conditions or encode the stub response.
		if md == nil {
			logger.Warn("grpc method not in proto registry — cannot transcode",
				"method", fullMethod)
			logger.LogGRPC(fullMethod, codes.Unimplemented, time.Since(start), logger.SourceStub)
			return status.Errorf(codes.Unimplemented,
				"method %s matched a [[grpc_routes]] entry but is not present in any loaded --grpc-proto file",
				fullMethod)
		}

		caseName := h.resolveCase(route, reqMap)
		if caseName == "" {
			break
		}

		c, ok := route.Cases[caseName]
		if !ok {
			logger.Warn("grpc case not found", "case", caseName, "method", fullMethod)
			break
		}

		// Persist (stateful mutations) — handled before delay/stub-load.
		if c.Persist {
			grpcCode, ok, persistResult := h.applyGRPCPersist(c, reqMap, start, fullMethod)
			if ok {
				// Encode the persist result into proto wire format and send it
				// back so the client receives the updated/created data.
				var wireResp []byte
				if persistResult != nil {
					jsonBody, err := json.Marshal(persistResult)
					if err != nil {
						return status.Errorf(codes.Internal, "marshal persist response: %v", err)
					}
					wireResp, err = h.registry.EncodeResponse(md, jsonBody)
					if err != nil {
						return status.Errorf(codes.Internal, "encode persist response: %v", err)
					}
				}
				if err := stream.SendMsg(&wireResp); err != nil {
					return status.Errorf(codes.Internal, "send persist response: %v", err)
				}
				if grpcCode != codes.OK {
					return status.Error(grpcCode, caseName)
				}
				return nil
			}
		}

		if c.Delay > 0 {
			time.Sleep(time.Duration(c.Delay) * time.Second)
		}

		// Load stub JSON.
		jsonBody, err := h.loadStub(c, reqMap, md)
		if err != nil {
			logger.Error("grpc load stub", "err", err, "method", fullMethod)
			logger.LogGRPC(fullMethod, codes.Internal, time.Since(start), logger.SourceStub)
			return status.Error(codes.Internal, "internal error loading stub response")
		}

		// Encode to proto wire bytes.
		var wireResp []byte
		if len(jsonBody) > 0 && string(jsonBody) != "{}" {
			wireResp, err = h.registry.EncodeResponse(md, jsonBody)
			if err != nil {
				logger.Error("grpc encode response", "err", err, "method", fullMethod)
				logger.LogGRPC(fullMethod, codes.Internal, time.Since(start), logger.SourceStub)
				return status.Error(codes.Internal, "internal error encoding stub response")
			}
		}

		// Send the response.
		if err := stream.SendMsg(&wireResp); err != nil {
			return status.Errorf(codes.Internal, "send: %v", err)
		}

		grpcCode := codes.Code(c.Status)
		logger.LogGRPC(fullMethod, grpcCode, time.Since(start), logger.SourceStub)

		if grpcCode != codes.OK {
			return status.Error(grpcCode, caseName)
		}
		return nil
	}

	// No mock matched — forward to upstream if configured.
	if h.target != "" {
		fwdErr := forwardGRPC(stream, h.target, fullMethod, rawReq)
		proxyCode := codes.OK
		if fwdErr != nil {
			if st, ok := status.FromError(fwdErr); ok {
				proxyCode = st.Code()
			} else {
				proxyCode = codes.Internal
			}
		}
		logger.LogGRPC(fullMethod, proxyCode, time.Since(start), logger.SourceProxy)
		return fwdErr
	}

	logger.LogGRPC(fullMethod, codes.Unimplemented, time.Since(start), logger.SourceStub)
	return status.Errorf(codes.Unimplemented, "no mock route matched %s and no --grpc-target configured", fullMethod)
}

// resolveCase determines which case to serve for a matched gRPC route.
// Priority: conditions → transitions → fallback (mirrors REST handler logic).
func (h *handler) resolveCase(route *config.GRPCRoute, reqMap map[string]interface{}) string {
	// 1. Conditions.
	reqBody := mapToJSONBytes(reqMap)
	for _, cond := range route.Conditions {
		if evalGRPCCondition(cond, reqBody) {
			return cond.Case
		}
	}

	// 2. Transitions.
	if len(route.Transitions) > 0 {
		if caseName := h.transitions.resolve(route); caseName != "" {
			return caseName
		}
	}

	// 3. Fallback.
	return route.Fallback
}

// loadStub reads stub content from Case.File or Case.JSON and runs template rendering.
// md is the method descriptor for the matched gRPC method (used for auto-wrapping directory responses).
func (h *handler) loadStub(c config.Case, reqMap map[string]interface{}, md *desc.MethodDescriptor) ([]byte, error) {
	configDir := h.loader.ConfigDir()

	switch {
	case c.File != "":
		filePath := resolveGRPCFilePath(c.File, reqMap, configDir)
		wantsDir := strings.HasSuffix(filePath, "/")

		// Check if this is a directory path (for aggregation)
		info, statErr := os.Stat(filePath)
		if (statErr == nil && info.IsDir()) || (wantsDir && os.IsNotExist(statErr)) {
			b, err := persist.ReadDir(filePath)
			if err != nil {
				return nil, fmt.Errorf("reading stub directory %q: %w", filePath, err)
			}
			// Wrap array into a response object field for gRPC compatibility.
			wrapKey := c.Wrap
			if wrapKey == "" && md != nil {
				wrapKey = h.registry.FindRepeatedField(md)
			}
			if wrapKey != "" {
				b = persist.WrapJSON(wrapKey, b)
			}
			return b, nil
		}

		// Regular file read
		b, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("reading stub file %q: %w", filePath, err)
		}
		if c.Wrap != "" {
			b = persist.WrapJSON(c.Wrap, b)
		}
		return b, nil

	case c.JSON != "":
		rendered, err := renderGRPCTemplate(c.JSON)
		if err != nil {
			return nil, fmt.Errorf("rendering json template: %w", err)
		}
		return []byte(rendered), nil

	default:
		return []byte("{}"), nil
	}
}

// grpcMatchPath applies the same matching logic as proxy.matchPath:
// exact match, wildcard (*), or regex (~prefix).
func grpcMatchPath(pattern, method string) bool {
	if strings.HasPrefix(pattern, "~") {
		re, err := regexp.Compile(pattern[1:])
		if err != nil {
			return false
		}
		return re.MatchString(method)
	}
	if strings.Contains(pattern, "*") {
		return grpcMatchWildcard(pattern, method)
	}
	return pattern == method
}

func grpcMatchWildcard(pattern, path string) bool {
	parts := strings.Split(pattern, "*")
	remaining := path
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(remaining, part)
		if idx == -1 {
			return false
		}
		if i == 0 && idx != 0 {
			return false
		}
		remaining = remaining[idx+len(part):]
	}
	if !strings.HasSuffix(pattern, "*") && remaining != "" {
		return false
	}
	return true
}

// evalGRPCCondition evaluates a condition against a JSON-encoded gRPC request body.
// Reuses the same op semantics as REST conditions; "query" and "header" sources are
// no-ops for gRPC (body is the only source).
func evalGRPCCondition(cond config.Condition, bodyBytes []byte) bool {
	if strings.ToLower(cond.Source) != "body" {
		return false
	}
	val, found := extractGRPCBodyField(cond.Field, bodyBytes)

	switch cond.Op {
	case "exists":
		return found
	case "not_exists":
		return !found
	case "eq":
		return found && val == cond.Value
	case "neq":
		return found && val != cond.Value
	case "contains":
		return found && strings.Contains(val, cond.Value)
	case "regex":
		if !found {
			return false
		}
		re, err := regexp.Compile(cond.Value)
		if err != nil {
			return false
		}
		return re.MatchString(val)
	}
	return false
}

// extractGRPCBodyField walks dot-notation path in the protojson-decoded map.
// Field names are tried as-is first (camelCase protojson convention), then
// converted from snake_case, so both "paymentType" and "payment_type" work.
func extractGRPCBodyField(dotPath string, body []byte) (string, bool) {
	if len(body) == 0 {
		return "", false
	}
	var data map[string]interface{}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&data); err != nil {
		return "", false
	}
	parts := strings.Split(dotPath, ".")
	var current interface{} = data
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return "", false
		}
		// Try the field name as-is first, then camelCase conversion.
		v, exists := m[part]
		if !exists {
			v, exists = m[snakeToCamel(part)]
			if !exists {
				return "", false
			}
		}
		current = v
	}
	switch v := current.(type) {
	case string:
		return v, true
	case float64:
		s := strings.TrimRight(strings.TrimRight(
			strings.TrimRight(fmt.Sprintf("%f", v), "0"), "."), "")
		return s, true
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	case nil:
		return "", false
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", false
		}
		return string(b), true
	}
}

// mapToJSONBytes serialises a map to JSON bytes for condition evaluation.
func mapToJSONBytes(m map[string]interface{}) []byte {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	return b
}

// snakeToCamel converts a snake_case field name to lowerCamelCase.
// e.g. "payment_type" → "paymentType", "user_id" → "userId".
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	if len(parts) == 1 {
		return s
	}
	var b strings.Builder
	for i, p := range parts {
		if i == 0 {
			b.WriteString(p)
			continue
		}
		if len(p) > 0 {
			b.WriteByte(p[0] - 32) // uppercase first letter
			b.WriteString(p[1:])
		}
	}
	return b.String()
}
