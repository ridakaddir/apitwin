package generate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jhump/protoreflect/desc"            //nolint:staticcheck
	"github.com/jhump/protoreflect/desc/protoparse" //nolint:staticcheck
)

// ProtoOptions configures the proto → config generator.
type ProtoOptions struct {
	ProtoFiles  []string // .proto files to parse
	ImportPaths []string // extra import search paths
	OutDir      string   // output directory (default: "mocks")
	Format      string   // "toml" | "yaml" | "json"
}

// ProtoResult summarises what was generated.
type ProtoResult struct {
	Methods     int
	ConfigFiles []string
	StubFiles   []string
}

// RunProto parses .proto files and generates [[grpc_routes]] config + stub files.
func RunProto(opts ProtoOptions) (*ProtoResult, error) {
	if opts.Format == "" {
		opts.Format = "toml"
	}
	if opts.Format == "yml" {
		opts.Format = "yaml"
	}
	switch opts.Format {
	case "toml", "yaml", "json":
	default:
		return nil, fmt.Errorf("unsupported format %q — use toml, yaml, or json", opts.Format)
	}
	if opts.OutDir == "" {
		opts.OutDir = "mocks"
	}

	// Parse proto files.
	p := protoparse.Parser{
		ImportPaths:      opts.ImportPaths,
		InferImportPaths: true,
	}
	fds, err := p.ParseFiles(opts.ProtoFiles...)
	if err != nil {
		return nil, fmt.Errorf("parsing proto files: %w", err)
	}

	// Collect all methods across all services.
	type methodInfo struct {
		fullMethod  string // "/package.Service/Method"
		serviceName string // "Service"
		methodName  string // "Method"
		outputDesc  *desc.MessageDescriptor
	}

	var methods []methodInfo
	for _, fd := range fds {
		for _, svc := range fd.GetServices() {
			for _, m := range svc.GetMethods() {
				methods = append(methods, methodInfo{
					fullMethod:  fmt.Sprintf("/%s/%s", svc.GetFullyQualifiedName(), m.GetName()),
					serviceName: svc.GetName(),
					methodName:  m.GetName(),
					outputDesc:  m.GetOutputType(),
				})
			}
		}
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no RPC methods found in the provided .proto files")
	}

	// Create output directories.
	stubsDir := filepath.Join(opts.OutDir, "stubs")
	if err := os.MkdirAll(stubsDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	result := &ProtoResult{}

	// Write stub files and collect route data.
	var routes []routeData

	for _, m := range methods {
		stub := synthesiseProtoJSON(m.outputDesc)

		stubFile := protoStubFilename(m.serviceName, m.methodName)
		stubPath := filepath.Join(stubsDir, stubFile)
		if err := os.WriteFile(stubPath, stub, 0644); err != nil {
			return nil, fmt.Errorf("writing stub %s: %w", stubPath, err)
		}
		result.StubFiles = append(result.StubFiles, stubPath)
		routes = append(routes, routeData{
			fullMethod: m.fullMethod,
			stubFile:   filepath.Join("stubs", stubFile),
		})
	}

	result.Methods = len(methods)

	// Write config file.
	var cfgPath string
	switch opts.Format {
	case "yaml":
		content := buildGRPCRoutesYAML(routes, opts.ProtoFiles)
		cfgPath = filepath.Join(opts.OutDir, "apitwin.yaml")
		if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
			return nil, fmt.Errorf("writing config: %w", err)
		}
	case "json":
		content, err := buildGRPCRoutesJSON(routes)
		if err != nil {
			return nil, err
		}
		cfgPath = filepath.Join(opts.OutDir, "apitwin.json")
		if err := os.WriteFile(cfgPath, content, 0644); err != nil {
			return nil, fmt.Errorf("writing config: %w", err)
		}
	default: // toml
		content := buildGRPCRoutesTOML(routes, opts.ProtoFiles)
		cfgPath = filepath.Join(opts.OutDir, "apitwin.toml")
		if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
			return nil, fmt.Errorf("writing config: %w", err)
		}
	}
	result.ConfigFiles = append(result.ConfigFiles, cfgPath)

	return result, nil
}

// ─── Stub synthesis ───────────────────────────────────────────────────────────

// synthesiseProtoJSON builds a synthetic JSON response from a proto message descriptor.
// Walks fields recursively (max depth 5) and picks sensible placeholder values.
func synthesiseProtoJSON(md *desc.MessageDescriptor) []byte {
	val := synthProtoMessage(md, 5)
	b, err := json.MarshalIndent(val, "", "  ")
	if err != nil {
		return []byte("{}\n")
	}
	return append(b, '\n')
}

func synthProtoMessage(md *desc.MessageDescriptor, depth int) map[string]interface{} {
	out := make(map[string]interface{})
	if md == nil || depth <= 0 {
		return out
	}
	for _, fd := range md.GetFields() {
		out[fd.GetJSONName()] = synthProtoField(fd, depth-1)
	}
	return out
}

func synthProtoField(fd *desc.FieldDescriptor, depth int) interface{} {
	// Repeated fields → single-element array.
	if fd.IsRepeated() && !fd.IsMap() {
		elem := synthProtoScalarOrMessage(fd, depth)
		return []interface{}{elem}
	}
	// Map fields → object with one example entry.
	if fd.IsMap() {
		return map[string]interface{}{"key": synthProtoField(fd.GetMapValueType(), depth)}
	}
	return synthProtoScalarOrMessage(fd, depth)
}

func synthProtoScalarOrMessage(fd *desc.FieldDescriptor, depth int) interface{} {
	// Message type → recurse.
	if fd.GetMessageType() != nil {
		if depth <= 0 {
			return map[string]interface{}{}
		}
		return synthProtoMessage(fd.GetMessageType(), depth-1)
	}

	// Scalar types.
	switch fd.GetType().String() {
	case "TYPE_STRING":
		// Use field name as a hint for common formats.
		name := strings.ToLower(fd.GetName())
		switch {
		case strings.Contains(name, "id"):
			return "{{uuid}}"
		case strings.Contains(name, "email"):
			return "user@example.com"
		case strings.Contains(name, "url") || strings.Contains(name, "uri"):
			return "https://example.com"
		case strings.Contains(name, "time") || strings.Contains(name, "at") || strings.Contains(name, "date"):
			return "{{now}}"
		case strings.Contains(name, "name"):
			return "Example Name"
		}
		return "string"
	case "TYPE_BOOL":
		return true
	case "TYPE_INT32", "TYPE_SINT32", "TYPE_SFIXED32",
		"TYPE_INT64", "TYPE_SINT64", "TYPE_SFIXED64",
		"TYPE_UINT32", "TYPE_FIXED32",
		"TYPE_UINT64", "TYPE_FIXED64":
		return 1
	case "TYPE_FLOAT", "TYPE_DOUBLE":
		return 1.0
	case "TYPE_BYTES":
		return "c3RyaW5n" // base64("string")
	case "TYPE_ENUM":
		et := fd.GetEnumType()
		if et != nil && len(et.GetValues()) > 0 {
			return et.GetValues()[0].GetName()
		}
		return 0
	}
	return nil
}

// ─── Config file renderers ────────────────────────────────────────────────────

type routeData struct {
	fullMethod string
	stubFile   string
}

func buildGRPCRoutesTOML(routes []routeData, protoFiles []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Generated from: %s\n\n", strings.Join(protoFiles, ", "))

	for _, r := range routes {
		b.WriteString("[[grpc_routes]]\n")
		fmt.Fprintf(&b, "match    = %q\n", r.fullMethod)
		b.WriteString("enabled  = true\n")
		b.WriteString("fallback = \"ok\"\n\n")
		b.WriteString("  [grpc_routes.cases.ok]\n")
		b.WriteString("  status = 0\n") // gRPC OK
		fmt.Fprintf(&b, "  file   = %q\n", r.stubFile)
		b.WriteString("\n")
	}

	return b.String()
}

func buildGRPCRoutesYAML(routes []routeData, protoFiles []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Generated from: %s\n", strings.Join(protoFiles, ", "))
	b.WriteString("grpc_routes:\n")

	for _, r := range routes {
		fmt.Fprintf(&b, "  - match: %q\n", r.fullMethod)
		b.WriteString("    enabled: true\n")
		b.WriteString("    fallback: ok\n")
		b.WriteString("    cases:\n")
		b.WriteString("      ok:\n")
		b.WriteString("        status: 0\n")
		fmt.Fprintf(&b, "        file: %q\n", r.stubFile)
		b.WriteString("\n")
	}

	return b.String()
}

func buildGRPCRoutesJSON(routes []routeData) ([]byte, error) {
	type jsonCase struct {
		Status int    `json:"status"`
		File   string `json:"file"`
	}
	type jsonRoute struct {
		Match    string              `json:"match"`
		Enabled  bool                `json:"enabled"`
		Fallback string              `json:"fallback"`
		Cases    map[string]jsonCase `json:"cases"`
	}
	type jsonConfig struct {
		GRPCRoutes []jsonRoute `json:"grpc_routes"`
	}

	var grpcRoutes []jsonRoute
	for _, r := range routes {
		grpcRoutes = append(grpcRoutes, jsonRoute{
			Match:    r.fullMethod,
			Enabled:  true,
			Fallback: "ok",
			Cases: map[string]jsonCase{
				"ok": {Status: 0, File: r.stubFile},
			},
		})
	}

	out, err := json.MarshalIndent(jsonConfig{GRPCRoutes: grpcRoutes}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding json config: %w", err)
	}
	return append(out, '\n'), nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// protoStubFilename builds: UserService_GetUser.json
func protoStubFilename(service, method string) string {
	return service + "_" + method + ".json"
}
