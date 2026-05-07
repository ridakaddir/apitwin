package grpc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jhump/protoreflect/desc" //nolint:staticcheck
	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/logger"
	"github.com/ridakaddir/apitwin/internal/persist"
	"google.golang.org/grpc/codes"
)

// applyGRPCPersist handles persist operations for a matched gRPC route case.
// It mutates the stub file on disk (append / replace / delete), logs the result,
// and returns (grpcCode, handled, responseData). The caller encodes responseData
// into the proto wire format and sends it back to the client.
func (h *handler) applyGRPCPersist(
	c config.Case,
	reqMap map[string]interface{},
	start time.Time,
	fullMethod string,
	md *desc.MethodDescriptor,
) (code codes.Code, handled bool, result map[string]interface{}, persistedPath string) {
	configDir := h.loader.StubRoot()
	filePath := resolveGRPCFilePath(c.File, reqMap, configDir)

	// When source is set, extract only that sub-field from the request body
	// before any other processing. This lets callers persist a nested entity
	// (e.g. "service") without polluting the stub with wrapper fields.
	//
	// If source is unset, fall back to the proto descriptor via
	// Registry.RequestEntity, which handles two Google API shapes:
	//   1. Direct-entity response — response type == entity type. The
	//      request field whose message type matches the response is picked.
	//   2. Response-wrapped — response has a single non-repeated message
	//      field (e.g. UpdateGatewayResponse { Gateway gateway = 1; }).
	//      The request field whose type matches that wrapper field's type
	//      is picked, and the wrapper field's JSON name is returned as
	//      the inferred wrap.
	//
	// Without this, the request envelope (e.g. {parent, cityName, city})
	// would be shallow-merged into a flat entity stub on `merge="update"`,
	// which would corrupt the file with both flat fields and a duplicated
	// nested wrapper, and break proto round-trip on encode.
	//
	// When the input has multiple matching fields the auto-derive bails
	// (ambiguous=true) and we log a deduplicated warn so the user has a
	// breadcrumb to set `source` explicitly. Explicit `c.Source` always
	// wins and short-circuits auto-derive entirely.
	sourceField := c.Source
	wrapField := c.Wrap
	if sourceField == "" {
		derivedSrc, derivedWrap, ambiguous := h.registry.RequestEntity(md)
		if ambiguous {
			h.warnAutoDeriveAmbiguousOnce(fullMethod)
		}
		sourceField = derivedSrc
		if wrapField == "" {
			wrapField = derivedWrap
		}
	} else if wrapField == "" {
		// Explicit source, implicit wrap: source extraction and response
		// wrapping are independent concerns, so setting source should not
		// opt the user out of wrap inference. Without this, a deep source
		// like "terrain.summit" against a response-wrapped RPC (e.g.
		// UpdateSummitResponse { Summit summit = 1; }) writes the extracted
		// entity directly into the outer response message and fails proto
		// round-trip on encode ("no known field named <entity-field>").
		wrapField = h.registry.ResponseWrap(md)
		if wrapField == "" {
			// ResponseWrap returns "" for multi-message responses (e.g.
			// LRO shapes with sibling status/audit/resource fields). Use
			// the explicit source to pick the right wrapper.
			wrapField = h.registry.ResponseWrapForSource(md, sourceField)
		}
	}
	srcMap := reqMap
	if sourceField != "" {
		if sub := persist.ExtractSourceField(reqMap, sourceField); sub != nil {
			srcMap = sub
		} else if c.Source != "" {
			// Only warn when the user explicitly configured a source that
			// failed to resolve. Auto-derived misses are silent because
			// they would fire on every persist call by design — c.Source is
			// the only user-facing signal that a miss is unexpected.
			logger.Warn("grpc persist source: field not found", "source", c.Source)
		}
	}

	// Strip fields used as {body.*} path placeholders so they don't leak
	// into the persisted stub (they are routing fields, not data fields).
	// persistData is what gets stored; reqMap (unfiltered) is still passed to
	// loadGRPCDefaults for template lookups since defaults may reference routing fields.
	persistData := stripPathPlaceholderFields(c.File, srcMap)

	// When wrap is set (explicitly or via auto-derive), filter persistData
	// to only fields valid in the entity message. This prevents routing
	// fields from the request (e.g. orgId, providerId) from leaking into
	// the persisted stub and causing proto encoding failures on the
	// response. Pre-allocated here so the merge=update safeguard below
	// can reuse the result without rebuilding the map.
	var allowedEntityFields map[string]bool
	if wrapField != "" && md != nil {
		allowedEntityFields = h.registry.EntityFieldNames(md, wrapField)
		if allowedEntityFields != nil {
			persistData = filterToEntityFields(persistData, allowedEntityFields)
		}
	}

	switch strings.ToLower(c.Merge) {

	case "update":
		// Apply defaults if specified (enrich incoming data before persisting).
		persistData = loadGRPCDefaults(c.Defaults, persistData, reqMap, configDir)

		// Defense-in-depth: refuse to write a payload whose top-level keys
		// are not valid for the response entity schema. This runs AFTER
		// loadGRPCDefaults so a defaults file with stray non-entity keys
		// cannot slip past. Scoped to merge=update because that is the
		// shape from the bug report; merge=append already round-trips its
		// payload through filterToEntityFields above and tightening it
		// would break existing append users with metadata sidecars.
		//
		// When wrap is set: reuse the entity-field set from above. When
		// wrap is empty (Shape 1 direct-entity or auto-derive blind):
		// fall back to the response output type's own field set.
		if md != nil {
			allowed := allowedEntityFields
			if allowed == nil && wrapField == "" {
				allowed = entityFieldsFromType(md.GetOutputType())
			}
			if allowed != nil {
				if extras := keysNotIn(persistData, allowed); len(extras) > 0 {
					logger.Error("grpc persist refused: payload has fields not present in entity schema",
						"method", fullMethod, "file", filePath, "extras", extras,
						"wrap", wrapField, "source", sourceField)
					logger.LogGRPC(fullMethod, codes.Internal, time.Since(start), logger.SourceStub)
					return codes.Internal, true, nil, ""
				}
			}
		}

		updated, err := persist.Update(filePath, persistData)
		if err != nil {
			if persist.IsNotFound(err) {
				// LogGRPC deferred to caller — handler may retry with
				// the fallback case on a transition 404.
				return codes.NotFound, true, nil, ""
			}
			if persist.IsConfigError(err) {
				logger.Error("grpc persist update config error", "file", filePath, "err", err)
				logger.LogGRPC(fullMethod, codes.InvalidArgument, time.Since(start), logger.SourceStub)
				return codes.InvalidArgument, true, nil, ""
			}
			logger.Error("grpc persist update", "file", filePath, "err", err)
			logger.LogGRPC(fullMethod, codes.Internal, time.Since(start), logger.SourceStub)
			return codes.Internal, true, nil, ""
		}
		logger.LogGRPC(fullMethod, codes.OK, time.Since(start), logger.SourceStub)
		if wrapField != "" {
			return codes.OK, true, map[string]interface{}{wrapField: updated}, filePath
		}
		return codes.OK, true, updated, filePath

	case "append":
		if !persist.IsDirectoryPath(filePath, c.File) {
			logger.Error("grpc persist append", "file", filePath, "err", "append requires directory path")
			logger.LogGRPC(fullMethod, codes.InvalidArgument, time.Since(start), logger.SourceStub)
			return codes.InvalidArgument, true, nil, ""
		}
		// Apply defaults if specified (enrich incoming data before persisting).
		persistData = loadGRPCDefaults(c.Defaults, persistData, reqMap, configDir)
		createdPath, appended, err := persist.AppendToDir(filePath, c.Key, persistData)
		if err != nil {
			logger.Error("grpc persist append to dir", "dir", filePath, "err", err)
			logger.LogGRPC(fullMethod, codes.Internal, time.Since(start), logger.SourceStub)
			return codes.Internal, true, nil, ""
		}
		logger.LogGRPC(fullMethod, codes.OK, time.Since(start), logger.SourceStub)
		if wrapField != "" {
			return codes.OK, true, map[string]interface{}{wrapField: appended}, createdPath
		}
		return codes.OK, true, appended, createdPath

	case "delete":
		if err := persist.DeleteFile(filePath); err != nil {
			if persist.IsNotFound(err) {
				logger.LogGRPC(fullMethod, codes.NotFound, time.Since(start), logger.SourceStub)
				return codes.NotFound, true, nil, ""
			}
			logger.Error("grpc persist delete file", "file", filePath, "err", err)
			logger.LogGRPC(fullMethod, codes.Internal, time.Since(start), logger.SourceStub)
			return codes.Internal, true, nil, ""
		}
		logger.LogGRPC(fullMethod, codes.OK, time.Since(start), logger.SourceStub)
		return codes.OK, true, nil, ""

	default:
		logger.Warn("grpc persist: unknown merge strategy", "merge", c.Merge)
		logger.LogGRPC(fullMethod, codes.Internal, time.Since(start), logger.SourceStub)
		return codes.Internal, true, nil, ""
	}
}

// entityFieldsFromType returns the JSON+proto field-name set of mt, used by
// the persist runtime safeguard when no `wrap` is in play (Shape 1
// direct-entity responses, or auto-derive blind). Returns nil when mt is nil
// or has zero fields.
func entityFieldsFromType(mt *desc.MessageDescriptor) map[string]bool {
	if mt == nil {
		return nil
	}
	fields := mt.GetFields()
	if len(fields) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(fields)*2)
	for _, f := range fields {
		allowed[f.GetJSONName()] = true
		allowed[f.GetName()] = true
	}
	return allowed
}

// keysNotIn returns the keys of data that are NOT in allowed. Used by the
// persist runtime safeguard to identify entity-schema violations before the
// file is written. Returns nil when no extras exist.
func keysNotIn(data map[string]interface{}, allowed map[string]bool) []string {
	var extras []string
	for k := range data {
		if !allowed[k] {
			extras = append(extras, k)
		}
	}
	return extras
}

// filterToEntityFields returns a shallow copy of data containing only keys
// present in the allowed set. Returns data unchanged if allowed is nil.
func filterToEntityFields(data map[string]interface{}, allowed map[string]bool) map[string]interface{} {
	if allowed == nil {
		return data
	}
	filtered := make(map[string]interface{}, len(data))
	for k, v := range data {
		if allowed[k] {
			filtered[k] = v
		}
	}
	return filtered
}

// stripPathPlaceholderFields returns a shallow copy of reqMap with fields used as
// {body.*} placeholders in the file path removed. This prevents routing fields
// (e.g. country_code from {body.country_code}) from leaking into persisted stubs.
func stripPathPlaceholderFields(filePattern string, reqMap map[string]interface{}) map[string]interface{} {
	matches := grpcPlaceholderRe.FindAllStringSubmatch(filePattern, -1)
	if len(matches) == 0 {
		return reqMap
	}

	// Collect placeholder field names and their camelCase variants.
	exclude := make(map[string]bool, len(matches)*2)
	for _, m := range matches {
		field := m[1]
		exclude[field] = true
		exclude[persist.SnakeToCamel(field)] = true
	}

	filtered := make(map[string]interface{}, len(reqMap))
	for k, v := range reqMap {
		if !exclude[k] {
			filtered[k] = v
		}
	}
	return filtered
}

// loadGRPCDefaults reads a defaults JSON file, resolves template tokens ({{uuid}},
// {{now}}, {{timestamp}}), and deep-merges the result under the incoming data
// so that incoming (request body) fields win on conflicts.
//
// Returns incoming unchanged if defaults is empty or on any error (warnings logged).
func loadGRPCDefaults(defaults string, incoming map[string]interface{},
	reqMap map[string]interface{}, configDir string) map[string]interface{} {

	if defaults == "" {
		return incoming
	}

	defaultsPath := resolveGRPCFilePath(defaults, reqMap, configDir)

	// Ensure resolved path stays within configDir to prevent directory traversal.
	if configDir != "" {
		cleaned := filepath.Clean(defaultsPath)
		absConfig, _ := filepath.Abs(configDir)
		absDefaults, _ := filepath.Abs(cleaned)
		if !strings.HasPrefix(absDefaults, absConfig+string(filepath.Separator)) && absDefaults != absConfig {
			logger.Warn("grpc persist defaults: path escapes config directory", "file", defaultsPath, "configDir", configDir)
			return incoming
		}
	}

	defaultsData, err := os.ReadFile(defaultsPath)
	if err != nil {
		logger.Warn("grpc persist defaults: cannot read file", "file", defaultsPath, "err", err)
		return incoming
	}

	resolved, err := renderGRPCTemplate(string(defaultsData))
	if err != nil {
		logger.Warn("grpc persist defaults: template error", "file", defaultsPath, "err", err)
		return incoming
	}

	var base map[string]interface{}
	if err := json.Unmarshal([]byte(resolved), &base); err != nil {
		logger.Warn("grpc persist defaults: invalid JSON", "file", defaultsPath, "err", err)
		return incoming
	}

	return persist.DeepMerge(base, incoming)
}

// resolveGRPCFilePath resolves {body.field} placeholders in the file path and
// makes it absolute relative to configDir.  Dot-notation is supported for
// nested fields, e.g. {body.service.name} walks reqMap["service"]["name"].
var grpcPlaceholderRe = regexp.MustCompile(`\{body\.([^}]+)\}`)

// grpcPathSanitizeRe matches characters unsafe for file-path segments.
var grpcPathSanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_\-.]`)

func resolveGRPCFilePath(filePath string, reqMap map[string]interface{}, configDir string) string {
	if grpcPlaceholderRe.MatchString(filePath) {
		filePath = grpcPlaceholderRe.ReplaceAllStringFunc(filePath, func(match string) string {
			sub := grpcPlaceholderRe.FindStringSubmatch(match)
			if len(sub) != 2 {
				return match
			}
			field := sub[1]
			if val, ok := walkNestedField(reqMap, field); ok {
				return grpcPathSanitizeRe.ReplaceAllString(val, "_")
			}
			return match
		})
	}
	if configDir != "" && !filepath.IsAbs(filePath) {
		filePath = filepath.Join(configDir, filePath)
	}
	return filePath
}


// walkNestedField walks a dot-notation path through a map[string]interface{},
// trying each segment as-is first then as camelCase.  Returns the string
// representation of the leaf value.
func walkNestedField(reqMap map[string]interface{}, dotPath string) (string, bool) {
	parts := strings.Split(dotPath, ".")
	var current interface{} = reqMap
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return "", false
		}
		v, exists := m[part]
		if !exists {
			v, exists = m[persist.SnakeToCamel(part)]
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
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", v), "0"), "."), true
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
