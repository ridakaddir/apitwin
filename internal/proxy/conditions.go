package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/ridakaddir/apitwin/internal/config"
)

// evalCondition checks whether a single condition matches the request.
// The request body is read from bodyBytes (already buffered) rather than r.Body.
func evalCondition(cond config.Condition, r *http.Request, bodyBytes []byte, routePattern string, pathParams map[string]string) bool {
	val, found := extractValue(cond.Source, cond.Field, r, bodyBytes, routePattern, pathParams)

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

// extractValue retrieves a value from the request based on source and field.
// Returns the string value and whether it was found.
func extractValue(source, field string, r *http.Request, bodyBytes []byte, routePattern string, pathParams map[string]string) (string, bool) {
	switch strings.ToLower(source) {
	case "path":
		// NEW: Check named params first
		if pathParams != nil {
			if v, ok := pathParams[field]; ok && v != "" {
				return v, true
			}
		}
		// Fallback: existing wildcard behavior
		if v, ok := extractWildcardValue(routePattern, r.URL.Path); ok && v != "" {
			return v, true
		}
		return "", false

	case "query":
		v := r.URL.Query().Get(field)
		return v, v != ""

	case "header":
		v := r.Header.Get(field)
		return v, v != ""

	case "body":
		return extractBodyField(field, bodyBytes)
	}

	return "", false
}

// extractBodyField parses the JSON body and resolves a dot-notation field path.
// e.g. "user.address.city" in {"user": {"address": {"city": "Paris"}}}
func extractBodyField(dotPath string, body []byte) (string, bool) {
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
		v, exists := m[part]
		if !exists {
			return "", false
		}
		current = v
	}

	switch v := current.(type) {
	case string:
		return v, true
	case float64:
		// Trim trailing zeros for integer-like numbers.
		s := strings.TrimRight(strings.TrimRight(
			strings.TrimRight(fmt.Sprintf("%f", v), "0"),
			"."), "")
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

// readBody drains r.Body and restores it so the handler can read it again.
func readBody(r *http.Request) []byte {
	if r.Body == nil {
		return nil
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	return data
}
