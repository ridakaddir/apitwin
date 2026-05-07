package config

import (
	"strings"
	"testing"
)

// fakeSchema implements PersistSchema for unit-testing the validator
// without dragging proto dependencies into the config package.
type fakeSchema struct {
	multi   map[string]bool      // routeMatch → has multi-message response
	derived map[string][3]string // routeMatch → [wrap, source, ambiguous("1"|"0")]
	known   map[string]bool      // routeMatch present in schema
}

func (s *fakeSchema) DeriveWrapSource(routeMatch string) (string, string, bool, bool) {
	if !s.known[routeMatch] {
		return "", "", false, false
	}
	d := s.derived[routeMatch]
	return d[0], d[1], d[2] == "1", true
}

func (s *fakeSchema) MultiMessageResponse(routeMatch string) (bool, bool) {
	if !s.known[routeMatch] {
		return false, false
	}
	return s.multi[routeMatch], true
}

func TestValidateGRPCRoutes_MissingWrapAndSource_FailsWhenAutoDeriveBlind(t *testing.T) {
	routes := []GRPCRoute{
		{
			Match:    "/lro.v1.CountryService/UpdateCountry",
			Fallback: "updated",
			Transitions: []Transition{
				{Case: "updating", Duration: 10},
				{Case: "ready"},
			},
			Cases: map[string]Case{
				"updated": {
					Persist: true,
					Merge:   "update",
					File:    "stubs/{body.id}.json",
					// no wrap, no source — but fallback inheritance is no-op
					// because there's nothing to inherit.
				},
				"ready": {
					Persist: true,
					Merge:   "update",
					File:    "stubs/{body.id}.json",
				},
			},
		},
	}
	schema := &fakeSchema{
		known: map[string]bool{"/lro.v1.CountryService/UpdateCountry": true},
		multi: map[string]bool{"/lro.v1.CountryService/UpdateCountry": true},
		// auto-derive returns ambiguous → can't recover.
		derived: map[string][3]string{
			"/lro.v1.CountryService/UpdateCountry": {"", "", "1"},
		},
	}
	errs := ValidateGRPCRoutes(routes, schema)
	if len(errs) == 0 {
		t.Fatal("expected validation errors")
	}
	// Both 'updated' and 'ready' should fail.
	var got []string
	for _, e := range errs {
		got = append(got, e.Case)
	}
	if !contains(got, "updated") || !contains(got, "ready") {
		t.Errorf("expected both cases to fail; got %v", got)
	}
}

func TestValidateGRPCRoutes_AutoDeriveSucceeds_NoError(t *testing.T) {
	routes := []GRPCRoute{
		{
			Match:    "/lro.v1.CountryService/UpdateCountry",
			Fallback: "updated",
			Transitions: []Transition{
				{Case: "ready"},
			},
			Cases: map[string]Case{
				"updated": {Persist: true, Merge: "update", File: "stubs/x.json"},
				"ready":   {Persist: true, Merge: "update", File: "stubs/x.json"},
			},
		},
	}
	schema := &fakeSchema{
		known: map[string]bool{"/lro.v1.CountryService/UpdateCountry": true},
		multi: map[string]bool{"/lro.v1.CountryService/UpdateCountry": true},
		// Auto-derive succeeds: wrap=country, source=country.
		derived: map[string][3]string{
			"/lro.v1.CountryService/UpdateCountry": {"country", "country", "0"},
		},
	}
	errs := ValidateGRPCRoutes(routes, schema)
	if len(errs) != 0 {
		t.Errorf("expected no errors; got %v", errs)
	}
}

func TestValidateGRPCRoutes_TransitionCaseInheritsFromFallback_NoError(t *testing.T) {
	routes := []GRPCRoute{
		{
			Match:    "/lro.v1.CountryService/UpdateCountry",
			Fallback: "updated",
			Transitions: []Transition{
				{Case: "ready"},
			},
			Cases: map[string]Case{
				"updated": {
					Persist: true,
					Merge:   "update",
					File:    "stubs/x.json",
					Wrap:    "country",
					Source:  "country",
				},
				"ready": {
					Persist: true,
					Merge:   "update",
					File:    "stubs/x.json",
					// No wrap/source — must inherit from fallback.
				},
			},
		},
	}
	schema := &fakeSchema{
		known: map[string]bool{"/lro.v1.CountryService/UpdateCountry": true},
		multi: map[string]bool{"/lro.v1.CountryService/UpdateCountry": true},
		// Auto-derive can't recover (ambiguous), but inheritance does.
		derived: map[string][3]string{
			"/lro.v1.CountryService/UpdateCountry": {"", "", "1"},
		},
	}
	errs := ValidateGRPCRoutes(routes, schema)
	if len(errs) != 0 {
		t.Errorf("expected no errors via inheritance; got %v", errs)
	}
}

func TestValidateGRPCRoutes_AppendCasesAreSkipped(t *testing.T) {
	routes := []GRPCRoute{
		{
			Match:    "/lro.v1.CountryService/CreateCountry",
			Fallback: "created",
			Cases: map[string]Case{
				"created": {Persist: true, Merge: "append", File: "stubs/"},
			},
		},
	}
	schema := &fakeSchema{
		known: map[string]bool{"/lro.v1.CountryService/CreateCountry": true},
		multi: map[string]bool{"/lro.v1.CountryService/CreateCountry": true},
		derived: map[string][3]string{
			"/lro.v1.CountryService/CreateCountry": {"", "", "1"},
		},
	}
	errs := ValidateGRPCRoutes(routes, schema)
	if len(errs) != 0 {
		t.Errorf("append cases should not be validated for wrap/source; got %v", errs)
	}
}

func TestValidateGRPCRoutes_WildcardRouteSkipped(t *testing.T) {
	routes := []GRPCRoute{
		{
			Match:    "*",
			Fallback: "updated",
			Cases: map[string]Case{
				"updated": {Persist: true, Merge: "update", File: "stubs/x.json"},
			},
		},
	}
	schema := &fakeSchema{} // no entries at all
	errs := ValidateGRPCRoutes(routes, schema)
	if len(errs) != 0 {
		t.Errorf("wildcard routes should skip schema validation; got %v", errs)
	}
}

func TestValidateGRPCRoutes_FallbackMissing_Fails(t *testing.T) {
	routes := []GRPCRoute{
		{
			Match:    "/lro.v1.X/Y",
			Fallback: "ghost",
			Cases:    map[string]Case{},
		},
	}
	errs := ValidateGRPCRoutes(routes, &fakeSchema{})
	if len(errs) != 1 || !strings.Contains(errs[0].Reason, "fallback") {
		t.Errorf("expected fallback-missing error; got %v", errs)
	}
}

func TestValidateGRPCRoutes_UpdateMissingFile_Fails(t *testing.T) {
	routes := []GRPCRoute{
		{
			Match:    "/lro.v1.X/Y",
			Fallback: "u",
			Cases: map[string]Case{
				"u": {Persist: true, Merge: "update"}, // no file
			},
		},
	}
	errs := ValidateGRPCRoutes(routes, &fakeSchema{})
	found := false
	for _, e := range errs {
		if strings.Contains(e.Reason, "non-empty file") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing-file error; got %v", errs)
	}
}

func TestValidateGRPCRoutes_DisabledRouteSkipped(t *testing.T) {
	off := false
	routes := []GRPCRoute{
		{
			Match:    "/lro.v1.X/Y",
			Enabled:  &off,
			Fallback: "ghost", // would fail if enabled
		},
	}
	if errs := ValidateGRPCRoutes(routes, &fakeSchema{}); len(errs) != 0 {
		t.Errorf("disabled routes should be skipped; got %v", errs)
	}
}

func TestValidateRESTRoutes_PersistUpdateMissingFile_Fails(t *testing.T) {
	routes := []Route{
		{
			Method:   "PATCH",
			Match:    "/countries/{code}",
			Fallback: "updated",
			Cases: map[string]Case{
				"updated": {Persist: true, Merge: "update"}, // no file
			},
		},
	}
	errs := ValidateRESTRoutes(routes)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error; got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Reason, "non-empty file") {
		t.Errorf("expected missing-file error; got %v", errs[0])
	}
}

func TestValidateRESTRoutes_FallbackMissing_Fails(t *testing.T) {
	routes := []Route{
		{
			Method:   "GET",
			Match:    "/x",
			Fallback: "ghost",
			Cases:    map[string]Case{},
		},
	}
	errs := ValidateRESTRoutes(routes)
	if len(errs) != 1 || !strings.Contains(errs[0].Reason, "fallback") {
		t.Errorf("expected fallback-missing error; got %v", errs)
	}
}

func TestValidateRESTRoutes_ValidConfig_NoError(t *testing.T) {
	routes := []Route{
		{
			Method:   "PATCH",
			Match:    "/countries/{code}",
			Fallback: "updated",
			Cases: map[string]Case{
				"updated": {
					Persist: true,
					Merge:   "update",
					File:    "stubs/countries/{params.code}.json",
				},
			},
		},
	}
	if errs := ValidateRESTRoutes(routes); len(errs) != 0 {
		t.Errorf("expected no errors; got %v", errs)
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
