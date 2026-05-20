package validation

import "strings"

// extract walks a dot-notation path through a decoded JSON object and
// returns the raw value plus a found flag. Unlike the conditions extractor
// (which coerces every value to string), this one preserves the original
// JSON type so numeric, array, and boolean operators can dispatch on it.
//
// Missing intermediate keys and type mismatches both yield (nil, false).
func extract(payload map[string]any, dotPath string) (any, bool) {
	if payload == nil || dotPath == "" {
		return nil, false
	}
	parts := strings.Split(dotPath, ".")
	var current any = payload
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		v, exists := m[part]
		if !exists {
			return nil, false
		}
		current = v
	}
	return current, true
}
