// Package template provides the shared token-rendering primitive used by both
// REST and gRPC handlers to resolve {{uuid}}, {{now}}, and {{timestamp}} in
// mock response strings.
//
// REST layers its ref-token preservation ({{ref:...}}) on top of this base,
// gRPC calls RenderTokens directly.
package template

import (
	"bytes"
	"fmt"
	texttemplate "text/template"
	"time"

	"github.com/google/uuid"
)

// FuncMap returns the template function map with the built-in mock tokens:
//
//	{{uuid}}      — a fresh UUIDv4
//	{{now}}       — current UTC time in RFC3339 format
//	{{timestamp}} — current Unix time in milliseconds
func FuncMap() texttemplate.FuncMap {
	return texttemplate.FuncMap{
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
}

// RenderTokens parses s as a Go text/template with FuncMap() registered and
// executes it with no data context. Returns the rendered string, or (s, err)
// on parse/execute failure so callers can fall back to the raw input.
//
// Callers that need to preserve non-standard tokens (e.g. REST's {{ref:...}}
// cross-reference syntax) should escape those before calling RenderTokens and
// restore them afterwards.
func RenderTokens(s string) (string, error) {
	tmpl, err := texttemplate.New("mock").Funcs(FuncMap()).Parse(s)
	if err != nil {
		return s, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		return s, err
	}
	return buf.String(), nil
}
