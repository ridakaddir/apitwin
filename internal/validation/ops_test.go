package validation

import (
	"testing"

	"github.com/ridakaddir/apitwin/internal/config"
)

// All test fixtures use the continent / country / city domain, per the
// project-wide convention.

func TestOps_Required(t *testing.T) {
	rules := []config.ValidationRule{{Field: "name", Op: "required"}}

	t.Run("missing fails", func(t *testing.T) {
		res := Validate(nil, rules)
		if res.OK {
			t.Fatal("expected required to fail on empty payload")
		}
		if len(res.Violations) != 1 || res.Violations[0].Field != "name" {
			t.Fatalf("unexpected violations: %+v", res.Violations)
		}
	})

	t.Run("present passes", func(t *testing.T) {
		res := Validate(map[string]any{"name": "Paris"}, rules)
		if !res.OK {
			t.Fatalf("expected pass, got %+v", res.Violations)
		}
	})
}

func TestOps_Forbidden(t *testing.T) {
	rules := []config.ValidationRule{{Field: "internal_id", Op: "forbidden"}}

	if res := Validate(map[string]any{"internal_id": 1.0}, rules); res.OK {
		t.Fatal("forbidden field present should fail")
	}
	if res := Validate(map[string]any{"name": "Lagos"}, rules); !res.OK {
		t.Fatalf("forbidden field absent should pass: %+v", res.Violations)
	}
}

func TestOps_Type(t *testing.T) {
	cases := []struct {
		name    string
		want    string
		value   any
		shouldPass bool
	}{
		{"string match", "string", "Tokyo", true},
		{"string mismatch", "string", 42.0, false},
		{"integer whole float", "integer", 42.0, true},
		{"integer rejects float", "integer", 1.5, false},
		{"number accepts float", "number", 1.5, true},
		{"boolean match", "boolean", true, true},
		{"array match", "array", []any{1.0, 2.0}, true},
		{"object match", "object", map[string]any{"k": "v"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rules := []config.ValidationRule{{Field: "x", Op: "type", Value: c.want}}
			res := Validate(map[string]any{"x": c.value}, rules)
			if res.OK != c.shouldPass {
				t.Fatalf("type=%s value=%v: got OK=%v, want %v (%+v)",
					c.want, c.value, res.OK, c.shouldPass, res.Violations)
			}
		})
	}
}

func TestOps_InNotIn(t *testing.T) {
	in := []config.ValidationRule{{Field: "continent", Op: "in", Value: "africa,europe,asia"}}
	notIn := []config.ValidationRule{{Field: "continent", Op: "not_in", Value: "antarctica"}}

	if res := Validate(map[string]any{"continent": "europe"}, in); !res.OK {
		t.Fatalf("europe should be in allowed list: %+v", res.Violations)
	}
	if res := Validate(map[string]any{"continent": "atlantis"}, in); res.OK {
		t.Fatal("atlantis should not be in allowed list")
	}
	if res := Validate(map[string]any{"continent": "antarctica"}, notIn); res.OK {
		t.Fatal("antarctica should be blocked by not_in")
	}
	if res := Validate(map[string]any{"continent": "africa"}, notIn); !res.OK {
		t.Fatalf("africa should pass not_in: %+v", res.Violations)
	}
}

func TestOps_NumericBounds(t *testing.T) {
	cases := []struct {
		name       string
		op         string
		bound      string
		value      any
		shouldPass bool
	}{
		{"gte pass", "gte", "0", 100.0, true},
		{"gte fail", "gte", "0", -1.0, false},
		{"gt boundary fail", "gt", "0", 0.0, false},
		{"lt pass", "lt", "1000000000", 500000.0, true},
		{"lte boundary pass", "lte", "100", 100.0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rules := []config.ValidationRule{{Field: "population", Op: c.op, Value: c.bound}}
			res := Validate(map[string]any{"population": c.value}, rules)
			if res.OK != c.shouldPass {
				t.Fatalf("%s %s %v: got OK=%v, want %v (%+v)",
					c.op, c.bound, c.value, res.OK, c.shouldPass, res.Violations)
			}
		})
	}
}

func TestOps_StringLength(t *testing.T) {
	rules := []config.ValidationRule{
		{Field: "name", Op: "min_len", Value: "3"},
		{Field: "name", Op: "max_len", Value: "50"},
	}
	if res := Validate(map[string]any{"name": "Lima"}, rules); !res.OK {
		t.Fatalf("Lima should pass length bounds: %+v", res.Violations)
	}
	if res := Validate(map[string]any{"name": "L"}, rules); res.OK {
		t.Fatal("L should fail min_len=3")
	}
}

func TestOps_PatternAndFormats(t *testing.T) {
	cases := []struct {
		name       string
		rule       config.ValidationRule
		value      any
		shouldPass bool
	}{
		{"country code ok", config.ValidationRule{Field: "code", Op: "pattern", Value: "^[A-Z]{2}$"}, "FR", true},
		{"country code bad", config.ValidationRule{Field: "code", Op: "pattern", Value: "^[A-Z]{2}$"}, "fra", false},
		{"email ok", config.ValidationRule{Field: "contact", Op: "email"}, "mayor@paris.fr", true},
		{"email bad", config.ValidationRule{Field: "contact", Op: "email"}, "not-an-email", false},
		{"uri ok", config.ValidationRule{Field: "site", Op: "uri"}, "https://paris.fr", true},
		{"uuid ok", config.ValidationRule{Field: "id", Op: "uuid"}, "550e8400-e29b-41d4-a716-446655440000", true},
		{"uuid bad", config.ValidationRule{Field: "id", Op: "uuid"}, "not-a-uuid", false},
		{"ipv4 ok", config.ValidationRule{Field: "addr", Op: "ipv4"}, "10.0.0.1", true},
		{"ipv4 bad", config.ValidationRule{Field: "addr", Op: "ipv4"}, "10.0.0.999", false},
		{"hostname ok", config.ValidationRule{Field: "host", Op: "hostname"}, "city.example.com", true},
		{"prefix ok", config.ValidationRule{Field: "name", Op: "prefix", Value: "São "}, "São Paulo", true},
		{"contains ok", config.ValidationRule{Field: "name", Op: "contains", Value: "an"}, "Tirana", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := Validate(map[string]any{c.rule.Field: c.value}, []config.ValidationRule{c.rule})
			if res.OK != c.shouldPass {
				t.Fatalf("rule=%+v value=%v: got OK=%v, want %v (%+v)",
					c.rule, c.value, res.OK, c.shouldPass, res.Violations)
			}
		})
	}
}

func TestOps_Arrays(t *testing.T) {
	rules := []config.ValidationRule{
		{Field: "neighbors", Op: "min_items", Value: "1"},
		{Field: "neighbors", Op: "max_items", Value: "10"},
		{Field: "neighbors", Op: "unique"},
	}
	if res := Validate(map[string]any{"neighbors": []any{"Belgium", "Spain"}}, rules); !res.OK {
		t.Fatalf("valid neighbors list rejected: %+v", res.Violations)
	}
	if res := Validate(map[string]any{"neighbors": []any{}}, rules); res.OK {
		t.Fatal("empty list should fail min_items=1")
	}
	dup := []any{"Belgium", "Belgium"}
	if res := Validate(map[string]any{"neighbors": dup}, rules); res.OK {
		t.Fatal("duplicate neighbors should fail unique")
	}
}

func TestOps_CustomMessageOverride(t *testing.T) {
	rules := []config.ValidationRule{
		{Field: "country_code", Op: "pattern", Value: "^[A-Z]{2}$",
			Message: "country_code must be an ISO 3166-1 alpha-2 code"},
	}
	res := Validate(map[string]any{"country_code": "fra"}, rules)
	if res.OK {
		t.Fatal("expected violation")
	}
	if res.Violations[0].Message != "country_code must be an ISO 3166-1 alpha-2 code" {
		t.Fatalf("custom message not used: %q", res.Violations[0].Message)
	}
}
