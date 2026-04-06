package proxy

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRefContext(t *testing.T) {
	// Create a test request
	req, err := http.NewRequest("POST", "https://example.com/endpoints/prod?version=v1", strings.NewReader(`{"endpointId":"test","config":{"env":"staging"}}`))
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}
	req.Header.Set("X-Tenant-Id", "tenant123")

	bodyBytes := []byte(`{"endpointId":"test","config":{"env":"staging"}}`)
	pathParams := map[string]string{"id": "prod"}

	ctx := NewRefContext(req, bodyBytes, pathParams)

	// Check body parsing
	if ctx.Body["endpointId"] != "test" {
		t.Errorf("expected endpointId=test, got %v", ctx.Body["endpointId"])
	}

	// Check nested body parsing
	config, ok := ctx.Body["config"].(map[string]interface{})
	if !ok || config["env"] != "staging" {
		t.Errorf("expected config.env=staging, got %v", config)
	}

	// Check path params
	if ctx.PathParams["id"] != "prod" {
		t.Errorf("expected path param id=prod, got %v", ctx.PathParams["id"])
	}

	// Check query params
	if ctx.Query.Get("version") != "v1" {
		t.Errorf("expected query version=v1, got %v", ctx.Query.Get("version"))
	}

	// Check headers
	if ctx.Headers.Get("X-Tenant-Id") != "tenant123" {
		t.Errorf("expected header X-Tenant-Id=tenant123, got %v", ctx.Headers.Get("X-Tenant-Id"))
	}
}

func TestNewRefContext_HeaderSanitization(t *testing.T) {
	// Create request with both safe and sensitive headers
	req, err := http.NewRequest("POST", "/test", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}
	req.Header.Set("X-Tenant-Id", "safe-header")
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Cookie", "session=secret-session")
	req.Header.Set("Proxy-Authorization", "Basic secret-proxy")
	req.Header.Set("User-Agent", "apitwin-test")

	ctx := NewRefContext(req, []byte(`{}`), nil)

	// Safe headers should be preserved
	if ctx.Headers.Get("X-Tenant-Id") != "safe-header" {
		t.Errorf("expected X-Tenant-Id=safe-header, got %v", ctx.Headers.Get("X-Tenant-Id"))
	}
	if ctx.Headers.Get("User-Agent") != "apitwin-test" {
		t.Errorf("expected User-Agent=apitwin-test, got %v", ctx.Headers.Get("User-Agent"))
	}

	// Sensitive headers should be stripped
	if ctx.Headers.Get("Authorization") != "" {
		t.Errorf("expected Authorization to be stripped, got %v", ctx.Headers.Get("Authorization"))
	}
	if ctx.Headers.Get("Cookie") != "" {
		t.Errorf("expected Cookie to be stripped, got %v", ctx.Headers.Get("Cookie"))
	}
	if ctx.Headers.Get("Proxy-Authorization") != "" {
		t.Errorf("expected Proxy-Authorization to be stripped, got %v", ctx.Headers.Get("Proxy-Authorization"))
	}
}

func TestResolveDynamicInRefs_BodyField(t *testing.T) {
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{"endpointId":"prod"}`))
	ctx := NewRefContext(req, []byte(`{"endpointId":"prod"}`), nil)

	content := []byte(`{"data": "{{ref:stubs/{.endpointId}/models/}}"}`)
	result, err := resolveDynamicInRefs(content, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := `{"data": "{{ref:stubs/prod/models/}}"}`
	if string(result) != expected {
		t.Errorf("expected %s, got %s", expected, string(result))
	}
}

func TestResolveDynamicInRefs_NestedBodyField(t *testing.T) {
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{"config":{"env":"staging"}}`))
	ctx := NewRefContext(req, []byte(`{"config":{"env":"staging"}}`), nil)

	content := []byte(`{"data": "{{ref:stubs/{.config.env}/models/}}"}`)
	result, err := resolveDynamicInRefs(content, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := `{"data": "{{ref:stubs/staging/models/}}"}`
	if string(result) != expected {
		t.Errorf("expected %s, got %s", expected, string(result))
	}
}

func TestResolveDynamicInRefs_PathParam(t *testing.T) {
	req, _ := http.NewRequest("GET", "/test", nil)
	pathParams := map[string]string{"endpointId": "prod"}
	ctx := NewRefContext(req, []byte(`{}`), pathParams)

	content := []byte(`{"data": "{{ref:stubs/{path.endpointId}/models/}}"}`)
	result, err := resolveDynamicInRefs(content, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := `{"data": "{{ref:stubs/prod/models/}}"}`
	if string(result) != expected {
		t.Errorf("expected %s, got %s", expected, string(result))
	}
}

func TestResolveDynamicInRefs_QueryParam(t *testing.T) {
	req, _ := http.NewRequest("GET", "/test?version=v2", nil)
	ctx := NewRefContext(req, []byte(`{}`), nil)

	content := []byte(`{"data": "{{ref:stubs/{query.version}/models/}}"}`)
	result, err := resolveDynamicInRefs(content, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := `{"data": "{{ref:stubs/v2/models/}}"}`
	if string(result) != expected {
		t.Errorf("expected %s, got %s", expected, string(result))
	}
}

func TestResolveDynamicInRefs_Header(t *testing.T) {
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Tenant-Id", "tenant123")
	ctx := NewRefContext(req, []byte(`{}`), nil)

	content := []byte(`{"data": "{{ref:stubs/{header.X-Tenant-Id}/models/}}"}`)
	result, err := resolveDynamicInRefs(content, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := `{"data": "{{ref:stubs/tenant123/models/}}"}`
	if string(result) != expected {
		t.Errorf("expected %s, got %s", expected, string(result))
	}
}

func TestResolveDynamicInRefs_MissingField_Error(t *testing.T) {
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{}`))
	ctx := NewRefContext(req, []byte(`{}`), nil)

	content := []byte(`{"data": "{{ref:stubs/{.endpointId}/models/}}"}`)
	_, err := resolveDynamicInRefs(content, ctx)
	if err == nil {
		t.Fatalf("expected error for missing field")
	}
	if !strings.Contains(err.Error(), "field \"endpointId\" not found") {
		t.Errorf("error should mention missing field, got: %v", err)
	}
}

func TestResolveDynamicInRefs_EmptyValue_Error(t *testing.T) {
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{"endpointId":""}`))
	ctx := NewRefContext(req, []byte(`{"endpointId":""}`), nil)

	content := []byte(`{"data": "{{ref:stubs/{.endpointId}/models/}}"}`)
	_, err := resolveDynamicInRefs(content, ctx)
	if err == nil {
		t.Fatalf("expected error for empty value")
	}
	if !strings.Contains(err.Error(), "resolved to empty string") {
		t.Errorf("error should mention empty string, got: %v", err)
	}
}

func TestResolveDynamicInRefs_InvalidChars_Error(t *testing.T) {
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{"endpointId":"prod?filter=status:active"}`))
	ctx := NewRefContext(req, []byte(`{"endpointId":"prod?filter=status:active"}`), nil)

	content := []byte(`{"data": "{{ref:stubs/{.endpointId}/models/}}"}`)
	_, err := resolveDynamicInRefs(content, ctx)
	if err == nil {
		t.Fatalf("expected error for value with reserved characters")
	}
	if !strings.Contains(err.Error(), "reserved characters") {
		t.Errorf("error should mention reserved characters, got: %v", err)
	}
}

func TestResolveDynamicInRefs_NilContext_Error(t *testing.T) {
	content := []byte(`{"data": "{{ref:stubs/{.endpointId}/models/}}"}`)
	_, err := resolveDynamicInRefs(content, nil)
	if err == nil {
		t.Fatalf("expected error for nil context with placeholders")
	}
	if !strings.Contains(err.Error(), "no request context available") {
		t.Errorf("error should mention no request context, got: %v", err)
	}
}

func TestResolveDynamicInRefs_NilContext_NoPlaceholders_Success(t *testing.T) {
	content := []byte(`{"data": "{{ref:stubs/prod/models/}}"}`)
	result, err := resolveDynamicInRefs(content, nil)
	if err != nil {
		t.Fatalf("unexpected error for nil context without placeholders: %v", err)
	}
	expected := `{"data": "{{ref:stubs/prod/models/}}"}`
	if string(result) != expected {
		t.Errorf("expected %s, got %s", expected, string(result))
	}
}

func TestResolveDynamicInRefs_MultiplePlaceholders(t *testing.T) {
	req, _ := http.NewRequest("POST", "/endpoints/prod?version=v1", strings.NewReader(`{"tenantId":"acme"}`))
	pathParams := map[string]string{"id": "prod"}
	ctx := NewRefContext(req, []byte(`{"tenantId":"acme"}`), pathParams)

	content := []byte(`{"data": "{{ref:stubs/{.tenantId}/{path.id}/{query.version}/models/}}"}`)
	result, err := resolveDynamicInRefs(content, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := `{"data": "{{ref:stubs/acme/prod/v1/models/}}"}`
	if string(result) != expected {
		t.Errorf("expected %s, got %s", expected, string(result))
	}
}

func TestResolveRefsWithContext_Integration(t *testing.T) {
	tempDir := t.TempDir()

	// Create directory structure with subdirs based on dynamic values
	modelsDir := filepath.Join(tempDir, "models", "prod")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatalf("creating models directory: %v", err)
	}

	// Create model file
	modelFile := filepath.Join(modelsDir, "gpt4.json")
	if err := os.WriteFile(modelFile, []byte(`{"id":"gpt4","name":"GPT-4"}`), 0644); err != nil {
		t.Fatalf("writing model file: %v", err)
	}

	// Create request context
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{"env":"prod"}`))
	ctx := NewRefContext(req, []byte(`{"env":"prod"}`), nil)

	// Test dynamic ref resolution
	content := []byte(`{"models": "{{ref:models/{.env}/}}"}`)
	result, err := resolveRefsWithContext(content, tempDir, make(map[string]bool), ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	models, ok := parsed["models"].([]interface{})
	if !ok {
		t.Fatalf("models should be an array")
	}
	if len(models) != 1 {
		t.Errorf("expected 1 model, got %d", len(models))
	}

	model := models[0].(map[string]interface{})
	if model["name"] != "GPT-4" {
		t.Errorf("expected model name GPT-4, got %v", model["name"])
	}
}

func TestLoadDefaults_WithStaticRef(t *testing.T) {
	tempDir := t.TempDir()

	// Create models directory
	modelsDir := filepath.Join(tempDir, "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatalf("creating models directory: %v", err)
	}

	// Create model file
	modelFile := filepath.Join(modelsDir, "1.json")
	if err := os.WriteFile(modelFile, []byte(`{"id":"1","name":"Test Model"}`), 0644); err != nil {
		t.Fatalf("writing model file: %v", err)
	}

	// Create defaults file with static ref
	defaultsFile := filepath.Join(tempDir, "defaults.json")
	defaultsContent := `{"defaultField":"value","models":"{{ref:models/}}"}`
	if err := os.WriteFile(defaultsFile, []byte(defaultsContent), 0644); err != nil {
		t.Fatalf("writing defaults file: %v", err)
	}

	// Create request
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{"userField":"userValue"}`))
	bodyBytes := []byte(`{"userField":"userValue"}`)
	incoming := map[string]interface{}{"userField": "userValue"}

	// Test loadDefaults with static ref
	result, err := loadDefaults("defaults.json", incoming, req, bodyBytes, tempDir, "/test", nil, make(map[string]bool))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that defaults were merged
	if result["defaultField"] != "value" {
		t.Errorf("expected defaultField=value, got %v", result["defaultField"])
	}

	// Check that user field wins over defaults
	if result["userField"] != "userValue" {
		t.Errorf("expected userField=userValue, got %v", result["userField"])
	}

	// Directory refs are preserved as live references in defaults so they
	// resolve dynamically on each read (not baked in at creation time).
	modelsRef, ok := result["models"].(string)
	if !ok {
		t.Fatalf("models should be a string (preserved ref token), got %T", result["models"])
	}
	if !strings.Contains(modelsRef, "{{ref:") {
		t.Errorf("expected models to contain a live ref token, got %v", modelsRef)
	}
}

func TestLoadDefaults_WithDynamicRef(t *testing.T) {
	tempDir := t.TempDir()

	// Create environment-specific models directory
	modelsDir := filepath.Join(tempDir, "models", "staging")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatalf("creating models directory: %v", err)
	}

	// Create model file
	modelFile := filepath.Join(modelsDir, "1.json")
	if err := os.WriteFile(modelFile, []byte(`{"id":"1","name":"Staging Model"}`), 0644); err != nil {
		t.Fatalf("writing model file: %v", err)
	}

	// Create defaults file with dynamic ref
	defaultsFile := filepath.Join(tempDir, "defaults.json")
	defaultsContent := `{"environment":"{.env}","models":"{{ref:models/{.env}/}}"}`
	if err := os.WriteFile(defaultsFile, []byte(defaultsContent), 0644); err != nil {
		t.Fatalf("writing defaults file: %v", err)
	}

	// Create request with env in body
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{"env":"staging","userField":"userValue"}`))
	bodyBytes := []byte(`{"env":"staging","userField":"userValue"}`)
	incoming := map[string]interface{}{"env": "staging", "userField": "userValue"}

	// Test loadDefaults with dynamic ref
	result, err := loadDefaults("defaults.json", incoming, req, bodyBytes, tempDir, "/test", nil, make(map[string]bool))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that template placeholder was resolved
	if result["environment"] != "staging" {
		t.Errorf("expected environment=staging, got %v", result["environment"])
	}

	// Directory refs are preserved with dynamic placeholders resolved but the
	// ref token itself intact so it resolves dynamically on each read.
	modelsRef, ok := result["models"].(string)
	if !ok {
		t.Fatalf("models should be a string (preserved ref token), got %T", result["models"])
	}
	// Dynamic placeholder {.env} should be resolved to "staging"
	if !strings.Contains(modelsRef, "models/staging/") {
		t.Errorf("expected dynamic placeholder to be resolved in ref, got %v", modelsRef)
	}
	// But the ref itself should still be a raw token
	if !strings.Contains(modelsRef, "{{ref:") {
		t.Errorf("expected ref token to be preserved, got %v", modelsRef)
	}
}

func TestLoadDefaults_RefError_Propagates(t *testing.T) {
	tempDir := t.TempDir()

	// Create defaults file with ref to non-existent file (not directory)
	defaultsFile := filepath.Join(tempDir, "defaults.json")
	defaultsContent := `{"models":"{{ref:nonexistent.json}}"}`
	if err := os.WriteFile(defaultsFile, []byte(defaultsContent), 0644); err != nil {
		t.Fatalf("writing defaults file: %v", err)
	}

	// Create request
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{}`))
	bodyBytes := []byte(`{}`)
	incoming := map[string]interface{}{}

	// Test that ref error propagates
	_, err := loadDefaults("defaults.json", incoming, req, bodyBytes, tempDir, "/test", nil, make(map[string]bool))
	if err == nil {
		t.Fatalf("expected error for non-existent ref")
	}
	if !strings.Contains(err.Error(), "resolving refs") {
		t.Errorf("error should mention ref resolution, got: %v", err)
	}
}

func TestLoadDefaultsStatic_WithStoredContext(t *testing.T) {
	tempDir := t.TempDir()

	// Create tenant-specific models directory
	modelsDir := filepath.Join(tempDir, "models", "tenant123")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatalf("creating models directory: %v", err)
	}

	// Create model file
	modelFile := filepath.Join(modelsDir, "1.json")
	if err := os.WriteFile(modelFile, []byte(`{"id":"1","name":"Tenant Model"}`), 0644); err != nil {
		t.Fatalf("writing model file: %v", err)
	}

	// Create defaults file with dynamic ref using header
	defaultsFile := filepath.Join(tempDir, "defaults.json")
	defaultsContent := `{"tenant":"{header.X-Tenant-Id}","models":"{{ref:models/{header.X-Tenant-Id}/}}"}`
	if err := os.WriteFile(defaultsFile, []byte(defaultsContent), 0644); err != nil {
		t.Fatalf("writing defaults file: %v", err)
	}

	// Create RefContext as would be stored by scheduler
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(`{}`))
	req.Header.Set("X-Tenant-Id", "tenant123")
	refCtx := NewRefContext(req, []byte(`{}`), nil)

	// Test loadDefaultsStatic with stored context
	result, err := loadDefaultsStatic("defaults.json", tempDir, make(map[string]bool), refCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that template placeholder was resolved
	if result["tenant"] != "tenant123" {
		t.Errorf("expected tenant=tenant123, got %v", result["tenant"])
	}

	// Check that dynamic ref was resolved using stored context
	models, ok := result["models"].([]interface{})
	if !ok {
		t.Fatalf("models should be an array")
	}
	if len(models) != 1 {
		t.Errorf("expected 1 model, got %d", len(models))
	}

	model := models[0].(map[string]interface{})
	if model["name"] != "Tenant Model" {
		t.Errorf("expected model name 'Tenant Model', got %v", model["name"])
	}
}

func TestCircularDetection_AcrossDefaultsAndStubs(t *testing.T) {
	tempDir := t.TempDir()

	// Create files that reference each other in a circle
	defaultsFile := filepath.Join(tempDir, "defaults.json")
	defaultsContent := `{"ref":"{{ref:stub.json}}"}`
	if err := os.WriteFile(defaultsFile, []byte(defaultsContent), 0644); err != nil {
		t.Fatalf("writing defaults file: %v", err)
	}

	stubFile := filepath.Join(tempDir, "stub.json")
	stubContent := `{"ref":"{{ref:defaults.json}}"}`
	if err := os.WriteFile(stubFile, []byte(stubContent), 0644); err != nil {
		t.Fatalf("writing stub file: %v", err)
	}

	// Create shared visited map to track across stub + defaults
	visited := make(map[string]bool)

	// Simulate resolving refs in stub file first
	_, err := resolveRefsWithContext([]byte(stubContent), tempDir, visited, nil)
	if err == nil {
		t.Fatalf("expected circular reference error")
	}
	if !strings.Contains(err.Error(), "circular reference") {
		t.Errorf("error should mention circular reference, got: %v", err)
	}
}
