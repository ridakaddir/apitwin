package config

import (
	"errors"
	"fmt"
	"strings"
)

// PersistSchema is implemented by transports that have a schema-driven view
// of what a route's persisted entity looks like (gRPC, backed by the proto
// registry). REST has no schema, so its validator does structural-only
// checks and never queries this interface.
type PersistSchema interface {
	// DeriveWrapSource returns the auto-derived wrap and source for the
	// route. ambiguous=true means the schema reports multiple possible
	// matches — caller should treat this as "unresolved." ok=false means
	// the schema has no entry for the route (e.g. wildcard match or proto
	// not loaded).
	DeriveWrapSource(routeMatch string) (wrap, source string, ambiguous, ok bool)

	// MultiMessageResponse reports whether the route's response type has
	// two or more non-repeated message fields — the dangerous shape that
	// requires explicit or inherited wrap/source. ok=false means the
	// schema has no entry for the route.
	MultiMessageResponse(routeMatch string) (multi bool, ok bool)
}

// ValidationError is one fail-fast offender from a config validation pass.
type ValidationError struct {
	Route  string
	Case   string
	Reason string
}

func (e ValidationError) Error() string {
	if e.Case == "" {
		return fmt.Sprintf("route %q: %s", e.Route, e.Reason)
	}
	return fmt.Sprintf("route %q case %q: %s", e.Route, e.Case, e.Reason)
}

// AsError joins a list of ValidationErrors into a single error whose
// Error() text is a multi-line punch list ready to surface to the user.
// Returns nil for an empty list.
func AsError(errs []ValidationError) error {
	if len(errs) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("config validation failed:\n")
	for _, e := range errs {
		b.WriteString("  - ")
		b.WriteString(e.Error())
		b.WriteString("\n")
	}
	return errors.New(b.String())
}

// ValidateGRPCRoutes walks every gRPC route and returns the list of
// validation errors. Returns nil when the config is valid.
//
// Per-route checks:
//  1. The fallback case must exist if referenced.
//
// Per-case checks (only when Persist=true and Merge=="update"):
//  2. After ResolveCase inheritance + auto-derive, if the response type is
//     a multi-message shape (the dangerous one), the case must end up with
//     either a non-empty Wrap, a non-empty Source, or both. Otherwise the
//     full request envelope leaks into the persisted file.
//  3. Update cases must have a non-empty File (after inheritance).
//
// Wildcard route matches (containing "*" or starting with "~") get a
// schema-skip — schema lookup needs an exact method path. Their
// structural checks still run.
func ValidateGRPCRoutes(routes []GRPCRoute, schema PersistSchema) []ValidationError {
	var errs []ValidationError
	for i := range routes {
		route := &routes[i]
		if !route.IsEnabled() {
			continue
		}
		// Fallback referenced but missing.
		if route.Fallback != "" {
			if _, ok := route.Cases[route.Fallback]; !ok {
				errs = append(errs, ValidationError{
					Route:  route.Match,
					Reason: fmt.Sprintf("fallback %q not in cases", route.Fallback),
				})
			}
		}

		// Validate each case that participates as a request handler:
		// fallback + every transition case present in route.Cases +
		// every condition case.
		seen := map[string]bool{}
		toValidate := []string{route.Fallback}
		for _, t := range route.Transitions {
			toValidate = append(toValidate, t.Case)
		}
		for _, cond := range route.Conditions {
			toValidate = append(toValidate, cond.Case)
		}

		isWildcard := strings.Contains(route.Match, "*") || strings.HasPrefix(route.Match, "~")

		for _, name := range toValidate {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			c, ok := ResolveCase(route.Cases, name, route.Fallback)
			if !ok {
				continue // not a request-time case; ignore
			}
			if !c.Persist {
				continue
			}
			if !strings.EqualFold(c.Merge, "update") {
				continue
			}
			if c.File == "" {
				errs = append(errs, ValidationError{
					Route:  route.Match,
					Case:   name,
					Reason: "persist+merge=update requires a non-empty file",
				})
				continue
			}
			if isWildcard || schema == nil {
				// Schema-driven entity checks not available; structural
				// checks above are the only enforcement.
				continue
			}

			multi, schemaOK := schema.MultiMessageResponse(route.Match)
			if !schemaOK {
				continue // proto for this route not loaded
			}
			if !multi {
				// Single-message or direct-entity shapes are safe even
				// without explicit wrap/source — the runtime safeguard
				// + filterToEntityFields handle them.
				continue
			}

			// Multi-message response — the dangerous shape. Resolve final
			// wrap/source: explicit > inherited (already in c) > auto-derived.
			finalWrap, finalSource := c.Wrap, c.Source
			if finalWrap == "" || finalSource == "" {
				dWrap, dSource, ambiguous, ok := schema.DeriveWrapSource(route.Match)
				if !ok || ambiguous {
					// Auto-derive can't help.
				} else {
					if finalWrap == "" {
						finalWrap = dWrap
					}
					if finalSource == "" {
						finalSource = dSource
					}
				}
			}
			if finalWrap == "" && finalSource == "" {
				errs = append(errs, ValidationError{
					Route: route.Match,
					Case:  name,
					Reason: "response has multiple message fields and case has no wrap/source " +
						"(explicit, inherited from fallback, or auto-derived) — request envelope " +
						"would leak into the persisted stub. Set wrap and/or source explicitly.",
				})
			}
		}
	}
	return errs
}

// ValidateRESTRoutes runs structural-only checks on REST routes. There is
// no schema, so the validator can only enforce config self-consistency:
//   - Update cases must have a non-empty File.
//   - Fallback referenced but missing.
//
// Wrap/source enforcement on REST is handled by the runtime safeguard in
// applyPersist (it refuses payloads whose top-level keys collide with the
// wrap key).
func ValidateRESTRoutes(routes []Route) []ValidationError {
	var errs []ValidationError
	for i := range routes {
		route := &routes[i]
		if !route.IsEnabled() {
			continue
		}
		if route.Fallback != "" {
			if _, ok := route.Cases[route.Fallback]; !ok {
				errs = append(errs, ValidationError{
					Route:  route.Method + " " + route.Match,
					Reason: fmt.Sprintf("fallback %q not in cases", route.Fallback),
				})
			}
		}
		seen := map[string]bool{}
		toValidate := []string{route.Fallback}
		for _, t := range route.Transitions {
			toValidate = append(toValidate, t.Case)
		}
		for _, cond := range route.Conditions {
			toValidate = append(toValidate, cond.Case)
		}
		for _, name := range toValidate {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			c, ok := ResolveCase(route.Cases, name, route.Fallback)
			if !ok {
				continue
			}
			if !c.Persist {
				continue
			}
			if !strings.EqualFold(c.Merge, "update") {
				continue
			}
			if c.File == "" {
				errs = append(errs, ValidationError{
					Route:  route.Method + " " + route.Match,
					Case:   name,
					Reason: "persist+merge=update requires a non-empty file",
				})
			}
		}
	}
	return errs
}
