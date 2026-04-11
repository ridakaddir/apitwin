// Package conditions contains transport-agnostic helpers for evaluating
// config.Condition rules. Value extraction is transport-specific (REST pulls
// from http.Request, gRPC pulls from a decoded map) but the operator switch
// is identical, so both handlers delegate to EvalOp after resolving the value.
package conditions

import (
	"regexp"
	"strings"
)

// EvalOp applies op against (value, compareTo) given whether the source field
// was found on the request. Supported operators: exists, not_exists, eq, neq,
// contains, regex. Unknown operators return false.
//
// - exists/not_exists ignore value and compareTo, they only check found.
// - eq/neq/contains require found=true and compare value against compareTo.
// - regex compiles compareTo as a regular expression; a compile error is
//   treated as no match (false).
func EvalOp(op, value, compareTo string, found bool) bool {
	switch op {
	case "exists":
		return found
	case "not_exists":
		return !found
	case "eq":
		return found && value == compareTo
	case "neq":
		return found && value != compareTo
	case "contains":
		return found && strings.Contains(value, compareTo)
	case "regex":
		if !found {
			return false
		}
		re, err := regexp.Compile(compareTo)
		if err != nil {
			return false
		}
		return re.MatchString(value)
	}
	return false
}
