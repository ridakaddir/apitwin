package persist

import "testing"

func TestExtractSourceField_TopLevel(t *testing.T) {
	m := map[string]interface{}{
		"continent": "Europe",
		"country": map[string]interface{}{
			"name": "France",
			"code": "FR",
		},
	}
	got := ExtractSourceField(m, "country")
	if got == nil {
		t.Fatal("expected non-nil sub-map")
	}
	if got["name"] != "France" {
		t.Errorf("expected name=France, got %v", got["name"])
	}
	if got["continent"] != nil {
		t.Error("continent should not be in extracted sub-map")
	}
}

func TestExtractSourceField_Nested(t *testing.T) {
	m := map[string]interface{}{
		"continent": map[string]interface{}{
			"country": map[string]interface{}{
				"name": "France",
			},
		},
	}
	got := ExtractSourceField(m, "continent.country")
	if got == nil {
		t.Fatal("expected non-nil sub-map")
	}
	if got["name"] != "France" {
		t.Errorf("expected name=France, got %v", got["name"])
	}
}

func TestExtractSourceField_CamelCaseFallback(t *testing.T) {
	m := map[string]interface{}{
		"countryInfo": map[string]interface{}{"name": "France"},
	}
	got := ExtractSourceField(m, "country_info")
	if got == nil {
		t.Fatal("expected non-nil sub-map via camelCase fallback")
	}
	if got["name"] != "France" {
		t.Errorf("expected name=France, got %v", got["name"])
	}
}

func TestExtractSourceField_NotFound(t *testing.T) {
	m := map[string]interface{}{"a": "b"}
	got := ExtractSourceField(m, "missing")
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestExtractSourceField_NotAMap(t *testing.T) {
	m := map[string]interface{}{"name": "scalar-value"}
	got := ExtractSourceField(m, "name")
	if got != nil {
		t.Errorf("expected nil for scalar value, got %v", got)
	}
}
