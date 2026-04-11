package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/logger"
	"github.com/ridakaddir/apitwin/internal/persist"
)

// applyPersist reads the stub file, mutates the array according to merge strategy,
// writes the file back, and sends the appropriate HTTP response.
//
// Returns (handled, createdFilePath). handled is true if the response was written
// (caller should not write anything else). createdFilePath is non-empty only for
// merge="append" operations, providing the path of the newly created file.
func applyPersist(w http.ResponseWriter, r *http.Request, c config.Case, bodyBytes []byte, routePattern string, configDir string, pathParams map[string]string) (bool, string) {
	if !c.Persist {
		return false, ""
	}

	filePath := resolveFilePath(c.File, r, bodyBytes, configDir, routePattern, pathParams)

	// Parse request body for incoming record.
	incoming := parseJSONBody(bodyBytes)

	// When source is set, extract only that sub-field from the request body
	// before any other processing. This lets callers persist a nested entity
	// (e.g. "service") without polluting the stub with wrapper fields.
	if c.Source != "" {
		if sub := persist.ExtractSourceField(incoming, c.Source); sub != nil {
			incoming = sub
		} else {
			logger.Warn("persist source: field not found", "source", c.Source)
		}
	}

	switch strings.ToLower(c.Merge) {
	case "cascade":
		// Handle cascade mutations with atomic semantics
		context := persist.RequestContext{
			Body:        incoming,
			PathParams:  pathParams,
			QueryParams: extractQueryParams(r),
			Headers:     extractHeaders(r),
			// Add proxy-specific context needed for path resolution
			Request:      r,
			BodyBytes:    bodyBytes,
			ConfigDir:    configDir,
			RoutePattern: routePattern,
		}

		if err := persist.ExecuteCascade(c, incoming, context); err != nil {
			logger.Error("cascade operation", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return true, ""
		}

		// Return success with operation summary
		result := map[string]interface{}{
			"message":        "Cascade operation completed successfully",
			"primaryFile":    c.Primary.File,
			"cascadeTargets": len(c.Cascade),
		}
		writeJSON(w, c.StatusCode(), result)
		return true, ""

	case "update":
		// Apply defaults if specified (enrich incoming data before persisting).
		var err error
		incoming, err = loadDefaults(c.Defaults, incoming, r, bodyBytes, configDir, routePattern, pathParams, make(map[string]bool))
		if err != nil {
			logger.Error("loading defaults", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load defaults"})
			return true, ""
		}
		updated, err := persist.Update(filePath, incoming)
		if err != nil {
			if persist.IsNotFound(err) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			} else if persist.IsConfigError(err) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			} else {
				logger.Error("persist update", "file", filePath, "err", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stub file error"})
			}
			return true, ""
		}
		// Expand any remaining live refs (directory refs were intentionally
		// kept raw on disk) so the response matches what a subsequent GET
		// would return. Best-effort: on failure, log and return the raw data.
		updated = resolveResponseRefs(updated, r, bodyBytes, configDir, pathParams)
		logger.SetSource(w, logger.SourceStub)
		if c.Wrap != "" {
			writeJSON(w, c.StatusCode(), map[string]any{c.Wrap: updated})
		} else {
			writeJSON(w, c.StatusCode(), updated)
		}
		return true, filePath

	case "append":
		if !persist.IsDirectoryPath(filePath, c.File) {
			logger.Error("persist append", "file", filePath, "err", "append requires directory path")
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "merge=\"append\" requires file to point to a directory (path ending with /)",
			})
			return true, ""
		}
		// Apply defaults if specified (enrich incoming data before persisting).
		var err error
		incoming, err = loadDefaults(c.Defaults, incoming, r, bodyBytes, configDir, routePattern, pathParams, make(map[string]bool))
		if err != nil {
			logger.Error("loading defaults", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to apply defaults"})
			return true, ""
		}

		// Key resolution: if the key field is configured but missing from the
		// incoming record (after defaults), resolve it from path params, then
		// wildcards, then query parameters. This allows named parameters like
		// {endpointId} in the route match pattern to be used as the filename
		// when the body doesn't contain the key field.
		if c.Key != "" {
			if v, exists := incoming[c.Key]; !exists || v == nil || fmt.Sprintf("%v", v) == "" {
				if resolved, ok := extractValue("path", c.Key, r, bodyBytes, routePattern, pathParams); ok {
					incoming[c.Key] = resolved
				} else if resolved, ok := extractValue("query", c.Key, r, bodyBytes, routePattern, pathParams); ok {
					incoming[c.Key] = resolved
				}
			}
		}

		createdPath, result, err := persist.AppendToDir(filePath, c.Key, incoming)
		if err != nil {
			logger.Error("persist append to dir", "dir", filePath, "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stub directory error"})
			return true, ""
		}
		// Expand any remaining live refs (directory refs were intentionally
		// kept raw on disk) so the response matches what a subsequent GET
		// would return. Best-effort: on failure, log and return the raw data.
		result = resolveResponseRefs(result, r, bodyBytes, configDir, pathParams)
		logger.SetSource(w, logger.SourceStub)
		if c.Wrap != "" {
			writeJSON(w, c.StatusCode(), map[string]any{c.Wrap: result})
		} else {
			writeJSON(w, c.StatusCode(), result)
		}
		return true, createdPath

	case "delete":
		if err := persist.DeleteFile(filePath); err != nil {
			if persist.IsNotFound(err) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			} else {
				logger.Error("persist delete file", "file", filePath, "err", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return true, ""
		}
		logger.SetSource(w, logger.SourceStub)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(c.StatusCode())

	default:
		logger.Warn("persist: unknown merge strategy", "merge", c.Merge)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("unknown merge strategy %q. Valid options: update, append, delete", c.Merge),
		})
	}

	return true, ""
}

// resolveFilePath applies dynamic placeholders and configDir to a file path.
func resolveFilePath(filePath string, r *http.Request, bodyBytes []byte, configDir string, routePattern string, pathParams map[string]string) string {
	if hasDynamicPlaceholders(filePath) {
		filePath = resolveDynamicFile(filePath, r, bodyBytes, routePattern, pathParams)
	}
	return absPath(filePath, configDir)
}

// resolveResponseRefs does a best-effort full ref resolution on persist data
// before it is returned to the client. The on-disk stub keeps directory refs
// as live `{{ref:.../}}` templates so subsequent GETs re-resolve them against
// current state; without this helper a POST/PUT response would return the
// raw templates, creating a confusing gap between POST and GET visibility of
// the same record.
//
// On any error (target directory does not exist yet, ref resolution failure,
// JSON round-trip error) the original data is returned unchanged and a warn
// is logged. The persist write has already succeeded by this point, so the
// response must always reflect a valid record even when refs cannot expand.
func resolveResponseRefs(data map[string]interface{}, r *http.Request, bodyBytes []byte, configDir string, pathParams map[string]string) map[string]interface{} {
	raw, err := json.Marshal(data)
	if err != nil {
		logger.Warn("persist response: marshal for ref resolution", "err", err)
		return data
	}
	refCtx := NewRefContext(r, bodyBytes, pathParams)
	resolved, err := resolveRefsWithContext(raw, configDir, make(map[string]bool), refCtx)
	if err != nil {
		logger.Warn("persist response: ref resolution failed", "err", err)
		return data
	}
	var out map[string]interface{}
	if err := json.Unmarshal(resolved, &out); err != nil {
		logger.Warn("persist response: unmarshal resolved", "err", err)
		return data
	}
	return out
}

// loadDefaults reads a defaults JSON file, resolves dynamic placeholders and cross-endpoint
// references, resolves template tokens ({{uuid}}, {{now}}, {{timestamp}}), and deep-merges
// the result under the incoming data so that incoming (request body) fields win on conflicts.
//
// Returns error instead of graceful degradation to ensure data integrity.
func loadDefaults(defaults string, incoming map[string]interface{}, r *http.Request, bodyBytes []byte,
	configDir, routePattern string, pathParams map[string]string, visited map[string]bool) (map[string]interface{}, error) {

	if defaults == "" {
		return incoming, nil
	}

	defaultsPath := resolveFilePath(defaults, r, bodyBytes, configDir, routePattern, pathParams)

	// Ensure resolved path stays within configDir to prevent directory traversal.
	if configDir != "" {
		absConfig, err := filepath.Abs(configDir)
		if err != nil {
			return nil, fmt.Errorf("resolving config directory: %w", err)
		}
		absDefaults, err := filepath.Abs(defaultsPath)
		if err != nil {
			return nil, fmt.Errorf("resolving defaults path: %w", err)
		}
		if !strings.HasPrefix(absDefaults, absConfig+string(filepath.Separator)) && absDefaults != absConfig {
			return nil, fmt.Errorf("defaults path escapes config directory: %s", defaultsPath)
		}
	}

	defaultsData, err := os.ReadFile(defaultsPath)
	if err != nil {
		return nil, fmt.Errorf("reading defaults file %q: %w", defaultsPath, err)
	}

	// Create RefContext for dynamic placeholder resolution
	refCtx := NewRefContext(r, bodyBytes, pathParams)

	// Resolve dynamic placeholders inside {{ref:...}} tokens (e.g. {.endpointId} → "ep-123")
	// but keep the ref tokens themselves intact so they remain as live references
	// in the persisted file. This is critical: if we resolve refs here, the concrete
	// value (e.g. an empty deployment array) gets baked into the file, preventing
	// future reads from picking up new deployments.
	defaultsData, err = resolveDynamicInRefs(defaultsData, refCtx)
	if err != nil {
		return nil, fmt.Errorf("resolving dynamic placeholders in defaults %q: %w", defaultsPath, err)
	}

	// Resolve file-based refs (single files, not directories) because they are
	// static data. Directory refs (ending with /) are left as live references
	// so they resolve dynamically on each read.
	defaultsData, err = resolveFileRefs(defaultsData, configDir, visited)
	if err != nil {
		return nil, fmt.Errorf("resolving refs in defaults %q: %w", defaultsPath, err)
	}

	// Render template tokens ({{uuid}}, {{now}}, {{timestamp}}) and request data placeholders ({.field})
	resolved, err := renderTemplateWithData(string(defaultsData), refCtx)
	if err != nil {
		return nil, fmt.Errorf("rendering template in defaults %q: %w", defaultsPath, err)
	}

	var base map[string]interface{}
	if err := json.Unmarshal([]byte(resolved), &base); err != nil {
		return nil, fmt.Errorf("parsing JSON in defaults %q: %w", defaultsPath, err)
	}

	return persist.DeepMerge(base, incoming), nil
}

// loadDefaultsStatic reads a defaults JSON file, resolves cross-endpoint references,
// and returns its content as a map. Used by the transition scheduler for deferred
// (background) file mutations where no request is available.
//
// The refCtx parameter provides the stored request context from when the transition
// was originally scheduled, enabling dynamic placeholders to work in background transitions.
//
// Returns error instead of graceful degradation to ensure data integrity.
func loadDefaultsStatic(defaults, configDir string, visited map[string]bool, refCtx *RefContext) (map[string]interface{}, error) {
	if defaults == "" {
		return nil, nil
	}

	defaultsPath := absPath(defaults, configDir)

	// Ensure resolved path stays within configDir to prevent directory traversal.
	if configDir != "" {
		absConfig, err := filepath.Abs(configDir)
		if err != nil {
			return nil, fmt.Errorf("resolving config directory: %w", err)
		}
		absDefaults, err := filepath.Abs(defaultsPath)
		if err != nil {
			return nil, fmt.Errorf("resolving defaults path: %w", err)
		}
		if !strings.HasPrefix(absDefaults, absConfig+string(filepath.Separator)) && absDefaults != absConfig {
			return nil, fmt.Errorf("defaults path escapes config directory: %s", defaultsPath)
		}
	}

	defaultsData, err := os.ReadFile(defaultsPath)
	if err != nil {
		return nil, fmt.Errorf("reading defaults file %q: %w", defaultsPath, err)
	}

	// Resolve cross-endpoint references (using stored RefContext)
	defaultsData, err = resolveRefsWithContext(defaultsData, configDir, visited, refCtx)
	if err != nil {
		return nil, fmt.Errorf("resolving refs in defaults %q: %w", defaultsPath, err)
	}

	// Render template tokens ({{uuid}}, {{now}}, {{timestamp}}) and request data placeholders ({.field})
	resolved, err := renderTemplateWithData(string(defaultsData), refCtx)
	if err != nil {
		return nil, fmt.Errorf("rendering template in defaults %q: %w", defaultsPath, err)
	}

	var base map[string]interface{}
	if err := json.Unmarshal([]byte(resolved), &base); err != nil {
		return nil, fmt.Errorf("parsing JSON in defaults %q: %w", defaultsPath, err)
	}

	return base, nil
}

// extractQueryParams extracts query parameters from the request.
func extractQueryParams(r *http.Request) map[string]string {
	params := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			params[key] = values[0] // Take first value if multiple
		}
	}
	return params
}

// extractHeaders extracts headers from the request.
func extractHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0] // Take first value if multiple
		}
	}
	return headers
}
