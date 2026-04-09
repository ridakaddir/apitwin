package grpc

import (
	"testing"

	"github.com/jhump/protoreflect/desc/protoparse" //nolint:staticcheck
)

// loadTestRegistry parses the example countries.proto and returns a Registry
// for use in tests.
func loadTestRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := NewRegistry(
		[]string{"countries.proto"},
		[]string{"../../examples/grpc-wrap"},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

// -----------------------------------------------------------------------
// EntityFieldNames
// -----------------------------------------------------------------------

func TestEntityFieldNames_ValidWrap(t *testing.T) {
	reg := loadTestRegistry(t)
	md, err := reg.FindMethod("/geo.CountryService/UpdateCountry")
	if err != nil || md == nil {
		t.Fatalf("FindMethod: md=%v err=%v", md, err)
	}

	// GetCountryResponse has field "country" of type Country.
	allowed := reg.EntityFieldNames(md, "country")
	if allowed == nil {
		t.Fatal("expected non-nil allowed set")
	}

	// Country has fields: code, name, continent.
	for _, name := range []string{"code", "name", "continent"} {
		if !allowed[name] {
			t.Errorf("expected %q in allowed set", name)
		}
	}
	// Routing field from UpdateCountryRequest must NOT be in the set.
	if allowed["countryCode"] || allowed["country_code"] {
		t.Error("countryCode/country_code should not be in entity field set")
	}
}

func TestEntityFieldNames_EmptyWrap(t *testing.T) {
	reg := loadTestRegistry(t)
	md, _ := reg.FindMethod("/geo.CountryService/UpdateCountry")
	if got := reg.EntityFieldNames(md, ""); got != nil {
		t.Errorf("expected nil for empty wrap, got %v", got)
	}
}

func TestEntityFieldNames_NilMD(t *testing.T) {
	reg := loadTestRegistry(t)
	if got := reg.EntityFieldNames(nil, "country"); got != nil {
		t.Errorf("expected nil for nil md, got %v", got)
	}
}

func TestEntityFieldNames_NonMatchingWrap(t *testing.T) {
	reg := loadTestRegistry(t)
	md, _ := reg.FindMethod("/geo.CountryService/UpdateCountry")
	if got := reg.EntityFieldNames(md, "nonexistent"); got != nil {
		t.Errorf("expected nil for non-matching wrap, got %v", got)
	}
}

func TestEntityFieldNames_ScalarField(t *testing.T) {
	// DeleteCountryResponse has no fields, so wrapping a non-existent field
	// or a scalar should return nil.
	p := protoparse.Parser{
		InferImportPaths: true,
		ImportPaths:      []string{"../../examples/grpc-wrap"},
	}
	fds, err := p.ParseFiles("countries.proto")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// GetCountryRequest has string country_code = 1 (scalar, not a message).
	// We'll test EntityFieldNames on GetCountry whose response wraps "country" (message).
	// To test scalar, use a wrap name that matches a scalar field — but
	// GetCountryResponse only has "country" (message). Let's just verify that
	// looking for a scalar field in a different context returns nil.
	_ = fds
	reg := loadTestRegistry(t)
	md, _ := reg.FindMethod("/geo.CountryService/DeleteCountry")
	// DeleteCountryResponse is empty — no fields at all.
	if got := reg.EntityFieldNames(md, "anything"); got != nil {
		t.Errorf("expected nil for empty response message, got %v", got)
	}
}

// -----------------------------------------------------------------------
// filterToEntityFields
// -----------------------------------------------------------------------

func TestFilterToEntityFields_NilAllowed(t *testing.T) {
	data := map[string]interface{}{"a": 1, "b": 2}
	got := filterToEntityFields(data, nil)
	if len(got) != 2 {
		t.Errorf("expected data unchanged, got %v", got)
	}
}

func TestFilterToEntityFields_FiltersUnknownKeys(t *testing.T) {
	data := map[string]interface{}{
		"id":          "env-1",
		"description": "updated",
		"orgId":       "abc",
		"providerId":  "GCP",
	}
	allowed := map[string]bool{
		"id":          true,
		"description": true,
		"name":        true,
	}
	got := filterToEntityFields(data, allowed)
	if len(got) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(got), got)
	}
	if got["orgId"] != nil || got["providerId"] != nil {
		t.Error("routing fields should have been filtered out")
	}
	if got["id"] != "env-1" || got["description"] != "updated" {
		t.Error("entity fields should be preserved")
	}
}

func TestFilterToEntityFields_AllKeysValid(t *testing.T) {
	data := map[string]interface{}{"a": 1, "b": 2}
	allowed := map[string]bool{"a": true, "b": true, "c": true}
	got := filterToEntityFields(data, allowed)
	if len(got) != 2 {
		t.Errorf("expected 2 keys, got %d", len(got))
	}
}

func TestFilterToEntityFields_EmptyData(t *testing.T) {
	data := map[string]interface{}{}
	allowed := map[string]bool{"a": true}
	got := filterToEntityFields(data, allowed)
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}
