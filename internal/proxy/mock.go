package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/logger"
	"github.com/ridakaddir/apitwin/internal/persist"
)

// serveMock writes a mock response for the given case to w.
// The responseWriter w should be a buffered responseRecorder to allow
// the caller to inspect the response before finalizing it.
func serveMock(w http.ResponseWriter, r *http.Request, c config.Case, bodyBytes []byte, configDir string, routePattern string, pathParams map[string]string) {
	if c.Delay > 0 {
		time.Sleep(time.Duration(c.Delay) * time.Second)
	}

	// Create RefContext for dynamic placeholder resolution in refs
	refCtx := NewRefContext(r, bodyBytes, pathParams)

	// Create shared visited map for circular detection across stub and defaults
	visited := make(map[string]bool)

	// Determine response body.
	var body []byte
	var err error

	switch {
	case c.File != "":
		filePath := c.File
		if hasDynamicPlaceholders(filePath) {
			filePath = resolveDynamicFile(filePath, r, bodyBytes, routePattern, pathParams)
		}
		// Resolve relative paths against the config file's directory.
		if configDir != "" && !filepath.IsAbs(filePath) {
			filePath = filepath.Join(configDir, filePath)
		}

		// Check if this is a directory path (for aggregation)
		if isDirectoryPath(filePath, c.File) {
			body, err = persist.ReadDir(filePath)
			if err != nil {
				logger.Error("reading stub directory", "dir", filePath, "err", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": "stub directory read error",
				})
				return
			}
		} else {
			body, err = os.ReadFile(filePath)
		}
		if err != nil {
			if os.IsNotExist(err) {
				// Signal caller that the file was not found so it can fall through.
				w.Header().Set("X-Mockr-File-Missing", "true")
				return
			}
			logger.Error("reading stub file", "file", filePath, "err", err)
			http.Error(w, `{"error":"stub file read error"}`, http.StatusInternalServerError)
			return
		}

		// Resolve cross-references in file content
		body, err = resolveRefsWithContext(body, configDir, visited, refCtx)
		if err != nil {
			logger.Error("resolving refs", "file", filePath, "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "ref resolution error",
			})
			return
		}

		// Wrap response into {"field": <content>} if configured.
		if c.Wrap != "" {
			body = wrapJSON(c.Wrap, body)
		}

	case c.JSON != "":
		// Resolve cross-references first
		resolved, err := resolveRefsWithContext([]byte(c.JSON), configDir, visited, refCtx)
		if err != nil {
			logger.Error("resolving refs in inline json", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "ref resolution error",
			})
			return
		}

		// Then render existing template tokens (uuid, now, timestamp)
		rendered, err := renderTemplate(string(resolved))
		if err != nil {
			logger.Error("rendering json template", "err", err)
			http.Error(w, `{"error":"template render error"}`, http.StatusInternalServerError)
			return
		}
		body = []byte(rendered)

	default:
		body = []byte("{}")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(c.StatusCode())
	_, _ = w.Write(body)
}

// renderTemplate processes {{uuid}}, {{now}}, {{timestamp}} tokens in a JSON string.
// Any {{ref:...}} tokens are preserved intact through the rendering process.
func renderTemplate(s string) (string, error) {
	// Temporarily replace {{ref:...}} tokens so the Go template engine doesn't
	// try to interpret them ({{ref would be parsed as calling an undefined "ref" func).
	// Use a UUID-based prefix to guarantee uniqueness and avoid collisions with
	// literal content in the input.
	type placeholder struct {
		key   string
		value string
	}
	baseKey := "__MOCKR_REF_" + uuid.New().String() + "_"
	var placeholders []placeholder
	preserved := refPattern.ReplaceAllStringFunc(s, func(match string) string {
		key := fmt.Sprintf("%s%d__", baseKey, len(placeholders))
		placeholders = append(placeholders, placeholder{key: key, value: match})
		return key
	})

	funcMap := template.FuncMap{
		"uuid": func() string {
			return uuid.New().String()
		},
		"now": func() string {
			return time.Now().UTC().Format(time.RFC3339)
		},
		"timestamp": func() string {
			return fmt.Sprintf("%d", time.Now().UnixMilli())
		},
	}

	// Convert {{token}} to Go template syntax {{call token}}.
	// Our tokens are zero-arg functions so we can call them directly.
	tmpl, err := template.New("mock").Funcs(funcMap).Parse(preserved)
	if err != nil {
		return s, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		return s, err
	}

	// Restore the {{ref:...}} tokens.
	result := buf.String()
	for _, p := range placeholders {
		result = strings.Replace(result, p.key, p.value, 1)
	}

	return result, nil
}

// renderTemplateWithData processes both built-in tokens and request data placeholders.
// Built-in tokens: {{uuid}}, {{now}}, {{timestamp}}
// Request data: {.field}, {path.param}, {query.param}, {header.name}
func renderTemplateWithData(s string, refCtx *RefContext) (string, error) {
	// First resolve request data placeholders using the same logic as dynamic refs
	resolved, err := resolvePlaceholders(s, refCtx)
	if err != nil {
		return s, err
	}

	// Then process built-in template tokens
	return renderTemplate(resolved)
}

// writeJSON writes a JSON-encoded value with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// isFileMissing checks whether serveMock signalled a missing dynamic file.
func isFileMissing(w http.ResponseWriter) bool {
	return w.Header().Get("X-Mockr-File-Missing") == "true"
}

// clearFileMissing removes the internal signal header before sending to client.
func clearFileMissing(w http.ResponseWriter) {
	w.Header().Del("X-Mockr-File-Missing")
}

// wrapJSON wraps arbitrary JSON content into an object: {"key": <content>}.
func wrapJSON(key string, raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	var buf bytes.Buffer
	buf.WriteString(`{"`)
	buf.WriteString(key)
	buf.WriteString(`":`)
	buf.Write(trimmed)
	buf.WriteString("}\n")
	return buf.Bytes()
}

// indentJSON pretty-prints raw JSON bytes.
func indentJSON(raw []byte) []byte {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return raw
	}
	return buf.Bytes()
}

// parseJSONBody decodes JSON body bytes into a map. Returns an empty map on failure.
func parseJSONBody(body []byte) map[string]interface{} {
	out := make(map[string]interface{})
	if len(body) > 0 {
		_ = json.Unmarshal(body, &out)
	}
	return out
}

// absPath resolves a relative file path against configDir.
func absPath(filePath, configDir string) string {
	if configDir != "" && !filepath.IsAbs(filePath) {
		return filepath.Join(configDir, filePath)
	}
	return filePath
}

// slugify converts a URL path to a safe filename segment.
func slugify(s string) string {
	s = strings.TrimPrefix(s, "/")
	s = strings.ReplaceAll(s, "/", "_")
	r := strings.NewReplacer(
		"?", "_", "&", "_", "=", "_", " ", "_",
	)
	return r.Replace(s)
}
