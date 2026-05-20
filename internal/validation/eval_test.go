package validation

import (
	"strings"
	"testing"

	"github.com/ridakaddir/apitwin/internal/config"
)

func TestValidate_CollectsAllViolations(t *testing.T) {
	rules := []config.ValidationRule{
		{Field: "name", Op: "required"},
		{Field: "country_code", Op: "required"},
		{Field: "population", Op: "gte", Value: "0"},
	}
	res := Validate(map[string]any{"population": -5.0}, rules)
	if res.OK {
		t.Fatal("expected failure")
	}
	if len(res.Violations) != 3 {
		t.Fatalf("expected 3 violations, got %d: %+v", len(res.Violations), res.Violations)
	}
}

func TestValidate_NestedDotPath(t *testing.T) {
	rules := []config.ValidationRule{
		{Field: "city.country.continent", Op: "in", Value: "africa,europe,asia"},
	}
	payload := map[string]any{
		"city": map[string]any{
			"country": map[string]any{"continent": "asia"},
		},
	}
	if res := Validate(payload, rules); !res.OK {
		t.Fatalf("nested dot-path should resolve: %+v", res.Violations)
	}
}

func TestValidate_NilPayloadTriggersRequired(t *testing.T) {
	rules := []config.ValidationRule{
		{Field: "anything", Op: "required"},
	}
	res := Validate(nil, rules)
	if res.OK || len(res.Violations) != 1 {
		t.Fatalf("nil payload should fail required: %+v", res)
	}
}

func TestValidate_AbsentFieldSkipsNonPresenceRules(t *testing.T) {
	// max_len etc should pass when the field is absent — only `required`
	// and `forbidden` look at presence directly. This matches buf.validate
	// semantics: missing optional fields skip rule evaluation.
	rules := []config.ValidationRule{
		{Field: "name", Op: "max_len", Value: "10"},
		{Field: "population", Op: "gte", Value: "0"},
	}
	if res := Validate(map[string]any{}, rules); !res.OK {
		t.Fatalf("absent fields should not fail non-presence rules: %+v", res.Violations)
	}
}

func TestValidateRuleSet(t *testing.T) {
	t.Run("rejects empty field", func(t *testing.T) {
		err := ValidateRuleSet([]config.ValidationRule{{Op: "required"}})
		if err == nil || !strings.Contains(err.Error(), "field is empty") {
			t.Fatalf("expected empty-field error, got %v", err)
		}
	})
	t.Run("rejects unknown op", func(t *testing.T) {
		err := ValidateRuleSet([]config.ValidationRule{{Field: "name", Op: "frobulate"}})
		if err == nil || !strings.Contains(err.Error(), "unknown op") {
			t.Fatalf("expected unknown-op error, got %v", err)
		}
	})
	t.Run("rejects bad regex", func(t *testing.T) {
		err := ValidateRuleSet([]config.ValidationRule{{Field: "x", Op: "pattern", Value: "[unclosed"}})
		if err == nil || !strings.Contains(err.Error(), "does not compile") {
			t.Fatalf("expected regex-compile error, got %v", err)
		}
	})
	t.Run("rejects non-numeric bound", func(t *testing.T) {
		err := ValidateRuleSet([]config.ValidationRule{{Field: "n", Op: "gte", Value: "abc"}})
		if err == nil || !strings.Contains(err.Error(), "numeric") {
			t.Fatalf("expected numeric-value error, got %v", err)
		}
	})
	t.Run("rejects empty in list", func(t *testing.T) {
		err := ValidateRuleSet([]config.ValidationRule{{Field: "c", Op: "in", Value: " , "}})
		if err == nil {
			t.Fatal("expected error for empty in list")
		}
	})
	t.Run("rejects unknown type value", func(t *testing.T) {
		err := ValidateRuleSet([]config.ValidationRule{{Field: "x", Op: "type", Value: "thing"}})
		if err == nil {
			t.Fatal("expected error for unknown type")
		}
	})
	t.Run("accepts well-formed rules", func(t *testing.T) {
		err := ValidateRuleSet([]config.ValidationRule{
			{Field: "name", Op: "required"},
			{Field: "population", Op: "gte", Value: "0"},
			{Field: "code", Op: "pattern", Value: "^[A-Z]{2}$"},
			{Field: "continent", Op: "in", Value: "africa,europe,asia"},
			{Field: "tags", Op: "min_items", Value: "1"},
		})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestValidateConfig_WalksBothTransports(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{{
			Method: "POST", Match: "/cities",
			Validation: []config.ValidationRule{{Field: "name", Op: "required"}},
		}},
		GRPCRoutes: []config.GRPCRoute{{
			Match: "/cities.Cities/CreateCity",
			Validation: []config.ValidationRule{{Field: "name", Op: "required"}},
		}},
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("well-formed config rejected: %v", err)
	}

	cfg.Routes[0].Validation[0].Op = "frobulate"
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "POST /cities") {
		t.Fatalf("expected REST route error, got %v", err)
	}
}
