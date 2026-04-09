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
) (code codes.Code, handled bool, result map[string]interface{}) {
	configDir := h.loader.ConfigDir()
	filePath := resolveGRPCFilePath(c.File, reqMap, configDir)

	// Strip fields used as {body.*} path placeholders so they don't leak
	// into the persisted stub (they are routing fields, not data fields).
	// persistData is what gets stored; reqMap (unfiltered) is still passed to
	// loadGRPCDefaults for template lookups since defaults may reference routing fields.
	persistData := stripPathPlaceholderFields(c.File, reqMap)

	// When wrap is set, filter persistData to only fields valid in the entity
	// message. This prevents routing fields from the request (e.g. orgId,
	// providerId) from leaking into the persisted stub and causing proto
	// encoding failures on the response.
	if c.Wrap != "" && md != nil {
		if allowed := h.registry.EntityFieldNames(md, c.Wrap); allowed != nil {
			persistData = filterToEntityFields(persistData, allowed)
		}
	}

	switch strings.ToLower(c.Merge) {

	case "update":
		// Apply defaults if specified (enrich incoming data before persisting).
		persistData = loadGRPCDefaults(c.Defaults, persistData, reqMap, configDir)
		updated, err := persist.Update(filePath, persistData)
		if err != nil {
			if persist.IsNotFound(err) {
				logger.LogGRPC(fullMethod, codes.NotFound, time.Since(start), logger.SourceStub)
				return codes.NotFound, true, nil
			}
			if persist.IsConfigError(err) {
				logger.Error("grpc persist update config error", "file", filePath, "err", err)
				logger.LogGRPC(fullMethod, codes.InvalidArgument, time.Since(start), logger.SourceStub)
				return codes.InvalidArgument, true, nil
			}
			logger.Error("grpc persist update", "file", filePath, "err", err)
			logger.LogGRPC(fullMethod, codes.Internal, time.Since(start), logger.SourceStub)
			return codes.Internal, true, nil
		}
		logger.LogGRPC(fullMethod, codes.OK, time.Since(start), logger.SourceStub)
		if c.Wrap != "" {
			return codes.OK, true, map[string]interface{}{c.Wrap: updated}
		}
		return codes.OK, true, updated

	case "append":
		if !isGRPCDirectoryPath(filePath, c.File) {
			logger.Error("grpc persist append", "file", filePath, "err", "append requires directory path")
			logger.LogGRPC(fullMethod, codes.InvalidArgument, time.Since(start), logger.SourceStub)
			return codes.InvalidArgument, true, nil
		}
		// Apply defaults if specified (enrich incoming data before persisting).
		persistData = loadGRPCDefaults(c.Defaults, persistData, reqMap, configDir)
		_, appended, err := persist.AppendToDir(filePath, c.Key, persistData)
		if err != nil {
			logger.Error("grpc persist append to dir", "dir", filePath, "err", err)
			logger.LogGRPC(fullMethod, codes.Internal, time.Since(start), logger.SourceStub)
			return codes.Internal, true, nil
		}
		logger.LogGRPC(fullMethod, codes.OK, time.Since(start), logger.SourceStub)
		if c.Wrap != "" {
			return codes.OK, true, map[string]interface{}{c.Wrap: appended}
		}
		return codes.OK, true, appended

	case "delete":
		if err := persist.DeleteFile(filePath); err != nil {
			if persist.IsNotFound(err) {
				logger.LogGRPC(fullMethod, codes.NotFound, time.Since(start), logger.SourceStub)
				return codes.NotFound, true, nil
			}
			logger.Error("grpc persist delete file", "file", filePath, "err", err)
			logger.LogGRPC(fullMethod, codes.Internal, time.Since(start), logger.SourceStub)
			return codes.Internal, true, nil
		}
		logger.LogGRPC(fullMethod, codes.OK, time.Since(start), logger.SourceStub)
		return codes.OK, true, nil

	default:
		logger.Warn("grpc persist: unknown merge strategy", "merge", c.Merge)
		logger.LogGRPC(fullMethod, codes.Internal, time.Since(start), logger.SourceStub)
		return codes.Internal, true, nil
	}
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
		exclude[snakeToCamel(field)] = true
	}

	filtered := make(map[string]interface{}, len(reqMap))
	for k, v := range reqMap {
		if !exclude[k] {
			filtered[k] = v
		}
	}
	return filtered
}

// isGRPCDirectoryPath determines if a file path should be treated as a directory.
func isGRPCDirectoryPath(resolvedPath, originalConfigFile string) bool {
	info, err := os.Stat(resolvedPath)
	if err == nil && info.IsDir() {
		return true
	}
	// If path doesn't exist but original config indicated directory intent
	if os.IsNotExist(err) && strings.HasSuffix(originalConfigFile, "/") {
		return true
	}
	return false
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
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", v), "0"), "."), true
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	case nil:
		return "", false
	default:
		return fmt.Sprintf("%v", v), true
	}
}
