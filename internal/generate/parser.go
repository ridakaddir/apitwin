package generate

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Operation is a normalised representation of a single OpenAPI operation.
type Operation struct {
	Method      string // uppercase HTTP method
	Path        string // original OpenAPI path e.g. /users/{id}
	MatchPath   string // apitwin match pattern e.g. /users/*
	Tag         string // first tag or "default"
	OperationID string // operationId if present
	Summary     string
	Responses   []OperationResponse
}

// OperationResponse is a single response entry for an operation.
type OperationResponse struct {
	StatusCode  int
	Description string
	// ExampleBody is ready-to-write JSON bytes (from spec example or synthesised).
	ExampleBody []byte
	// ContentType is the response content type (e.g. application/json).
	ContentType string
	// Schema is the raw schema, kept for synthesis when no example exists.
	Schema *openapi3.Schema
}

// LoadSpec loads an OpenAPI 3 spec from a file path or a URL.
func LoadSpec(source string) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	// Detect URL vs file path.
	u, err := url.ParseRequestURI(source)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return loadFromURL(loader, source)
	}

	return loader.LoadFromFile(source)
}

func loadFromURL(loader *openapi3.Loader, rawURL string) (*openapi3.T, error) {
	resp, err := http.Get(rawURL) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("fetching spec from %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching spec: HTTP %d from %s", resp.StatusCode, rawURL)
	}

	buf, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 32 MB limit
	if readErr != nil {
		return nil, fmt.Errorf("reading spec body from %s: %w", rawURL, readErr)
	}

	return loader.LoadFromData(buf)
}

// ParseOperations extracts all operations from the spec in a stable order.
func ParseOperations(doc *openapi3.T) ([]Operation, error) {
	var ops []Operation

	// Sort paths for deterministic output.
	paths := make([]string, 0)
	if doc.Paths != nil {
		for path := range doc.Paths.Map() {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodHead,
		http.MethodOptions,
	}

	for _, path := range paths {
		item := doc.Paths.Find(path)
		if item == nil {
			continue
		}

		for _, method := range methods {
			op := item.GetOperation(method)
			if op == nil {
				continue
			}

			tag := "default"
			if len(op.Tags) > 0 {
				tag = slugifyTag(op.Tags[0])
			}

			responses, err := parseResponses(op, doc)
			if err != nil {
				return nil, fmt.Errorf("parsing responses for %s %s: %w", method, path, err)
			}

			ops = append(ops, Operation{
				Method:      strings.ToUpper(method),
				Path:        path,
				MatchPath:   openAPIPathToMockr(path),
				Tag:         tag,
				OperationID: op.OperationID,
				Summary:     op.Summary,
				Responses:   responses,
			})
		}
	}

	return ops, nil
}

// parseResponses extracts and synthesises response bodies for an operation.
func parseResponses(op *openapi3.Operation, doc *openapi3.T) ([]OperationResponse, error) {
	if op.Responses == nil {
		return nil, nil
	}

	var responses []OperationResponse
	seenCodes := make(map[int]bool) // deduplicate by numeric status code

	// Sort status codes for deterministic output.
	codes := make([]string, 0)
	for code := range op.Responses.Map() {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	for _, code := range codes {
		ref := op.Responses.Value(code)
		if ref == nil || ref.Value == nil {
			continue
		}
		resp := ref.Value

		// OpenAPI "default" is a catch-all — map to 0 so apitwin uses its own
		// default (200) at runtime and the stub filename doesn't collide with
		// a real 200 response.
		statusCode := 0
		if code != "default" {
			if n, err := strconv.Atoi(code); err == nil {
				statusCode = n
			}
		}

		description := ""
		if resp.Description != nil {
			description = *resp.Description
		}

		// Skip duplicate status codes (can happen with multiple content types).
		if seenCodes[statusCode] {
			continue
		}
		seenCodes[statusCode] = true

		or := OperationResponse{
			StatusCode:  statusCode,
			Description: description,
		}

		// Look for application/json content first, then any content.
		if resp.Content != nil {
			ct, mt := pickMediaType(resp.Content)
			if mt != nil {
				or.ContentType = ct

				// 1. Use first explicit example.
				if body := extractExample(mt); body != nil {
					or.ExampleBody = body
				} else if mt.Schema != nil && mt.Schema.Value != nil {
					// 2. Keep schema for synthesis.
					or.Schema = mt.Schema.Value
				}
			}
		}

		responses = append(responses, or)
	}

	return responses, nil
}

// pickMediaType prefers application/json, then first available in sorted order
// to ensure deterministic output across runs.
func pickMediaType(content openapi3.Content) (string, *openapi3.MediaType) {
	if mt, ok := content["application/json"]; ok {
		return "application/json", mt
	}
	if len(content) == 0 {
		return "", nil
	}
	cts := make([]string, 0, len(content))
	for ct := range content {
		cts = append(cts, ct)
	}
	sort.Strings(cts)
	ct := cts[0]
	return ct, content[ct]
}

// extractExample tries to get a JSON-encodable example from the media type.
func extractExample(mt *openapi3.MediaType) []byte {
	// 1. Inline example on the media type.
	if mt.Example != nil {
		if b := marshalExample(mt.Example); b != nil {
			return b
		}
	}

	// 2. Named examples map — iterate in sorted key order for determinism.
	// Prefer keys named "default" or "example" first, then alphabetical.
	if len(mt.Examples) > 0 {
		keys := make([]string, 0, len(mt.Examples))
		for k := range mt.Examples {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			// Prefer "default" and "example" keys first.
			pi, pj := exampleKeyPriority(keys[i]), exampleKeyPriority(keys[j])
			if pi != pj {
				return pi < pj
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			exRef := mt.Examples[k]
			if exRef == nil || exRef.Value == nil {
				continue
			}
			if exRef.Value.Value != nil {
				if b := marshalExample(exRef.Value.Value); b != nil {
					return b
				}
			}
		}
	}

	// 3. Example on the schema itself.
	if mt.Schema != nil && mt.Schema.Value != nil {
		if mt.Schema.Value.Example != nil {
			if b := marshalExample(mt.Schema.Value.Example); b != nil {
				return b
			}
		}
	}

	return nil
}

// openAPIPathToMockr converts /users/{id}/orders/{orderId} → /users/*/orders/*
func openAPIPathToMockr(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			parts[i] = "*"
		}
	}
	return strings.Join(parts, "/")
}

// exampleKeyPriority gives lower numbers to preferred example key names.
func exampleKeyPriority(key string) int {
	switch strings.ToLower(key) {
	case "default":
		return 0
	case "example":
		return 1
	}
	return 2
}

// slugifyTag converts a tag name to a safe filename segment.
func slugifyTag(tag string) string {
	tag = strings.ToLower(tag)
	tag = strings.ReplaceAll(tag, " ", "-")
	var b strings.Builder
	for _, r := range tag {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
