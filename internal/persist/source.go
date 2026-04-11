package persist

import "strings"

// ExtractSourceField walks a dot-notation path through data and returns the
// sub-map at that path. Each path segment is tried as-is first, then as
// camelCase (converting snake_case), so both "country_code" and "countryCode"
// are matched. Returns nil if the path does not resolve to a map.
func ExtractSourceField(data map[string]interface{}, dotPath string) map[string]interface{} {
	parts := strings.Split(dotPath, ".")
	var current interface{} = data
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		v, exists := m[part]
		if !exists {
			v, exists = m[SnakeToCamel(part)]
			if !exists {
				return nil
			}
		}
		current = v
	}
	if sub, ok := current.(map[string]interface{}); ok {
		return sub
	}
	return nil
}

// SnakeToCamel converts a snake_case string to camelCase.
// Exported so both REST and gRPC transports can reuse a single implementation.
func SnakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	if len(parts) == 1 {
		return s
	}
	var b strings.Builder
	for i, p := range parts {
		if i == 0 {
			b.WriteString(p)
			continue
		}
		if len(p) > 0 {
			b.WriteByte(p[0] - 32) // uppercase first letter
			b.WriteString(p[1:])
		}
	}
	return b.String()
}
