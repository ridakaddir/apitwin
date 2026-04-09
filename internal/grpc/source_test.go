package grpc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
	"google.golang.org/grpc/codes"
)

// stubLoader implements the configLoader interface for tests.
type stubLoader struct {
	cfg       *config.Config
	configDir string
}

func (s *stubLoader) Get() *config.Config { return s.cfg }
func (s *stubLoader) ConfigDir() string   { return s.configDir }

// newTestHandler returns a handler wired to a temp config directory.
func newTestHandler(t *testing.T, configDir string) *handler {
	t.Helper()
	return &handler{
		loader:      &stubLoader{cfg: &config.Config{}, configDir: configDir},
		transitions: newGRPCTransitionState(),
	}
}

// readStub reads and parses a JSON stub file.
func readStub(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading stub: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing stub: %v", err)
	}
	return m
}

// writeStub writes a JSON stub file.
func writeStub(t *testing.T, path string, data map[string]interface{}) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating stub dir: %v", err)
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("marshalling stub: %v", err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("writing stub: %v", err)
	}
}

// initialCityStub returns the initial stub data shared across tests.
func initialCityStub() map[string]interface{} {
	return map[string]interface{}{
		"name":        "marrakech",
		"description": "Red city of Morocco",
		"elevation":   "466m",
		"population":  float64(928850),
		"area":        "230km2",
	}
}

// -----------------------------------------------------------------------
// Test 1: source extracts nested field before merge
// -----------------------------------------------------------------------

func TestApplyGRPCPersist_SourceExtractsNestedField(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "cities", "marrakech.json")
	writeStub(t, stubFile, initialCityStub())

	h := newTestHandler(t, dir)

	c := config.Case{
		Status:  0,
		File:    "stubs/cities/{body.city.name}.json",
		Persist: true,
		Merge:   "update",
		Source:  "city",
		Wrap:    "city",
	}

	reqMap := map[string]interface{}{
		"continentCode": "africa",
		"regionId":      "north-africa",
		"language":      "fr",
		"city": map[string]interface{}{
			"name":      "marrakech",
			"elevation": "470m",
		},
	}

	code, handled, result, _ := h.applyGRPCPersist(c, reqMap, time.Now(),
		"/geo.CityService/UpdateCity", nil)

	if !handled {
		t.Fatal("expected handled=true")
	}
	if code != codes.OK {
		t.Fatalf("expected OK, got %v", code)
	}

	// Response should be wrapped: {"city": {...}}
	inner, ok := result["city"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected wrapped response with city key, got %v", result)
	}
	if inner["elevation"] != "470m" {
		t.Errorf("expected elevation=470m in response, got %v", inner["elevation"])
	}

	// Verify persisted file.
	stub := readStub(t, stubFile)

	// elevation changed from 466m to 470m.
	if stub["elevation"] != "470m" {
		t.Errorf("expected elevation=470m, got %v", stub["elevation"])
	}

	// Metadata fields must NOT be in the file.
	for _, field := range []string{"continentCode", "regionId", "language", "continent_code", "region_id"} {
		if stub[field] != nil {
			t.Errorf("%s should not be in stub, got %v", field, stub[field])
		}
	}

	// No nested city key inside the file.
	if stub["city"] != nil {
		t.Error("city sub-object should not be in stub")
	}

	// Other fields unchanged.
	if stub["description"] != "Red city of Morocco" {
		t.Errorf("description changed: %v", stub["description"])
	}
	if stub["area"] != "230km2" {
		t.Errorf("area changed: %v", stub["area"])
	}
	if stub["population"] != float64(928850) {
		t.Errorf("population changed: %v", stub["population"])
	}
}

// -----------------------------------------------------------------------
// Test 2: Multiple fields updated at once
// -----------------------------------------------------------------------

func TestApplyGRPCPersist_SourceMultipleFieldsUpdated(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "cities", "marrakech.json")
	writeStub(t, stubFile, initialCityStub())

	h := newTestHandler(t, dir)

	c := config.Case{
		Status:  0,
		File:    "stubs/cities/{body.city.name}.json",
		Persist: true,
		Merge:   "update",
		Source:  "city",
		Wrap:    "city",
	}

	reqMap := map[string]interface{}{
		"continentCode": "africa",
		"regionId":      "north-africa",
		"language":      "ar",
		"city": map[string]interface{}{
			"name":        "marrakech",
			"elevation":   "466m",
			"population":  float64(1000000),
			"description": "Updated description",
		},
	}

	code, handled, _, _ := h.applyGRPCPersist(c, reqMap, time.Now(),
		"/geo.CityService/UpdateCity", nil)

	if !handled || code != codes.OK {
		t.Fatalf("expected handled=true, OK; got handled=%v, code=%v", handled, code)
	}

	stub := readStub(t, stubFile)

	// All three fields updated.
	if stub["elevation"] != "466m" {
		t.Errorf("expected elevation=466m, got %v", stub["elevation"])
	}
	if stub["population"] != float64(1000000) {
		t.Errorf("expected population=1000000, got %v", stub["population"])
	}
	if stub["description"] != "Updated description" {
		t.Errorf("expected description=Updated description, got %v", stub["description"])
	}

	// area unchanged (not in request).
	if stub["area"] != "230km2" {
		t.Errorf("area should be unchanged, got %v", stub["area"])
	}

	// No metadata fields leaked.
	for _, field := range []string{"continentCode", "regionId", "language"} {
		if stub[field] != nil {
			t.Errorf("%s should not be in stub", field)
		}
	}
}

// -----------------------------------------------------------------------
// Test 3: Without source — verifies old (broken) behavior
//
// This is an intentional regression test that asserts metadata DOES leak
// when source is omitted. If automatic source detection or default
// behavior changes in the future, this test will need updating.
// -----------------------------------------------------------------------

func TestApplyGRPCPersist_WithoutSourceLeaksMetadata(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "cities", "marrakech.json")
	writeStub(t, stubFile, initialCityStub())

	h := newTestHandler(t, dir)

	// Same config but WITHOUT source.
	c := config.Case{
		Status:  0,
		File:    "stubs/cities/{body.city.name}.json",
		Persist: true,
		Merge:   "update",
		Wrap:    "city",
	}

	reqMap := map[string]interface{}{
		"continentCode": "africa",
		"regionId":      "north-africa",
		"language":      "fr",
		"city": map[string]interface{}{
			"name":      "marrakech",
			"elevation": "470m",
		},
	}

	code, handled, _, _ := h.applyGRPCPersist(c, reqMap, time.Now(),
		"/geo.CityService/UpdateCityRaw", nil)

	if !handled || code != codes.OK {
		t.Fatalf("expected handled=true, OK; got handled=%v, code=%v", handled, code)
	}

	stub := readStub(t, stubFile)

	// Without source, metadata fields leak into the file.
	if stub["continentCode"] == nil {
		t.Error("without source, continentCode should leak into stub (broken behavior)")
	}
	if stub["regionId"] == nil {
		t.Error("without source, regionId should leak into stub (broken behavior)")
	}
	if stub["language"] == nil {
		t.Error("without source, language should leak into stub (broken behavior)")
	}

	// The nested city object is added as-is.
	if stub["city"] == nil {
		t.Error("without source, city sub-object should be in stub (broken behavior)")
	}

	// Original elevation is NOT updated (still 466m) because the top-level merge
	// doesn't reach into the nested city object.
	if stub["elevation"] != "466m" {
		t.Errorf("without source, elevation should remain 466m, got %v", stub["elevation"])
	}
}

// -----------------------------------------------------------------------
// Test 4: Deep nested source path (dot-path depth > 1)
// -----------------------------------------------------------------------

func TestApplyGRPCPersist_SourceDeepNestedPath(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "summits", "toubkal.json")
	writeStub(t, stubFile, map[string]interface{}{
		"id":       "toubkal",
		"name":     "Jbel Toubkal",
		"altitude": "4167m",
	})

	h := newTestHandler(t, dir)

	c := config.Case{
		Status:  0,
		File:    "stubs/summits/{body.terrain.summit.id}.json",
		Persist: true,
		Merge:   "update",
		Source:  "terrain.summit",
	}

	reqMap := map[string]interface{}{
		"metadata": "should-not-leak",
		"terrain": map[string]interface{}{
			"version": "v2",
			"summit": map[string]interface{}{
				"id":       "toubkal",
				"altitude": "4168m",
			},
		},
	}

	code, handled, result, _ := h.applyGRPCPersist(c, reqMap, time.Now(),
		"/geo.TerrainService/UpdateSummit", nil)

	if !handled || code != codes.OK {
		t.Fatalf("expected handled=true, OK; got handled=%v, code=%v", handled, code)
	}

	// Response should contain the merged result (no wrap configured).
	if result["altitude"] != "4168m" {
		t.Errorf("expected altitude=4168m in response, got %v", result["altitude"])
	}

	stub := readStub(t, stubFile)

	// Deep source extracted body.terrain.summit.
	if stub["altitude"] != "4168m" {
		t.Errorf("expected altitude=4168m, got %v", stub["altitude"])
	}
	if stub["name"] != "Jbel Toubkal" {
		t.Errorf("name should be unchanged, got %v", stub["name"])
	}

	// Metadata and intermediate fields must NOT leak.
	if stub["metadata"] != nil {
		t.Error("metadata should not be in stub")
	}
	if stub["terrain"] != nil {
		t.Error("terrain should not be in stub")
	}
	if stub["version"] != nil {
		t.Error("version should not be in stub")
	}
}
