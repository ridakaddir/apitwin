package config

import "testing"

func TestResolveCase_MissingFieldsInheritFromFallback(t *testing.T) {
	cases := map[string]Case{
		"updated": {
			Persist: true,
			Merge:   "update",
			File:    "stubs/countries/{body.code}.json",
			Wrap:    "country",
			Source:  "country",
		},
		"ready": {
			Persist:  true,
			Defaults: "stubs/defaults/country-ready.json",
		},
	}
	got, ok := ResolveCase(cases, "ready", "updated")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got.Wrap != "country" {
		t.Errorf("Wrap = %q, want inherited %q", got.Wrap, "country")
	}
	if got.Source != "country" {
		t.Errorf("Source = %q, want inherited %q", got.Source, "country")
	}
	if got.File != "stubs/countries/{body.code}.json" {
		t.Errorf("File = %q, want inherited", got.File)
	}
	if got.Merge != "update" {
		t.Errorf("Merge = %q, want inherited %q", got.Merge, "update")
	}
	if got.Defaults != "stubs/defaults/country-ready.json" {
		t.Errorf("Defaults = %q, want kept (not overridden)", got.Defaults)
	}
}

func TestResolveCase_NonEmptyFieldsAreNotOverridden(t *testing.T) {
	cases := map[string]Case{
		"updated": {Wrap: "country", Source: "country", Merge: "update"},
		"ready":   {Wrap: "ignored", Source: "ignored", Merge: "append"},
	}
	got, _ := ResolveCase(cases, "ready", "updated")
	if got.Wrap != "ignored" {
		t.Errorf("Wrap = %q, want kept %q", got.Wrap, "ignored")
	}
	if got.Source != "ignored" {
		t.Errorf("Source = %q, want kept %q", got.Source, "ignored")
	}
	if got.Merge != "append" {
		t.Errorf("Merge = %q, want kept %q", got.Merge, "append")
	}
}

func TestResolveCase_PersistNotInherited(t *testing.T) {
	cases := map[string]Case{
		"updated": {Persist: true, Merge: "update"},
		"readonly": {Wrap: "country"}, // Persist defaults false; must stay false
	}
	got, _ := ResolveCase(cases, "readonly", "updated")
	if got.Persist {
		t.Error("Persist must not be inherited (intent flag)")
	}
	// But Merge should be inherited.
	if got.Merge != "update" {
		t.Errorf("Merge = %q, want inherited %q", got.Merge, "update")
	}
}

func TestResolveCase_StatusJSONDelayNotInherited(t *testing.T) {
	cases := map[string]Case{
		"updated": {Status: 200, JSON: `{"ok":true}`, Delay: 5, Wrap: "country"},
		"ready":   {}, // all zero values
	}
	got, _ := ResolveCase(cases, "ready", "updated")
	if got.Status != 0 {
		t.Errorf("Status inherited (= %d); should not be", got.Status)
	}
	if got.JSON != "" {
		t.Errorf("JSON inherited; should not be")
	}
	if got.Delay != 0 {
		t.Errorf("Delay inherited (= %d); should not be", got.Delay)
	}
	// Wrap must be inherited though.
	if got.Wrap != "country" {
		t.Errorf("Wrap not inherited: %q", got.Wrap)
	}
}

func TestResolveCase_FallbackCaseReturnsUnchanged(t *testing.T) {
	cases := map[string]Case{
		"updated": {Wrap: "country", Source: "country"},
	}
	got, ok := ResolveCase(cases, "updated", "updated")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got.Wrap != "country" || got.Source != "country" {
		t.Errorf("fallback case should be returned as-is: %+v", got)
	}
}

func TestResolveCase_NoFallbackReturnsUnchanged(t *testing.T) {
	cases := map[string]Case{
		"only": {Wrap: "country"},
	}
	got, ok := ResolveCase(cases, "only", "")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got.Wrap != "country" {
		t.Errorf("Wrap = %q", got.Wrap)
	}
}

func TestResolveCase_FallbackNotInCasesReturnsUnchanged(t *testing.T) {
	cases := map[string]Case{
		"ready": {Wrap: "country"},
	}
	got, ok := ResolveCase(cases, "ready", "missing")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got.Wrap != "country" {
		t.Errorf("Wrap = %q", got.Wrap)
	}
}

func TestResolveCase_UnknownCaseReturnsFalse(t *testing.T) {
	cases := map[string]Case{
		"updated": {Wrap: "country"},
	}
	_, ok := ResolveCase(cases, "nonexistent", "updated")
	if ok {
		t.Error("expected ok=false for unknown caseName")
	}
}

func TestResolveCase_InheritsKeyAndArrayKey(t *testing.T) {
	cases := map[string]Case{
		"updated": {Key: "code", ArrayKey: "countries"},
		"ready":   {},
	}
	got, _ := ResolveCase(cases, "ready", "updated")
	if got.Key != "code" {
		t.Errorf("Key not inherited: %q", got.Key)
	}
	if got.ArrayKey != "countries" {
		t.Errorf("ArrayKey not inherited: %q", got.ArrayKey)
	}
}

func TestResolveCase_InlineJSONCaseDoesNotInheritFileOrMerge(t *testing.T) {
	// Mirrors the QA scenario: an inline-JSON condition case (e.g. "invalid"
	// returning INVALID_ARGUMENT with a static body) sits next to a
	// file-backed fallback case. Inheriting File / Merge would turn the
	// inline case into a directory reader and break the JSON precedence in
	// loadStub.
	cases := map[string]Case{
		"created": {
			Persist:  true,
			Merge:    "append",
			File:     "stubs/x/",
			Key:      "code",
			Defaults: "defaults/x.json",
			Wrap:     "country",
		},
		"invalid": {
			Status: 3,
			JSON:   `{"reason":"bad code"}`,
		},
	}
	got, _ := ResolveCase(cases, "invalid", "created")
	if got.JSON == "" {
		t.Error("inline JSON dropped")
	}
	if got.File != "" {
		t.Errorf("File inherited onto inline-JSON case: %q", got.File)
	}
	if got.Merge != "" {
		t.Errorf("Merge inherited onto inline-JSON case: %q", got.Merge)
	}
	if got.Key != "" {
		t.Errorf("Key inherited onto inline-JSON case: %q", got.Key)
	}
	if got.Defaults != "" {
		t.Errorf("Defaults inherited onto inline-JSON case: %q", got.Defaults)
	}
	// Wrap/Source should still be inherited so the inline body is wrapped
	// consistently with the rest of the route.
	if got.Wrap != "country" {
		t.Errorf("Wrap not inherited on inline-JSON case: %q", got.Wrap)
	}
}

func TestResolveCase_PrimaryAndCascadeNotInherited(t *testing.T) {
	cases := map[string]Case{
		"updated": {
			Wrap:    "country",
			Primary: &CascadePrimary{File: "x.json", Merge: "update"},
			Cascade: []CascadeTarget{{Pattern: "y/*.json", Merge: "delete"}},
		},
		"ready": {},
	}
	got, _ := ResolveCase(cases, "ready", "updated")
	if got.Primary != nil {
		t.Error("Primary must not be inherited")
	}
	if got.Cascade != nil {
		t.Error("Cascade must not be inherited")
	}
	if got.Wrap != "country" {
		t.Errorf("Wrap not inherited: %q", got.Wrap)
	}
}
