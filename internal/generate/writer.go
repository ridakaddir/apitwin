package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// WriteOptions controls how config files and stubs are written.
type WriteOptions struct {
	OutDir  string // output directory
	Format  string // "toml" | "yaml" | "json"
	Split   bool   // one file per tag when true, single file when false
	SpecSrc string // original spec source for comment header
}

// WriteResult reports what was written.
type WriteResult struct {
	ConfigFiles []string
	StubFiles   []string
}

// Write outputs config files and stub JSON files to OutDir.
func Write(groups map[string][]Operation, opts WriteOptions) (*WriteResult, error) {
	result := &WriteResult{}

	stubsDir := filepath.Join(opts.OutDir, "stubs")
	if err := os.MkdirAll(stubsDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	tags := sortedKeys(groups)
	tagRoutes := make(map[string]string) // tag → rendered config block(s)

	for _, tag := range tags {
		ops := groups[tag]
		var blocks strings.Builder

		for _, op := range ops {
			// Write each stub file.
			for i := range op.Responses {
				resp := &op.Responses[i]

				body := resp.ExampleBody
				if body == nil && resp.Schema != nil {
					body = SynthesiseJSON(resp.Schema)
				}
				if body == nil {
					body = []byte(`{}`)
				}

				stubFile := stubFilename(op.Method, op.Path, resp.StatusCode)
				stubPath := filepath.Join(stubsDir, stubFile)
				if err := os.WriteFile(stubPath, prettyJSON(body), 0644); err != nil {
					return nil, fmt.Errorf("writing stub %s: %w", stubPath, err)
				}
				result.StubFiles = append(result.StubFiles, stubPath)
			}

			// Render route block.
			switch opts.Format {
			case "yaml":
				blocks.WriteString(buildRouteYAML(op))
			default: // toml
				blocks.WriteString(buildRouteTOML(op))
			}
		}

		tagRoutes[tag] = blocks.String()
	}

	// Write config files.
	if opts.Format == "json" {
		// JSON: single file always, full array structure.
		cfgPath := filepath.Join(opts.OutDir, "apitwin.json")
		content, err := buildJSONConfig(groups)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(cfgPath, content, 0644); err != nil {
			return nil, fmt.Errorf("writing config %s: %w", cfgPath, err)
		}
		result.ConfigFiles = append(result.ConfigFiles, cfgPath)
		return result, nil
	}

	ext := opts.Format // "toml" or "yaml"

	if opts.Split {
		for _, tag := range tags {
			content := fileHeader(opts.SpecSrc, tag, opts.Format) + tagRoutes[tag]
			cfgPath := filepath.Join(opts.OutDir, tag+"."+ext)
			if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
				return nil, fmt.Errorf("writing config %s: %w", cfgPath, err)
			}
			result.ConfigFiles = append(result.ConfigFiles, cfgPath)
		}
	} else {
		var all strings.Builder
		all.WriteString(fileHeader(opts.SpecSrc, "", opts.Format))
		for _, tag := range tags {
			all.WriteString(tagRoutes[tag])
		}
		cfgPath := filepath.Join(opts.OutDir, "apitwin."+ext)
		if err := os.WriteFile(cfgPath, []byte(all.String()), 0644); err != nil {
			return nil, fmt.Errorf("writing config %s: %w", cfgPath, err)
		}
		result.ConfigFiles = append(result.ConfigFiles, cfgPath)
	}

	return result, nil
}

// ─── TOML renderer ────────────────────────────────────────────────────────────

func buildRouteTOML(op Operation) string {
	var b strings.Builder

	if op.Summary != "" {
		fmt.Fprintf(&b, "# %s\n", op.Summary)
	}

	b.WriteString("[[routes]]\n")
	fmt.Fprintf(&b, "method   = %q\n", op.Method)
	fmt.Fprintf(&b, "match    = %q\n", op.MatchPath)
	b.WriteString("enabled  = true\n")
	fmt.Fprintf(&b, "fallback = %q\n", fallbackCaseName(op.Responses))
	b.WriteString("\n")

	for _, resp := range op.Responses {
		caseName := statusCaseName(resp.StatusCode)
		stubFile := filepath.Join("stubs", stubFilename(op.Method, op.Path, resp.StatusCode))
		fmt.Fprintf(&b, "  [routes.cases.%s]\n", caseName)
		fmt.Fprintf(&b, "  status = %d\n", resp.StatusCode)
		fmt.Fprintf(&b, "  file   = %q\n", stubFile)
		b.WriteString("\n")
	}

	return b.String()
}

// ─── YAML renderer ────────────────────────────────────────────────────────────

func buildRouteYAML(op Operation) string {
	var b strings.Builder

	if op.Summary != "" {
		fmt.Fprintf(&b, "  # %s\n", op.Summary)
	}

	fmt.Fprintf(&b, "  - method: %s\n", op.Method)
	fmt.Fprintf(&b, "    match: %s\n", op.MatchPath)
	b.WriteString("    enabled: true\n")
	fmt.Fprintf(&b, "    fallback: %s\n", fallbackCaseName(op.Responses))
	b.WriteString("    cases:\n")

	for _, resp := range op.Responses {
		caseName := statusCaseName(resp.StatusCode)
		stubFile := filepath.Join("stubs", stubFilename(op.Method, op.Path, resp.StatusCode))
		fmt.Fprintf(&b, "      %s:\n", caseName)
		fmt.Fprintf(&b, "        status: %d\n", resp.StatusCode)
		fmt.Fprintf(&b, "        file: %s\n", stubFile)
	}
	b.WriteString("\n")

	return b.String()
}

// ─── JSON renderer ────────────────────────────────────────────────────────────

func buildJSONConfig(groups map[string][]Operation) ([]byte, error) {
	type jsonCase struct {
		Status int    `json:"status"`
		File   string `json:"file"`
	}
	type jsonRoute struct {
		Method   string              `json:"method"`
		Match    string              `json:"match"`
		Enabled  bool                `json:"enabled"`
		Fallback string              `json:"fallback"`
		Cases    map[string]jsonCase `json:"cases"`
	}
	type jsonConfig struct {
		Routes []jsonRoute `json:"routes"`
	}

	var routes []jsonRoute
	for _, tag := range sortedKeys(groups) {
		for _, op := range groups[tag] {
			cases := make(map[string]jsonCase)
			for _, resp := range op.Responses {
				name := statusCaseName(resp.StatusCode)
				cases[name] = jsonCase{
					Status: resp.StatusCode,
					File:   filepath.Join("stubs", stubFilename(op.Method, op.Path, resp.StatusCode)),
				}
			}
			routes = append(routes, jsonRoute{
				Method:   op.Method,
				Match:    op.MatchPath,
				Enabled:  true,
				Fallback: fallbackCaseName(op.Responses),
				Cases:    cases,
			})
		}
	}

	out, err := json.MarshalIndent(jsonConfig{Routes: routes}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func fileHeader(specSrc, tag, format string) string {
	comment := func(s string) string {
		if format == "yaml" || format == "toml" {
			return "# " + s + "\n"
		}
		return ""
	}

	var h strings.Builder
	h.WriteString(comment("Generated from " + specSrc))
	if tag != "" {
		h.WriteString(comment("Tag: " + tag))
	}

	if format == "yaml" {
		h.WriteString("routes:\n")
	} else {
		h.WriteString("\n")
	}

	return h.String()
}

// stubFilename builds a safe filename: get_users_id_200.json
func stubFilename(method, path string, statusCode int) string {
	slug := strings.ToLower(method) + "_" + pathSlug(path) + "_" + strconv.Itoa(statusCode)
	return slug + ".json"
}

// pathSlug converts /users/{id}/orders → users_id_orders
func pathSlug(path string) string {
	path = strings.TrimPrefix(path, "/")
	path = strings.ReplaceAll(path, "/", "_")
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")
	path = strings.ReplaceAll(path, "-", "_")
	return path
}

// statusCaseName maps an HTTP status code to a human-readable case name.
// code 0 is used for OpenAPI "default" responses.
func statusCaseName(code int) string {
	switch code {
	case 0:
		return "default"
	case 200:
		return "success"
	case 201:
		return "created"
	case 202:
		return "accepted"
	case 204:
		return "no_content"
	case 400:
		return "bad_request"
	case 401:
		return "unauthorized"
	case 403:
		return "forbidden"
	case 404:
		return "not_found"
	case 409:
		return "conflict"
	case 422:
		return "unprocessable"
	case 429:
		return "too_many_requests"
	}
	if code >= 200 && code < 300 {
		return fmt.Sprintf("success_%d", code)
	}
	if code >= 400 && code < 500 {
		return fmt.Sprintf("error_%d", code)
	}
	if code >= 500 {
		return fmt.Sprintf("error_%d", code)
	}
	return fmt.Sprintf("status_%d", code)
}

// fallbackCaseName picks the first 2xx case, or the first case overall.
func fallbackCaseName(responses []OperationResponse) string {
	for _, r := range responses {
		if r.StatusCode >= 200 && r.StatusCode < 300 {
			return statusCaseName(r.StatusCode)
		}
	}
	if len(responses) > 0 {
		return statusCaseName(responses[0].StatusCode)
	}
	return "success"
}

// prettyJSON indents raw JSON bytes.
func prettyJSON(b []byte) []byte {
	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		return b
	}
	buf.WriteByte('\n')
	return buf.Bytes()
}

// sortedKeys returns map keys in alphabetical order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
