package persist

import (
	"bytes"
	"strings"
)

// WrapJSON wraps arbitrary JSON content into an object: {"key": <content>}.
// The key is sanitized to remove any characters that would produce invalid JSON.
func WrapJSON(key string, raw []byte) []byte {
	key = sanitizeJSONKey(key)
	trimmed := bytes.TrimSpace(raw)
	var buf bytes.Buffer
	buf.WriteString(`{"`)
	buf.WriteString(key)
	buf.WriteString(`":`)
	buf.Write(trimmed)
	buf.WriteString("}\n")
	return buf.Bytes()
}

// sanitizeJSONKey removes characters that would break a JSON string key.
func sanitizeJSONKey(key string) string {
	r := strings.NewReplacer(`"`, "", `\`, "", "\n", "", "\r", "", "\t", "")
	return r.Replace(key)
}
