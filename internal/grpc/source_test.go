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

// initialServiceStub returns the initial stub data shared across tests.
func initialServiceStub() map[string]interface{} {
	return map[string]interface{}{
		"name":        "my-service",
		"description": "Original description",
		"cpu":         "500m",
		"memory":      "256Mi",
		"replicas":    float64(3),
	}
}

// -----------------------------------------------------------------------
// Test 1: source extracts nested field before merge
// -----------------------------------------------------------------------

func TestApplyGRPCPersist_SourceExtractsNestedField(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "services", "my-service.json")
	writeStub(t, stubFile, initialServiceStub())

	h := newTestHandler(t, dir)

	c := config.Case{
		Status:  0,
		File:    "stubs/services/{body.service.name}.json",
		Persist: true,
		Merge:   "update",
		Source:  "service",
		Wrap:    "service",
	}

	reqMap := map[string]interface{}{
		"orgId":         "org-123",
		"providerId":    "GCP",
		"environmentId": "env-dev",
		"service": map[string]interface{}{
			"name": "my-service",
			"cpu":  "1000m",
		},
	}

	code, handled, result := h.applyGRPCPersist(c, reqMap, time.Now(),
		"/service.v1.ServiceService/UpdateService", nil)

	if !handled {
		t.Fatal("expected handled=true")
	}
	if code != codes.OK {
		t.Fatalf("expected OK, got %v", code)
	}

	// Response should be wrapped: {"service": {...}}
	inner, ok := result["service"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected wrapped response with service key, got %v", result)
	}
	if inner["cpu"] != "1000m" {
		t.Errorf("expected cpu=1000m in response, got %v", inner["cpu"])
	}

	// Verify persisted file.
	stub := readStub(t, stubFile)

	// cpu changed from 500m to 1000m.
	if stub["cpu"] != "1000m" {
		t.Errorf("expected cpu=1000m, got %v", stub["cpu"])
	}

	// Metadata fields must NOT be in the file.
	for _, field := range []string{"orgId", "providerId", "environmentId", "org_id", "provider_id", "environment_id"} {
		if stub[field] != nil {
			t.Errorf("%s should not be in stub, got %v", field, stub[field])
		}
	}

	// No nested service key inside the file.
	if stub["service"] != nil {
		t.Error("service sub-object should not be in stub")
	}

	// Other fields unchanged.
	if stub["description"] != "Original description" {
		t.Errorf("description changed: %v", stub["description"])
	}
	if stub["memory"] != "256Mi" {
		t.Errorf("memory changed: %v", stub["memory"])
	}
	if stub["replicas"] != float64(3) {
		t.Errorf("replicas changed: %v", stub["replicas"])
	}
}

// -----------------------------------------------------------------------
// Test 2: Multiple fields updated at once
// -----------------------------------------------------------------------

func TestApplyGRPCPersist_SourceMultipleFieldsUpdated(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "services", "my-service.json")
	writeStub(t, stubFile, initialServiceStub())

	h := newTestHandler(t, dir)

	c := config.Case{
		Status:  0,
		File:    "stubs/services/{body.service.name}.json",
		Persist: true,
		Merge:   "update",
		Source:  "service",
		Wrap:    "service",
	}

	reqMap := map[string]interface{}{
		"orgId":         "org-123",
		"providerId":    "GCP",
		"environmentId": "env-dev",
		"service": map[string]interface{}{
			"name":        "my-service",
			"cpu":         "2000m",
			"memory":      "512Mi",
			"description": "Updated description",
		},
	}

	code, handled, _ := h.applyGRPCPersist(c, reqMap, time.Now(),
		"/service.v1.ServiceService/UpdateService", nil)

	if !handled || code != codes.OK {
		t.Fatalf("expected handled=true, OK; got handled=%v, code=%v", handled, code)
	}

	stub := readStub(t, stubFile)

	// All three fields updated.
	if stub["cpu"] != "2000m" {
		t.Errorf("expected cpu=2000m, got %v", stub["cpu"])
	}
	if stub["memory"] != "512Mi" {
		t.Errorf("expected memory=512Mi, got %v", stub["memory"])
	}
	if stub["description"] != "Updated description" {
		t.Errorf("expected description=Updated description, got %v", stub["description"])
	}

	// replicas unchanged (not in request).
	if stub["replicas"] != float64(3) {
		t.Errorf("replicas should be unchanged, got %v", stub["replicas"])
	}

	// No metadata fields leaked.
	for _, field := range []string{"orgId", "providerId", "environmentId"} {
		if stub[field] != nil {
			t.Errorf("%s should not be in stub", field)
		}
	}
}

// -----------------------------------------------------------------------
// Test 3: Without source — verifies old (broken) behavior
// -----------------------------------------------------------------------

func TestApplyGRPCPersist_WithoutSourceLeaksMetadata(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "services", "my-service.json")
	writeStub(t, stubFile, initialServiceStub())

	h := newTestHandler(t, dir)

	// Same config but WITHOUT source.
	c := config.Case{
		Status:  0,
		File:    "stubs/services/{body.service.name}.json",
		Persist: true,
		Merge:   "update",
		Wrap:    "service",
	}

	reqMap := map[string]interface{}{
		"orgId":         "org-123",
		"providerId":    "GCP",
		"environmentId": "env-dev",
		"service": map[string]interface{}{
			"name": "my-service",
			"cpu":  "1000m",
		},
	}

	code, handled, _ := h.applyGRPCPersist(c, reqMap, time.Now(),
		"/service.v1.ServiceService/UpdateService", nil)

	if !handled || code != codes.OK {
		t.Fatalf("expected handled=true, OK; got handled=%v, code=%v", handled, code)
	}

	stub := readStub(t, stubFile)

	// Without source, metadata fields leak into the file.
	if stub["orgId"] == nil {
		t.Error("without source, orgId should leak into stub (broken behavior)")
	}
	if stub["providerId"] == nil {
		t.Error("without source, providerId should leak into stub (broken behavior)")
	}
	if stub["environmentId"] == nil {
		t.Error("without source, environmentId should leak into stub (broken behavior)")
	}

	// The nested service object is added as-is.
	if stub["service"] == nil {
		t.Error("without source, service sub-object should be in stub (broken behavior)")
	}

	// Original cpu is NOT updated (still 500m) because the top-level merge
	// doesn't reach into the nested service object.
	if stub["cpu"] != "500m" {
		t.Errorf("without source, cpu should remain 500m, got %v", stub["cpu"])
	}
}

// -----------------------------------------------------------------------
// Test 4: Deep nested source path (dot-path depth > 1)
// -----------------------------------------------------------------------

func TestApplyGRPCPersist_SourceDeepNestedPath(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "stubs", "endpoints", "ep-42.json")
	writeStub(t, stubFile, map[string]interface{}{
		"id":          "ep-42",
		"name":        "my-endpoint",
		"description": "Original",
	})

	h := newTestHandler(t, dir)

	c := config.Case{
		Status:  0,
		File:    "stubs/endpoints/{body.config.endpoint.id}.json",
		Persist: true,
		Merge:   "update",
		Source:  "config.endpoint",
	}

	reqMap := map[string]interface{}{
		"metadata": "should-not-leak",
		"config": map[string]interface{}{
			"version": "v2",
			"endpoint": map[string]interface{}{
				"id":          "ep-42",
				"description": "Updated",
			},
		},
	}

	code, handled, result := h.applyGRPCPersist(c, reqMap, time.Now(),
		"/config.v1.ConfigService/UpdateEndpoint", nil)

	if !handled || code != codes.OK {
		t.Fatalf("expected handled=true, OK; got handled=%v, code=%v", handled, code)
	}

	// Response should contain the merged result (no wrap configured).
	if result["description"] != "Updated" {
		t.Errorf("expected description=Updated in response, got %v", result["description"])
	}

	stub := readStub(t, stubFile)

	// Deep source extracted body.config.endpoint.
	if stub["description"] != "Updated" {
		t.Errorf("expected description=Updated, got %v", stub["description"])
	}
	if stub["name"] != "my-endpoint" {
		t.Errorf("name should be unchanged, got %v", stub["name"])
	}

	// Metadata and intermediate fields must NOT leak.
	if stub["metadata"] != nil {
		t.Error("metadata should not be in stub")
	}
	if stub["config"] != nil {
		t.Error("config should not be in stub")
	}
	if stub["version"] != nil {
		t.Error("version should not be in stub")
	}
}
