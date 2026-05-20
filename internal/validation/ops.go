package validation

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// op is the signature every validator must implement. value is the raw
// payload value (any JSON type, or nil when absent); found reports whether
// the field was present in the payload; comparand is the raw rule value
// from the config (always a string — parsed per-op as needed).
//
// Returning (true, "") means the rule passed. Returning (false, msg) means
// the rule failed; msg is the default failure message used when the rule
// did not set its own Message override.
type op func(value any, found bool, comparand string) (ok bool, msg string)

// ops is the registered operator set. Adding a new operator is a one-line
// registration; see docs/validation.md for the user-facing reference.
var ops = map[string]op{
	"required":  opRequired,
	"forbidden": opForbidden,
	"type":      opType,
	"const":     opConst,
	"in":        opIn,
	"not_in":    opNotIn,
	"gt":        numericCompare(">"),
	"gte":       numericCompare(">="),
	"lt":        numericCompare("<"),
	"lte":       numericCompare("<="),
	"min_len":   opMinLen,
	"max_len":   opMaxLen,
	"pattern":   opPattern,
	"prefix":    opPrefix,
	"suffix":    opSuffix,
	"contains":  opContains,
	"email":     stringFormat("email", isEmail),
	"uri":       stringFormat("uri", isURI),
	"uuid":      stringFormat("uuid", isUUID),
	"ipv4":      stringFormat("ipv4", isIPv4),
	"ipv6":      stringFormat("ipv6", isIPv6),
	"hostname":  stringFormat("hostname", isHostname),
	"min_items": opMinItems,
	"max_items": opMaxItems,
	"unique":    opUnique,
}

// knownOp reports whether the op name is registered. Used by config-time
// validation to fail-fast on typos.
func knownOp(name string) bool {
	_, ok := ops[name]
	return ok
}

// ---------- Presence ----------

func opRequired(_ any, found bool, _ string) (bool, string) {
	if !found {
		return false, "is required"
	}
	return true, ""
}

func opForbidden(_ any, found bool, _ string) (bool, string) {
	if found {
		return false, "must not be set"
	}
	return true, ""
}

// ---------- Type ----------

func opType(value any, found bool, comparand string) (bool, string) {
	if !found {
		return true, ""
	}
	want := strings.ToLower(strings.TrimSpace(comparand))
	got := jsonType(value)
	switch want {
	case "number":
		if got == "number" || got == "integer" {
			return true, ""
		}
	case "integer":
		if got == "integer" {
			return true, ""
		}
		// Allow whole-number floats (JSON has no integer type natively).
		if f, ok := value.(float64); ok && f == float64(int64(f)) {
			return true, ""
		}
	default:
		if got == want {
			return true, ""
		}
	}
	return false, fmt.Sprintf("must be of type %s (got %s)", want, got)
}

// ---------- Equality / membership ----------

func opConst(value any, found bool, comparand string) (bool, string) {
	if !found {
		return true, ""
	}
	if scalarString(value) == comparand {
		return true, ""
	}
	return false, fmt.Sprintf("must equal %q", comparand)
}

func opIn(value any, found bool, comparand string) (bool, string) {
	if !found {
		return true, ""
	}
	allowed := splitList(comparand)
	v := scalarString(value)
	for _, a := range allowed {
		if v == a {
			return true, ""
		}
	}
	return false, fmt.Sprintf("must be one of [%s]", strings.Join(allowed, ", "))
}

func opNotIn(value any, found bool, comparand string) (bool, string) {
	if !found {
		return true, ""
	}
	blocked := splitList(comparand)
	v := scalarString(value)
	for _, b := range blocked {
		if v == b {
			return false, fmt.Sprintf("must not be one of [%s]", strings.Join(blocked, ", "))
		}
	}
	return true, ""
}

// ---------- Numeric ----------

func numericCompare(sym string) op {
	return func(value any, found bool, comparand string) (bool, string) {
		if !found {
			return true, ""
		}
		v, ok := toFloat(value)
		if !ok {
			return false, fmt.Sprintf("must be a number to compare %s %s", sym, comparand)
		}
		bound, err := strconv.ParseFloat(comparand, 64)
		if err != nil {
			return false, fmt.Sprintf("rule comparand %q is not numeric", comparand)
		}
		var pass bool
		switch sym {
		case ">":
			pass = v > bound
		case ">=":
			pass = v >= bound
		case "<":
			pass = v < bound
		case "<=":
			pass = v <= bound
		}
		if pass {
			return true, ""
		}
		return false, fmt.Sprintf("must be %s %s", sym, comparand)
	}
}

// ---------- String length / format ----------

func opMinLen(value any, found bool, comparand string) (bool, string) {
	if !found {
		return true, ""
	}
	s, ok := value.(string)
	if !ok {
		return false, "must be a string for min_len"
	}
	n, err := strconv.Atoi(comparand)
	if err != nil {
		return false, fmt.Sprintf("rule comparand %q is not an integer", comparand)
	}
	if len(s) >= n {
		return true, ""
	}
	return false, fmt.Sprintf("length must be >= %d", n)
}

func opMaxLen(value any, found bool, comparand string) (bool, string) {
	if !found {
		return true, ""
	}
	s, ok := value.(string)
	if !ok {
		return false, "must be a string for max_len"
	}
	n, err := strconv.Atoi(comparand)
	if err != nil {
		return false, fmt.Sprintf("rule comparand %q is not an integer", comparand)
	}
	if len(s) <= n {
		return true, ""
	}
	return false, fmt.Sprintf("length must be <= %d", n)
}

func opPattern(value any, found bool, comparand string) (bool, string) {
	if !found {
		return true, ""
	}
	s, ok := value.(string)
	if !ok {
		return false, "must be a string to match a pattern"
	}
	re, err := regexp.Compile(comparand)
	if err != nil {
		return false, fmt.Sprintf("rule pattern %q is not a valid regexp", comparand)
	}
	if re.MatchString(s) {
		return true, ""
	}
	return false, fmt.Sprintf("must match pattern %q", comparand)
}

func opPrefix(value any, found bool, comparand string) (bool, string) {
	if !found {
		return true, ""
	}
	s, ok := value.(string)
	if !ok {
		return false, "must be a string for prefix"
	}
	if strings.HasPrefix(s, comparand) {
		return true, ""
	}
	return false, fmt.Sprintf("must start with %q", comparand)
}

func opSuffix(value any, found bool, comparand string) (bool, string) {
	if !found {
		return true, ""
	}
	s, ok := value.(string)
	if !ok {
		return false, "must be a string for suffix"
	}
	if strings.HasSuffix(s, comparand) {
		return true, ""
	}
	return false, fmt.Sprintf("must end with %q", comparand)
}

func opContains(value any, found bool, comparand string) (bool, string) {
	if !found {
		return true, ""
	}
	s, ok := value.(string)
	if !ok {
		return false, "must be a string for contains"
	}
	if strings.Contains(s, comparand) {
		return true, ""
	}
	return false, fmt.Sprintf("must contain %q", comparand)
}

func stringFormat(name string, check func(string) bool) op {
	return func(value any, found bool, _ string) (bool, string) {
		if !found {
			return true, ""
		}
		s, ok := value.(string)
		if !ok {
			return false, fmt.Sprintf("must be a string to be a valid %s", name)
		}
		if check(s) {
			return true, ""
		}
		return false, fmt.Sprintf("must be a valid %s", name)
	}
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// hostnameRe accepts RFC 1123 hostnames: labels of letters/digits/hyphens,
// not starting/ending with a hyphen, separated by dots.
var hostnameRe = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

func isEmail(s string) bool { _, err := mail.ParseAddress(s); return err == nil }
func isURI(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}
func isUUID(s string) bool { return uuidRe.MatchString(s) }
func isIPv4(s string) bool { ip := net.ParseIP(s); return ip != nil && ip.To4() != nil }
func isIPv6(s string) bool { ip := net.ParseIP(s); return ip != nil && ip.To4() == nil && strings.Contains(s, ":") }
func isHostname(s string) bool {
	return len(s) > 0 && len(s) <= 253 && hostnameRe.MatchString(s)
}

// ---------- Arrays ----------

func opMinItems(value any, found bool, comparand string) (bool, string) {
	if !found {
		return true, ""
	}
	arr, ok := value.([]any)
	if !ok {
		return false, "must be an array for min_items"
	}
	n, err := strconv.Atoi(comparand)
	if err != nil {
		return false, fmt.Sprintf("rule comparand %q is not an integer", comparand)
	}
	if len(arr) >= n {
		return true, ""
	}
	return false, fmt.Sprintf("must have at least %d items", n)
}

func opMaxItems(value any, found bool, comparand string) (bool, string) {
	if !found {
		return true, ""
	}
	arr, ok := value.([]any)
	if !ok {
		return false, "must be an array for max_items"
	}
	n, err := strconv.Atoi(comparand)
	if err != nil {
		return false, fmt.Sprintf("rule comparand %q is not an integer", comparand)
	}
	if len(arr) <= n {
		return true, ""
	}
	return false, fmt.Sprintf("must have at most %d items", n)
}

func opUnique(value any, found bool, _ string) (bool, string) {
	if !found {
		return true, ""
	}
	arr, ok := value.([]any)
	if !ok {
		return false, "must be an array for unique"
	}
	seen := make(map[string]struct{}, len(arr))
	for _, item := range arr {
		key := scalarString(item)
		if _, dup := seen[key]; dup {
			return false, "items must be unique"
		}
		seen[key] = struct{}{}
	}
	return true, ""
}

// ---------- Helpers ----------

// jsonType reports the JSON-shaped type of a value decoded from
// encoding/json. Empty string for nil; everything else maps to one of
// string / integer / number / boolean / array / object.
func jsonType(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		if t == float64(int64(t)) {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// scalarString renders any JSON scalar as its string form for equality /
// membership comparisons. Float values that are whole numbers render
// without a trailing ".0" so users can write `value = "5"` and have it
// match the JSON number 5.
func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	}
	return 0, false
}

func splitList(s string) []string {
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
