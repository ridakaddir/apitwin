// Package validation runs config-declared payload checks against decoded
// request bodies. It is transport-agnostic: REST handlers feed it a parsed
// JSON map, gRPC handlers feed it the proto-decoded map. Failures are
// surfaced as a Result the caller turns into the right wire error (HTTP 400
// or gRPC INVALID_ARGUMENT with field-level details).
//
// The operator vocabulary is a superset that covers the rule surface area
// of buf.validate.field (gRPC standard) and OpenAPI 3 schema constraints
// (REST standard). See docs/validation.md for the full mapping table.
package validation

// Violation is one failed rule, reported back to the client.
type Violation struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Result is the outcome of evaluating a full rule set. When OK is false,
// Violations is non-empty and contains one entry per failed rule.
type Result struct {
	OK         bool        `json:"ok"`
	Violations []Violation `json:"violations,omitempty"`
}
