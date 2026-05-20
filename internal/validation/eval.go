package validation

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/ridakaddir/apitwin/internal/config"
)

// Validate runs every rule in order and collects all violations. It never
// short-circuits — clients get a complete punch list in one response so
// they can fix everything before retrying.
//
// payload may be nil; rules that check presence (required) will report the
// missing field, rules that gate on found=true will pass through silently.
func Validate(payload map[string]any, rules []config.ValidationRule) Result {
	res := Result{OK: true}
	for _, rule := range rules {
		fn, ok := ops[rule.Op]
		if !ok {
			// Unknown ops should be caught at config load time; if one
			// slips through, surface it as a violation rather than panic.
			res.OK = false
			res.Violations = append(res.Violations, Violation{
				Field:   rule.Field,
				Rule:    rule.Op,
				Message: fmt.Sprintf("unknown validation op %q", rule.Op),
			})
			continue
		}
		value, found := extract(payload, rule.Field)
		passed, defaultMsg := fn(value, found, rule.Value)
		if passed {
			continue
		}
		msg := rule.Message
		if msg == "" {
			msg = fmt.Sprintf("%s %s", rule.Field, defaultMsg)
		}
		res.OK = false
		res.Violations = append(res.Violations, Violation{
			Field:   rule.Field,
			Rule:    rule.Op,
			Message: msg,
		})
	}
	return res
}

// ValidateConfig walks every route and grpc_route in cfg and runs
// ValidateRuleSet on its validation block. Returns the first error
// encountered with the route identifier prefixed. Intended to be called
// from server startup alongside config.ValidateRESTRoutes /
// config.ValidateGRPCRoutes so malformed validation rules fail fast.
func ValidateConfig(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Routes {
		r := &cfg.Routes[i]
		if !r.IsEnabled() || len(r.Validation) == 0 {
			continue
		}
		if err := ValidateRuleSet(r.Validation); err != nil {
			return fmt.Errorf("route %s %s: %w", r.Method, r.Match, err)
		}
	}
	for i := range cfg.GRPCRoutes {
		r := &cfg.GRPCRoutes[i]
		if !r.IsEnabled() || len(r.Validation) == 0 {
			continue
		}
		if err := ValidateRuleSet(r.Validation); err != nil {
			return fmt.Errorf("grpc_route %s: %w", r.Match, err)
		}
	}
	return nil
}

// ValidateRuleSet checks that a list of rules is well-formed: every rule
// has a field and a known op, and ops that need a parseable comparand
// (pattern as regex, gt/gte/lt/lte as float, min_len/max_len/min_items/
// max_items as int, in/not_in as non-empty comma list, type as a known
// JSON type) get their comparand checked here so request-time evaluation
// never has to second-guess the config.
func ValidateRuleSet(rules []config.ValidationRule) error {
	for i, r := range rules {
		if r.Field == "" {
			return fmt.Errorf("rule #%d: field is empty", i)
		}
		if !knownOp(r.Op) {
			return fmt.Errorf("rule #%d (field %q): unknown op %q", i, r.Field, r.Op)
		}
		switch r.Op {
		case "pattern":
			if _, err := regexp.Compile(r.Value); err != nil {
				return fmt.Errorf("rule #%d (field %q): pattern %q does not compile: %v", i, r.Field, r.Value, err)
			}
		case "gt", "gte", "lt", "lte":
			if _, err := strconv.ParseFloat(r.Value, 64); err != nil {
				return fmt.Errorf("rule #%d (field %q): op %q needs a numeric value, got %q", i, r.Field, r.Op, r.Value)
			}
		case "min_len", "max_len", "min_items", "max_items":
			if _, err := strconv.Atoi(r.Value); err != nil {
				return fmt.Errorf("rule #%d (field %q): op %q needs an integer value, got %q", i, r.Field, r.Op, r.Value)
			}
		case "in", "not_in":
			if len(splitList(r.Value)) == 0 {
				return fmt.Errorf("rule #%d (field %q): op %q needs a non-empty comma-separated value list", i, r.Field, r.Op)
			}
		case "type":
			switch r.Value {
			case "string", "number", "integer", "boolean", "array", "object":
			default:
				return fmt.Errorf("rule #%d (field %q): op type expects one of string|number|integer|boolean|array|object, got %q", i, r.Field, r.Value)
			}
		}
	}
	return nil
}
