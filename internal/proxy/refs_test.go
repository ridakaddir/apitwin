package proxy

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRefs_NoRefs(t *testing.T) {
	content := []byte(`{"id": "123", "name": "test"}`)
	result, err := resolveRefs(content, "", make(map[string]bool))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != string(content) {
		t.Errorf("content should remain unchanged when no refs present")
	}
}

func TestResolveRefs_Directory(t *testing.T) {
	// Setup temporary directory with test files
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("creating test directory: %v", err)
	}

	// Create test files
	file1 := filepath.Join(dataDir, "1.json")
	if err := os.WriteFile(file1, []byte(`{"id": "1", "name": "Alice"}`), 0644); err != nil {
		t.Fatalf("writing test file 1: %v", err)
	}
	file2 := filepath.Join(dataDir, "2.json")
	if err := os.WriteFile(file2, []byte(`{"id": "2", "name": "Bob"}`), 0644); err != nil {
		t.Fatalf("writing test file 2: %v", err)
	}

	// Test directory reference
	content := []byte(`{"users": "{{ref:data/}}"}`)
	result, err := resolveRefs(content, tempDir, make(map[string]bool))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	users, ok := parsed["users"].([]interface{})
	if !ok {
		t.Fatalf("users should be an array")
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestResolveRefs_SingleFile(t *testing.T) {
	// Setup temporary file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "user.json")
	if err := os.WriteFile(testFile, []byte(`{"id": "1", "name": "Alice"}`), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	// Test single file reference
	content := []byte(`{"user": "{{ref:user.json}}"}`)
	result, err := resolveRefs(content, tempDir, make(map[string]bool))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	user, ok := parsed["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("user should be an object")
	}
	if user["id"] != "1" || user["name"] != "Alice" {
		t.Errorf("unexpected user data: %v", user)
	}
}

func TestResolveRefs_WithFilter(t *testing.T) {
	// Setup temporary directory with test files
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "users")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("creating test directory: %v", err)
	}

	// Create test files with different statuses
	file1 := filepath.Join(dataDir, "1.json")
	if err := os.WriteFile(file1, []byte(`{"id": "1", "name": "Alice", "status": "active"}`), 0644); err != nil {
		t.Fatalf("writing test file 1: %v", err)
	}
	file2 := filepath.Join(dataDir, "2.json")
	if err := os.WriteFile(file2, []byte(`{"id": "2", "name": "Bob", "status": "inactive"}`), 0644); err != nil {
		t.Fatalf("writing test file 2: %v", err)
	}
	file3 := filepath.Join(dataDir, "3.json")
	if err := os.WriteFile(file3, []byte(`{"id": "3", "name": "Charlie", "status": "active"}`), 0644); err != nil {
		t.Fatalf("writing test file 3: %v", err)
	}

	// Test filter reference
	content := []byte(`{"activeUsers": "{{ref:users/?filter=status:active}}"}`)
	result, err := resolveRefs(content, tempDir, make(map[string]bool))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	users, ok := parsed["activeUsers"].([]interface{})
	if !ok {
		t.Fatalf("activeUsers should be an array")
	}
	if len(users) != 2 {
		t.Errorf("expected 2 active users, got %d", len(users))
	}

	// Check that both active users are present
	names := make(map[string]bool)
	for _, user := range users {
		u := user.(map[string]interface{})
		names[u["name"].(string)] = true
		if u["status"] != "active" {
			t.Errorf("filtered user should have status 'active', got %v", u["status"])
		}
	}
	if !names["Alice"] || !names["Charlie"] {
		t.Errorf("expected Alice and Charlie, got users: %v", names)
	}
}

func TestResolveRefs_WithNestedFilter(t *testing.T) {
	// Setup temporary directory with test files containing nested objects
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "users")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("creating test directory: %v", err)
	}

	// Create test files with nested user data
	file1 := filepath.Join(dataDir, "1.json")
	if err := os.WriteFile(file1, []byte(`{"id": "1", "name": "Alice", "user": {"role": "admin"}}`), 0644); err != nil {
		t.Fatalf("writing test file 1: %v", err)
	}
	file2 := filepath.Join(dataDir, "2.json")
	if err := os.WriteFile(file2, []byte(`{"id": "2", "name": "Bob", "user": {"role": "user"}}`), 0644); err != nil {
		t.Fatalf("writing test file 2: %v", err)
	}

	// Test nested filter reference
	content := []byte(`{"admins": "{{ref:users/?filter=user.role:admin}}"}`)
	result, err := resolveRefs(content, tempDir, make(map[string]bool))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	admins, ok := parsed["admins"].([]interface{})
	if !ok {
		t.Fatalf("admins should be an array")
	}
	if len(admins) != 1 {
		t.Errorf("expected 1 admin, got %d", len(admins))
	}

	admin := admins[0].(map[string]interface{})
	if admin["name"] != "Alice" {
		t.Errorf("expected Alice, got %v", admin["name"])
	}
}

func TestResolveRefs_WithTemplate(t *testing.T) {
	// Setup temporary directories
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "users")
	templatesDir := filepath.Join(tempDir, "templates")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("creating data directory: %v", err)
	}
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("creating templates directory: %v", err)
	}

	// Create test files
	file1 := filepath.Join(dataDir, "1.json")
	if err := os.WriteFile(file1, []byte(`{"id": "1", "name": "Alice", "email": "alice@example.com"}`), 0644); err != nil {
		t.Fatalf("writing test file 1: %v", err)
	}
	file2 := filepath.Join(dataDir, "2.json")
	if err := os.WriteFile(file2, []byte(`{"id": "2", "name": "Bob", "email": "bob@example.com"}`), 0644); err != nil {
		t.Fatalf("writing test file 2: %v", err)
	}

	// Create template file
	templateFile := filepath.Join(templatesDir, "user-summary.json")
	templateContent := `{"userId": "{{.id}}", "displayName": "{{.name}}"}`
	if err := os.WriteFile(templateFile, []byte(templateContent), 0644); err != nil {
		t.Fatalf("writing template file: %v", err)
	}

	// Test template reference
	content := []byte(`{"users": "{{ref:users/?template=templates/user-summary.json}}"}`)
	result, err := resolveRefs(content, tempDir, make(map[string]bool))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	users, ok := parsed["users"].([]interface{})
	if !ok {
		t.Fatalf("users should be an array")
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}

	// Check transformed structure
	for _, user := range users {
		u := user.(map[string]interface{})
		if _, hasUserId := u["userId"]; !hasUserId {
			t.Errorf("user should have userId field after template transformation")
		}
		if _, hasDisplayName := u["displayName"]; !hasDisplayName {
			t.Errorf("user should have displayName field after template transformation")
		}
		if _, hasEmail := u["email"]; hasEmail {
			t.Errorf("user should not have email field after template transformation")
		}
	}
}

func TestResolveRefs_FilterAndTemplate(t *testing.T) {
	// Setup temporary directories
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "users")
	templatesDir := filepath.Join(tempDir, "templates")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("creating data directory: %v", err)
	}
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("creating templates directory: %v", err)
	}

	// Create test files
	file1 := filepath.Join(dataDir, "1.json")
	if err := os.WriteFile(file1, []byte(`{"id": "1", "name": "Alice", "status": "active"}`), 0644); err != nil {
		t.Fatalf("writing test file 1: %v", err)
	}
	file2 := filepath.Join(dataDir, "2.json")
	if err := os.WriteFile(file2, []byte(`{"id": "2", "name": "Bob", "status": "inactive"}`), 0644); err != nil {
		t.Fatalf("writing test file 2: %v", err)
	}

	// Create template file
	templateFile := filepath.Join(templatesDir, "active-user.json")
	templateContent := `{"userId": "{{.id}}", "userName": "{{.name}}"}`
	if err := os.WriteFile(templateFile, []byte(templateContent), 0644); err != nil {
		t.Fatalf("writing template file: %v", err)
	}

	// Test filter + template reference
	content := []byte(`{"activeUsers": "{{ref:users/?filter=status:active&template=templates/active-user.json}}"}`)
	result, err := resolveRefs(content, tempDir, make(map[string]bool))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	users, ok := parsed["activeUsers"].([]interface{})
	if !ok {
		t.Fatalf("activeUsers should be an array")
	}
	if len(users) != 1 {
		t.Errorf("expected 1 active user after filter, got %d", len(users))
	}

	// Check filtered and transformed user
	user := users[0].(map[string]interface{})
	if user["userId"] != "1" || user["userName"] != "Alice" {
		t.Errorf("unexpected user data after filter+template: %v", user)
	}
	if _, hasStatus := user["status"]; hasStatus {
		t.Errorf("user should not have status field after template transformation")
	}
}

func TestResolveRefs_EmptyFilterResult(t *testing.T) {
	// Setup temporary directory with test files
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "users")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("creating test directory: %v", err)
	}

	// Create test files (none matching the filter)
	file1 := filepath.Join(dataDir, "1.json")
	if err := os.WriteFile(file1, []byte(`{"id": "1", "name": "Alice", "status": "inactive"}`), 0644); err != nil {
		t.Fatalf("writing test file 1: %v", err)
	}

	// Test filter that matches nothing
	content := []byte(`{"activeUsers": "{{ref:users/?filter=status:active}}"}`)
	result, err := resolveRefs(content, tempDir, make(map[string]bool))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	users, ok := parsed["activeUsers"].([]interface{})
	if !ok {
		t.Fatalf("activeUsers should be an array")
	}
	if len(users) != 0 {
		t.Errorf("expected empty array when filter matches nothing, got %d items", len(users))
	}
}

func TestResolveRefs_MissingFile(t *testing.T) {
	tempDir := t.TempDir()
	content := []byte(`{"data": "{{ref:nonexistent.json}}"}`)

	_, err := resolveRefs(content, tempDir, make(map[string]bool))
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "nonexistent.json") {
		t.Errorf("error should mention missing file, got: %v", err)
	}
}

func TestResolveRefs_CircularDetection(t *testing.T) {
	// Setup temporary files with circular references
	tempDir := t.TempDir()

	file1 := filepath.Join(tempDir, "1.json")
	if err := os.WriteFile(file1, []byte(`{"ref": "{{ref:2.json}}"}`), 0644); err != nil {
		t.Fatalf("writing file 1: %v", err)
	}
	file2 := filepath.Join(tempDir, "2.json")
	if err := os.WriteFile(file2, []byte(`{"ref": "{{ref:1.json}}"}`), 0644); err != nil {
		t.Fatalf("writing file 2: %v", err)
	}

	content := []byte(`{"data": "{{ref:1.json}}"}`)

	_, err := resolveRefs(content, tempDir, make(map[string]bool))
	if err == nil {
		t.Fatalf("expected error for circular reference")
	}
	if !strings.Contains(err.Error(), "circular reference") {
		t.Errorf("error should mention circular reference, got: %v", err)
	}
}

func TestParseRefToken_Simple(t *testing.T) {
	path, filters, templatePath, err := parseRefToken("path/to/dir/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "path/to/dir/" {
		t.Errorf("expected path 'path/to/dir/', got '%s'", path)
	}
	if len(filters) != 0 {
		t.Errorf("expected no filters, got %d", len(filters))
	}
	if templatePath != "" {
		t.Errorf("expected no template path, got '%s'", templatePath)
	}
}

func TestParseRefToken_WithFilter(t *testing.T) {
	path, filters, templatePath, err := parseRefToken("path/?filter=status:active")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "path/" {
		t.Errorf("expected path 'path/', got '%s'", path)
	}
	if len(filters) != 1 || filters["status"] != "active" {
		t.Errorf("expected filter status:active, got %v", filters)
	}
	if templatePath != "" {
		t.Errorf("expected no template path, got '%s'", templatePath)
	}
}

func TestParseRefToken_WithTemplate(t *testing.T) {
	path, filters, templatePath, err := parseRefToken("path/?template=tpl.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "path/" {
		t.Errorf("expected path 'path/', got '%s'", path)
	}
	if len(filters) != 0 {
		t.Errorf("expected no filters, got %d", len(filters))
	}
	if templatePath != "tpl.json" {
		t.Errorf("expected template path 'tpl.json', got '%s'", templatePath)
	}
}

func TestParseRefToken_WithBoth(t *testing.T) {
	path, filters, templatePath, err := parseRefToken("path/?filter=status:active&template=tpl.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "path/" {
		t.Errorf("expected path 'path/', got '%s'", path)
	}
	if len(filters) != 1 || filters["status"] != "active" {
		t.Errorf("expected filter status:active, got %v", filters)
	}
	if templatePath != "tpl.json" {
		t.Errorf("expected template path 'tpl.json', got '%s'", templatePath)
	}
}

func TestParseRefToken_DuplicateFilter(t *testing.T) {
	_, _, _, err := parseRefToken("path/?filter=status:active&filter=status:inactive")
	if err == nil {
		t.Fatalf("expected error for duplicate filter")
	}
	if !strings.Contains(err.Error(), "duplicate filter") {
		t.Errorf("error should mention duplicate filter, got: %v", err)
	}
}

func TestParseRefToken_InvalidFilter(t *testing.T) {
	_, _, _, err := parseRefToken("path/?filter=invalid")
	if err == nil {
		t.Fatalf("expected error for invalid filter format")
	}
	if !strings.Contains(err.Error(), "invalid filter format") {
		t.Errorf("error should mention invalid filter format, got: %v", err)
	}
}

func TestResolveRefs_EmptyContent(t *testing.T) {
	// Test empty content
	result, err := resolveRefs([]byte(""), "", make(map[string]bool))
	if err != nil {
		t.Fatalf("unexpected error for empty content: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for empty content")
	}

	// Test whitespace-only content
	result, err = resolveRefs([]byte("   \n  \t  "), "", make(map[string]bool))
	if err != nil {
		t.Fatalf("unexpected error for whitespace-only content: %v", err)
	}
	if string(result) != "   \n  \t  " {
		t.Errorf("expected whitespace content to be preserved")
	}
}

func TestResolveRefs_InvalidJSONAfterResolution(t *testing.T) {
	// This test ensures that if ref resolution produces invalid JSON,
	// we get a proper error rather than a panic or malformed output.
	tempDir := t.TempDir()

	// Create a file that when referenced will break JSON structure
	badFile := filepath.Join(tempDir, "bad.json")
	if err := os.WriteFile(badFile, []byte(`{"unclosed": "quote`), 0644); err != nil {
		t.Fatalf("writing bad JSON file: %v", err)
	}

	// Try to resolve a ref to the bad file
	content := []byte(`{"data": "{{ref:bad.json}}"}`)
	_, err := resolveRefs(content, tempDir, make(map[string]bool))
	if err == nil {
		t.Fatalf("expected error when referencing file with invalid JSON")
	}
	if !strings.Contains(err.Error(), "bad.json") {
		t.Errorf("error should mention the problematic file, got: %v", err)
	}
}

func TestResolveRefs_DirectoryTraversalPrevention(t *testing.T) {
	tempDir := t.TempDir()

	// Test directory traversal in ref path
	content := []byte(`{"data": "{{ref:../secret.json}}"}`)
	_, err := resolveRefs(content, tempDir, make(map[string]bool))
	if err == nil {
		t.Fatalf("expected error for directory traversal")
	}
	if !strings.Contains(err.Error(), "directory traversal not allowed") {
		t.Errorf("error should mention directory traversal prevention, got: %v", err)
	}

	// Test absolute path in ref
	content = []byte(`{"data": "{{ref:/etc/passwd}}"}`)
	_, err = resolveRefs(content, tempDir, make(map[string]bool))
	if err == nil {
		t.Fatalf("expected error for absolute path")
	}
	if !strings.Contains(err.Error(), "absolute paths not allowed") {
		t.Errorf("error should mention absolute path prevention, got: %v", err)
	}
}

func TestResolveRefs_TemplatePathTraversalPrevention(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("creating test directory: %v", err)
	}

	// Create test file
	file1 := filepath.Join(dataDir, "1.json")
	if err := os.WriteFile(file1, []byte(`{"id": "1", "name": "Alice"}`), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	// Test directory traversal in template path
	content := []byte(`{"data": "{{ref:data/?template=../../../etc/passwd}}"}`)
	_, err := resolveRefs(content, tempDir, make(map[string]bool))
	if err == nil {
		t.Fatalf("expected error for template directory traversal")
	}
	if !strings.Contains(err.Error(), "directory traversal not allowed in template") {
		t.Errorf("error should mention template directory traversal prevention, got: %v", err)
	}

	// Test absolute path in template
	content = []byte(`{"data": "{{ref:data/?template=/etc/passwd}}"}`)
	_, err = resolveRefs(content, tempDir, make(map[string]bool))
	if err == nil {
		t.Fatalf("expected error for absolute template path")
	}
	if !strings.Contains(err.Error(), "absolute paths not allowed in template") {
		t.Errorf("error should mention absolute template path prevention, got: %v", err)
	}
}

func TestResolveRefs_DirectoryIntentPreservation(t *testing.T) {
	tempDir := t.TempDir()

	// Test that directory intent is preserved even when directory doesn't exist yet
	content := []byte(`{"data": "{{ref:nonexistent/}}"}`)
	result, err := resolveRefs(content, tempDir, make(map[string]bool))

	// persist.ReadDir returns [] for nonexistent directories, so this should succeed
	if err != nil {
		t.Fatalf("unexpected error for nonexistent directory: %v", err)
	}

	// Parse result to verify the empty array was returned
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	data, ok := parsed["data"].([]interface{})
	if !ok {
		t.Fatalf("data should be an array")
	}
	if len(data) != 0 {
		t.Errorf("expected empty array for nonexistent directory, got %d items", len(data))
	}
}

// Tests for spread reference functionality

func TestResolveSpreadRefs_BasicSpread(t *testing.T) {
	// Setup temporary directory with test files
	tempDir := t.TempDir()

	// Create a source file to spread from
	sourceFile := filepath.Join(tempDir, "source.json")
	sourceData := `{"name": "John", "age": 30, "role": "admin"}`
	if err := os.WriteFile(sourceFile, []byte(sourceData), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	// Test basic spreading
	content := []byte(`{
		"id": "123",
		"$spread": "{{ref:source.json}}",
		"status": "active"
	}`)

	result, err := resolveSpreadRefs(content, tempDir, make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	// Verify spread properties are present
	if parsed["id"] != "123" {
		t.Errorf("expected id to be '123', got %v", parsed["id"])
	}
	if parsed["name"] != "John" {
		t.Errorf("expected name to be 'John', got %v", parsed["name"])
	}
	if parsed["age"] != float64(30) { // JSON numbers are float64
		t.Errorf("expected age to be 30, got %v", parsed["age"])
	}
	if parsed["role"] != "admin" {
		t.Errorf("expected role to be 'admin', got %v", parsed["role"])
	}
	if parsed["status"] != "active" {
		t.Errorf("expected status to be 'active', got %v", parsed["status"])
	}
}

func TestResolveSpreadRefs_PropertyOverride(t *testing.T) {
	// Setup temporary directory with test files
	tempDir := t.TempDir()

	// Create a source file to spread from
	sourceFile := filepath.Join(tempDir, "source.json")
	sourceData := `{"name": "John", "status": "inactive", "role": "admin"}`
	if err := os.WriteFile(sourceFile, []byte(sourceData), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	// Test property override (explicit properties should override spread)
	content := []byte(`{
		"id": "123",
		"$spread": "{{ref:source.json}}",
		"status": "active"
	}`)

	result, err := resolveSpreadRefs(content, tempDir, make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	// Verify that explicit properties override spread properties
	if parsed["status"] != "active" {
		t.Errorf("expected explicit status to override spread, got %v", parsed["status"])
	}
	if parsed["name"] != "John" {
		t.Errorf("expected spread name to be preserved, got %v", parsed["name"])
	}
}

func TestResolveSpreadRefs_WithPathParams(t *testing.T) {
	// Setup temporary directory with test files
	tempDir := t.TempDir()

	// Create endpoint directory and file
	endpointDir := filepath.Join(tempDir, "endpoints")
	if err := os.MkdirAll(endpointDir, 0755); err != nil {
		t.Fatalf("creating endpoint directory: %v", err)
	}

	endpointFile := filepath.Join(endpointDir, "123.json")
	endpointData := `{"endpointId": "123", "name": "test-endpoint", "region": "us-east-1"}`
	if err := os.WriteFile(endpointFile, []byte(endpointData), 0644); err != nil {
		t.Fatalf("writing endpoint file: %v", err)
	}

	// Create deployment directory and file
	deploymentDir := filepath.Join(tempDir, "deployments", "123")
	if err := os.MkdirAll(deploymentDir, 0755); err != nil {
		t.Fatalf("creating deployment directory: %v", err)
	}

	deploymentFile := filepath.Join(deploymentDir, "model1.json")
	deploymentData := `{"modelId": "model1", "status": "ready"}`
	if err := os.WriteFile(deploymentFile, []byte(deploymentData), 0644); err != nil {
		t.Fatalf("writing deployment file: %v", err)
	}

	// Test spread with path parameters (simulating user's use case)
	content := []byte(`{
		"$spread": "{{ref:endpoints/{path.endpointId}.json}}",
		"deployedModels": "{{ref:deployments/{path.endpointId}/}}"
	}`)

	// Create ref context with path parameters
	refCtx := &RefContext{
		PathParams: map[string]string{
			"endpointId": "123",
		},
	}

	result, err := resolveSpreadRefs(content, tempDir, make(map[string]bool), refCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	// Verify spread worked with path parameters
	if parsed["endpointId"] != "123" {
		t.Errorf("expected endpointId to be '123', got %v", parsed["endpointId"])
	}
	if parsed["name"] != "test-endpoint" {
		t.Errorf("expected name to be 'test-endpoint', got %v", parsed["name"])
	}
	if parsed["region"] != "us-east-1" {
		t.Errorf("expected region to be 'us-east-1', got %v", parsed["region"])
	}
}

func TestResolveSpreadRefs_NonObjectError(t *testing.T) {
	// Setup temporary directory with test files
	tempDir := t.TempDir()

	// Create a source file that contains an array (not spreadable)
	sourceFile := filepath.Join(tempDir, "array.json")
	sourceData := `["item1", "item2", "item3"]`
	if err := os.WriteFile(sourceFile, []byte(sourceData), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	// Test spreading from non-object should error
	content := []byte(`{
		"id": "123",
		"$spread": "{{ref:array.json}}"
	}`)

	_, err := resolveSpreadRefs(content, tempDir, make(map[string]bool), nil)
	if err == nil {
		t.Fatalf("expected error when trying to spread non-object")
	}
	if !strings.Contains(err.Error(), "$spread ref must resolve to an object") {
		t.Errorf("expected specific error about non-object, got: %v", err)
	}
}

func TestResolveSpreadRefs_NoSpreadTokens(t *testing.T) {
	// Test that content without spread tokens is unchanged
	content := []byte(`{
		"id": "123",
		"regular": "{{ref:some/file.json}}",
		"name": "test"
	}`)

	result, err := resolveSpreadRefs(content, "", make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be unchanged since no spread tokens
	if string(result) != string(content) {
		t.Errorf("content should remain unchanged when no spread tokens present")
	}
}

func TestResolveSpreadRefs_EmptyContent(t *testing.T) {
	// Test with empty content
	content := []byte("")

	result, err := resolveSpreadRefs(content, "", make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(result) != string(content) {
		t.Errorf("empty content should remain unchanged")
	}
}

func TestResolveSpreadRefs_NewSyntaxValidation(t *testing.T) {
	// Test that $spread field must contain a {{ref:...}} token
	tempDir := t.TempDir()

	// Test with invalid $spread value (not a ref token)
	content := []byte(`{
		"id": "123",
		"$spread": "invalid-value"
	}`)

	_, err := resolveSpreadRefs(content, tempDir, make(map[string]bool), nil)
	if err == nil {
		t.Fatalf("expected error for invalid $spread value")
	}
	if !strings.Contains(err.Error(), "$spread value must be a {{ref:...}} token") {
		t.Errorf("expected specific error about invalid token, got: %v", err)
	}

	// Test with non-string $spread value
	content2 := []byte(`{
		"id": "123",
		"$spread": 123
	}`)

	_, err = resolveSpreadRefs(content2, tempDir, make(map[string]bool), nil)
	if err == nil {
		t.Fatalf("expected error for non-string $spread value")
	}
	if !strings.Contains(err.Error(), "$spread field must be a string") {
		t.Errorf("expected specific error about string type, got: %v", err)
	}
}

func TestResolveSpreadRefs_NestedSpread(t *testing.T) {
	// Test that $spread works in nested objects
	tempDir := t.TempDir()

	// Create source files
	sourceFile := filepath.Join(tempDir, "user.json")
	sourceData := `{"name": "John", "role": "admin"}`
	if err := os.WriteFile(sourceFile, []byte(sourceData), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	// Test nested spreading
	content := []byte(`{
		"id": "123",
		"profile": {
			"$spread": "{{ref:user.json}}",
			"active": true
		},
		"status": "online"
	}`)

	result, err := resolveSpreadRefs(content, tempDir, make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	// Verify structure
	if parsed["id"] != "123" {
		t.Errorf("expected id to be preserved")
	}
	if parsed["status"] != "online" {
		t.Errorf("expected status to be preserved")
	}

	profile, ok := parsed["profile"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected profile to be an object")
	}

	// Verify spread properties are in nested object
	if profile["name"] != "John" {
		t.Errorf("expected name from spread in nested object")
	}
	if profile["role"] != "admin" {
		t.Errorf("expected role from spread in nested object")
	}
	if profile["active"] != true {
		t.Errorf("expected explicit property in nested object")
	}
}

func TestResolveSpreadRefs_IntegrationUseCase(t *testing.T) {
	// Integration test for user's exact use case
	// This simulates the complete apitwin-config structure and workflow

	tempDir := t.TempDir()

	// Create apitwin-config-like structure
	stubsDir := filepath.Join(tempDir, "stubs")
	endpointsDir := filepath.Join(stubsDir, "endpoints")
	deploymentsDir := filepath.Join(stubsDir, "deployments")
	templatesDir := filepath.Join(stubsDir, "templates")

	if err := os.MkdirAll(endpointsDir, 0755); err != nil {
		t.Fatalf("creating endpoints dir: %v", err)
	}
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("creating templates dir: %v", err)
	}

	// Create endpoint file (simulates what POST creates)
	endpointID := "920316521215950848"
	endpointFile := filepath.Join(endpointsDir, endpointID+".json")
	endpointData := `{
		"endpointId": "920316521215950848",
		"endpointDisplayName": "my-vehicle-2-endpoint-1",
		"description": "working demo",
		"driftDetection": "disabled",
		"enablePredictRequestResponseLogging": false
	}`
	if err := os.WriteFile(endpointFile, []byte(endpointData), 0644); err != nil {
		t.Fatalf("writing endpoint file: %v", err)
	}

	// Create deployment directory and data
	endpointDeploymentDir := filepath.Join(deploymentsDir, endpointID)
	if err := os.MkdirAll(endpointDeploymentDir, 0755); err != nil {
		t.Fatalf("creating deployment dir: %v", err)
	}

	deploymentFile := filepath.Join(endpointDeploymentDir, "deployment1.json")
	deploymentData := `{
		"deploymentId": "e9cf8112-4a0d-4903-95b6-268e6442d758",
		"modelDisplayName": "new-deployment",
		"status": "Ready",
		"createTime": "2026-03-26T14:56:45Z",
		"deploymentSpec": {
			"acceleratorCount": 0,
			"machineType": "e2-standard-2"
		}
	}`
	if err := os.WriteFile(deploymentFile, []byte(deploymentData), 0644); err != nil {
		t.Fatalf("writing deployment file: %v", err)
	}

	// Create template file
	templateFile := filepath.Join(templatesDir, "deployed-model.json")
	templateData := `{
		"id": "{{.deploymentId}}",
		"displayName": "{{.modelDisplayName}}",
		"status": "{{.status}}",
		"createTime": "{{.createTime}}",
		"machineType": "{{.deploymentSpec.machineType}}",
		"acceleratorCount": "{{.deploymentSpec.acceleratorCount}}"
	}`
	if err := os.WriteFile(templateFile, []byte(templateData), 0644); err != nil {
		t.Fatalf("writing template file: %v", err)
	}

	// User's desired JSON structure using spread syntax
	content := []byte(`{
		"$spread": "{{ref:stubs/endpoints/{path.endpointId}.json}}",
		"deployedModels": "{{ref:stubs/deployments/{path.endpointId}/?template=stubs/templates/deployed-model.json}}"
	}`)

	// Create ref context with path parameters (simulates GET request)
	refCtx := &RefContext{
		PathParams: map[string]string{
			"endpointId": endpointID,
		},
	}

	// Test the complete pipeline: spread resolution -> dynamic placeholders -> regular refs
	result, err := resolveRefsWithContext(content, tempDir, make(map[string]bool), refCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse and verify result
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	// Verify spread properties are present at root level
	if parsed["endpointId"] != endpointID {
		t.Errorf("expected endpointId to be %q, got %v", endpointID, parsed["endpointId"])
	}
	if parsed["endpointDisplayName"] != "my-vehicle-2-endpoint-1" {
		t.Errorf("expected endpointDisplayName to be 'my-vehicle-2-endpoint-1', got %v", parsed["endpointDisplayName"])
	}
	if parsed["description"] != "working demo" {
		t.Errorf("expected description to be 'working demo', got %v", parsed["description"])
	}
	if parsed["driftDetection"] != "disabled" {
		t.Errorf("expected driftDetection to be 'disabled', got %v", parsed["driftDetection"])
	}
	if parsed["enablePredictRequestResponseLogging"] != false {
		t.Errorf("expected enablePredictRequestResponseLogging to be false, got %v", parsed["enablePredictRequestResponseLogging"])
	}

	// Verify deployedModels array is populated
	deployedModels, ok := parsed["deployedModels"].([]interface{})
	if !ok {
		t.Fatalf("deployedModels should be an array, got %T", parsed["deployedModels"])
	}
	if len(deployedModels) != 1 {
		t.Errorf("expected 1 deployed model, got %d", len(deployedModels))
	}

	// Verify deployed model structure (template transformation applied)
	if len(deployedModels) > 0 {
		model, ok := deployedModels[0].(map[string]interface{})
		if !ok {
			t.Fatalf("deployed model should be an object, got %T", deployedModels[0])
		}

		if model["id"] != "e9cf8112-4a0d-4903-95b6-268e6442d758" {
			t.Errorf("expected model id to be transformed, got %v", model["id"])
		}
		if model["displayName"] != "new-deployment" {
			t.Errorf("expected model displayName to be transformed, got %v", model["displayName"])
		}
		if model["status"] != "Ready" {
			t.Errorf("expected model status to be transformed, got %v", model["status"])
		}
		if model["createTime"] != "2026-03-26T14:56:45Z" {
			t.Errorf("expected model createTime to be transformed, got %v", model["createTime"])
		}

		// Verify nested fields are flattened in template
		if model["machineType"] != "e2-standard-2" {
			t.Errorf("expected model machineType to be transformed, got %v", model["machineType"])
		}
		if model["acceleratorCount"] != "0" {
			t.Errorf("expected model acceleratorCount to be transformed, got %v", model["acceleratorCount"])
		}
	}

	// Verify structure - all fields should be at root level (flat structure)
	// This is the key benefit of the spread syntax
	expectedFields := []string{
		"endpointId", "endpointDisplayName", "description",
		"driftDetection", "enablePredictRequestResponseLogging", "deployedModels",
	}
	for _, field := range expectedFields {
		if _, exists := parsed[field]; !exists {
			t.Errorf("expected field %q to exist at root level", field)
		}
	}

	// Verify no nested "endpoint" or "endpointData" objects (which would happen without spread)
	for key := range parsed {
		if strings.Contains(key, "endpoint") && key != "endpointId" && key != "endpointDisplayName" {
			t.Errorf("unexpected nested endpoint object found: %q", key)
		}
	}
}

// ─── $as directive tests ───────────────────────────────────────────────────

func TestResolveAsRefs_ObjectFromArray(t *testing.T) {
	// Basic test: merge an array of objects into a single object
	tempDir := t.TempDir()

	// Create source file that contains an array of objects
	sourceFile := filepath.Join(tempDir, "splits.json")
	sourceData := `[{"deploymentA": 50}, {"deploymentB": 30}, {"deploymentC": 20}]`
	if err := os.WriteFile(sourceFile, []byte(sourceData), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	content := []byte(`{
		"$as": "object",
		"from": "{{ref:splits.json}}"
	}`)

	result, err := resolveAsRefs(content, tempDir, make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	if parsed["deploymentA"] != float64(50) {
		t.Errorf("expected deploymentA=50, got %v", parsed["deploymentA"])
	}
	if parsed["deploymentB"] != float64(30) {
		t.Errorf("expected deploymentB=30, got %v", parsed["deploymentB"])
	}
	if parsed["deploymentC"] != float64(20) {
		t.Errorf("expected deploymentC=20, got %v", parsed["deploymentC"])
	}
}

func TestResolveAsRefs_EmptyObjects(t *testing.T) {
	// Empty objects in the array should be skipped gracefully
	tempDir := t.TempDir()

	sourceFile := filepath.Join(tempDir, "splits.json")
	sourceData := `[{"a": 1}, {}, {"b": 2}]`
	if err := os.WriteFile(sourceFile, []byte(sourceData), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	content := []byte(`{
		"$as": "object",
		"from": "{{ref:splits.json}}"
	}`)

	result, err := resolveAsRefs(content, tempDir, make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	if len(parsed) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(parsed), parsed)
	}
	if parsed["a"] != float64(1) {
		t.Errorf("expected a=1, got %v", parsed["a"])
	}
	if parsed["b"] != float64(2) {
		t.Errorf("expected b=2, got %v", parsed["b"])
	}
}

func TestResolveAsRefs_OverlappingKeys(t *testing.T) {
	// Later keys overwrite earlier ones (like Object.assign)
	tempDir := t.TempDir()

	sourceFile := filepath.Join(tempDir, "splits.json")
	sourceData := `[{"key": "first"}, {"key": "second"}]`
	if err := os.WriteFile(sourceFile, []byte(sourceData), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	content := []byte(`{
		"$as": "object",
		"from": "{{ref:splits.json}}"
	}`)

	result, err := resolveAsRefs(content, tempDir, make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	if parsed["key"] != "second" {
		t.Errorf("expected later key to win, got %v", parsed["key"])
	}
}

func TestResolveAsRefs_EmptyArray(t *testing.T) {
	// Empty array should produce an empty object
	tempDir := t.TempDir()

	sourceFile := filepath.Join(tempDir, "empty.json")
	if err := os.WriteFile(sourceFile, []byte(`[]`), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	content := []byte(`{
		"$as": "object",
		"from": "{{ref:empty.json}}"
	}`)

	result, err := resolveAsRefs(content, tempDir, make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	if len(parsed) != 0 {
		t.Errorf("expected empty object, got %v", parsed)
	}
}

func TestResolveAsRefs_ErrorOnObjectSource(t *testing.T) {
	// $as "object" should error when source is an object, not an array
	tempDir := t.TempDir()

	sourceFile := filepath.Join(tempDir, "obj.json")
	if err := os.WriteFile(sourceFile, []byte(`{"a": 1}`), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	content := []byte(`{
		"$as": "object",
		"from": "{{ref:obj.json}}"
	}`)

	_, err := resolveAsRefs(content, tempDir, make(map[string]bool), nil)
	if err == nil {
		t.Fatalf("expected error but got none")
	}
	if !strings.Contains(err.Error(), "source must be an array") {
		t.Errorf("expected error about array source, got: %v", err)
	}
}

func TestResolveAsRefs_ErrorOnInvalidRef(t *testing.T) {
	// $as "from" must be a {{ref:...}} token
	content := []byte(`{
		"$as": "object",
		"from": "not-a-ref"
	}`)

	_, err := resolveAsRefs(content, t.TempDir(), make(map[string]bool), nil)
	if err == nil {
		t.Fatalf("expected error but got none")
	}
	if !strings.Contains(err.Error(), "must be a {{ref:...}} token") {
		t.Errorf("expected error about ref token, got: %v", err)
	}
}

func TestResolveAsRefs_ErrorOnMissingFrom(t *testing.T) {
	// $as without "from" field should error
	content := []byte(`{
		"$as": "object"
	}`)

	_, err := resolveAsRefs(content, t.TempDir(), make(map[string]bool), nil)
	if err == nil {
		t.Fatalf("expected error but got none")
	}
	if !strings.Contains(err.Error(), "requires a \"from\" field") {
		t.Errorf("expected error about missing from, got: %v", err)
	}
}

func TestResolveAsRefs_ErrorOnUnsupportedType(t *testing.T) {
	// Unsupported $as type should error with a clear message
	content := []byte(`{
		"$as": "banana",
		"from": "{{ref:something.json}}"
	}`)

	_, err := resolveAsRefs(content, t.TempDir(), make(map[string]bool), nil)
	if err == nil {
		t.Fatalf("expected error but got none")
	}
	if !strings.Contains(err.Error(), "unsupported $as target type") {
		t.Errorf("expected error about unsupported type, got: %v", err)
	}
}

func TestResolveAsRefs_ErrorOnNonStringAs(t *testing.T) {
	// $as must be a string
	content := []byte(`{
		"$as": 123,
		"from": "{{ref:something.json}}"
	}`)

	_, err := resolveAsRefs(content, t.TempDir(), make(map[string]bool), nil)
	if err == nil {
		t.Fatalf("expected error but got none")
	}
	if !strings.Contains(err.Error(), "$as field must be a string") {
		t.Errorf("expected error about string type, got: %v", err)
	}
}

func TestResolveAsRefs_ErrorOnNonStringFrom(t *testing.T) {
	// $as "from" must be a string
	content := []byte(`{
		"$as": "object",
		"from": 123
	}`)

	_, err := resolveAsRefs(content, t.TempDir(), make(map[string]bool), nil)
	if err == nil {
		t.Fatalf("expected error but got none")
	}
	if !strings.Contains(err.Error(), "\"from\" field must be a string") {
		t.Errorf("expected error about string from, got: %v", err)
	}
}

func TestResolveAsRefs_Nested(t *testing.T) {
	// $as should work inside nested objects
	tempDir := t.TempDir()

	sourceFile := filepath.Join(tempDir, "splits.json")
	sourceData := `[{"a": 10}, {"b": 20}]`
	if err := os.WriteFile(sourceFile, []byte(sourceData), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	content := []byte(`{
		"name": "my-endpoint",
		"trafficSplit": {
			"$as": "object",
			"from": "{{ref:splits.json}}"
		},
		"status": "active"
	}`)

	result, err := resolveAsRefs(content, tempDir, make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	if parsed["name"] != "my-endpoint" {
		t.Errorf("expected name preserved, got %v", parsed["name"])
	}
	if parsed["status"] != "active" {
		t.Errorf("expected status preserved, got %v", parsed["status"])
	}

	trafficSplit, ok := parsed["trafficSplit"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected trafficSplit to be an object, got %T", parsed["trafficSplit"])
	}
	if trafficSplit["a"] != float64(10) {
		t.Errorf("expected a=10, got %v", trafficSplit["a"])
	}
	if trafficSplit["b"] != float64(20) {
		t.Errorf("expected b=20, got %v", trafficSplit["b"])
	}
}

func TestResolveAsRefs_WithTemplate(t *testing.T) {
	// Integration test: $as with ?template= to merge templated results
	// This is the real use case: extract trafficSplit from each deployment
	tempDir := t.TempDir()

	// Create deployment directory with multiple files
	deploymentsDir := filepath.Join(tempDir, "stubs", "deployments", "endpoint-1")
	templatesDir := filepath.Join(tempDir, "stubs", "templates")
	if err := os.MkdirAll(deploymentsDir, 0755); err != nil {
		t.Fatalf("creating deployments dir: %v", err)
	}
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("creating templates dir: %v", err)
	}

	// Create deployment files (each has deploymentSpec.trafficSplit)
	dep1 := `{
		"deploymentId": "dep-1",
		"deploymentSpec": {
			"machineType": "e2-standard-2",
			"trafficSplit": {"dep-1": 60}
		}
	}`
	dep2 := `{
		"deploymentId": "dep-2",
		"deploymentSpec": {
			"machineType": "e2-standard-4",
			"trafficSplit": {"dep-2": 40}
		}
	}`
	if err := os.WriteFile(filepath.Join(deploymentsDir, "dep-1.json"), []byte(dep1), 0644); err != nil {
		t.Fatalf("writing dep1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deploymentsDir, "dep-2.json"), []byte(dep2), 0644); err != nil {
		t.Fatalf("writing dep2: %v", err)
	}

	// Create template that extracts just the trafficSplit
	templateData := `{{json .deploymentSpec.trafficSplit}}`
	if err := os.WriteFile(filepath.Join(templatesDir, "traffic-split.json"), []byte(templateData), 0644); err != nil {
		t.Fatalf("writing template: %v", err)
	}

	// Use $as to merge the array of trafficSplit objects into one
	content := []byte(`{
		"$as": "object",
		"from": "{{ref:stubs/deployments/endpoint-1/?template=stubs/templates/traffic-split.json}}"
	}`)

	result, err := resolveRefsWithContext([]byte(content), tempDir, make(map[string]bool), &RefContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v\nraw: %s", err, result)
	}

	if parsed["dep-1"] != float64(60) {
		t.Errorf("expected dep-1=60, got %v", parsed["dep-1"])
	}
	if parsed["dep-2"] != float64(40) {
		t.Errorf("expected dep-2=40, got %v", parsed["dep-2"])
	}
}

func TestResolveAsRefs_WithDynamicPlaceholders(t *testing.T) {
	// Test $as with dynamic path placeholders (like in 08-endpoints.toml)
	tempDir := t.TempDir()

	// Create deployment directory
	deploymentsDir := filepath.Join(tempDir, "stubs", "deployments", "ep-123")
	templatesDir := filepath.Join(tempDir, "stubs", "templates")
	if err := os.MkdirAll(deploymentsDir, 0755); err != nil {
		t.Fatalf("creating deployments dir: %v", err)
	}
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("creating templates dir: %v", err)
	}

	dep := `{
		"deploymentId": "d1",
		"deploymentSpec": {"trafficSplit": {"d1": 100}}
	}`
	if err := os.WriteFile(filepath.Join(deploymentsDir, "d1.json"), []byte(dep), 0644); err != nil {
		t.Fatalf("writing dep: %v", err)
	}

	templateData := `{{json .deploymentSpec.trafficSplit}}`
	if err := os.WriteFile(filepath.Join(templatesDir, "traffic-split.json"), []byte(templateData), 0644); err != nil {
		t.Fatalf("writing template: %v", err)
	}

	content := []byte(`{
		"$as": "object",
		"from": "{{ref:stubs/deployments/{path.endpointId}/?template=stubs/templates/traffic-split.json}}"
	}`)

	refCtx := &RefContext{
		PathParams: map[string]string{
			"endpointId": "ep-123",
		},
	}

	result, err := resolveRefsWithContext(content, tempDir, make(map[string]bool), refCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v\nraw: %s", err, result)
	}

	if parsed["d1"] != float64(100) {
		t.Errorf("expected d1=100, got %v", parsed["d1"])
	}
}

func TestResolveAsRefs_ErrorOnNonObjectArrayItem(t *testing.T) {
	// Array items must be objects when converting to object
	tempDir := t.TempDir()

	sourceFile := filepath.Join(tempDir, "mixed.json")
	if err := os.WriteFile(sourceFile, []byte(`[{"a": 1}, "not-an-object"]`), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	content := []byte(`{
		"$as": "object",
		"from": "{{ref:mixed.json}}"
	}`)

	_, err := resolveAsRefs(content, tempDir, make(map[string]bool), nil)
	if err == nil {
		t.Fatalf("expected error but got none")
	}
	if !strings.Contains(err.Error(), "array item at index 1 must be an object") {
		t.Errorf("expected error about non-object item, got: %v", err)
	}
}

func TestResolveAsRefs_NoAsDirective(t *testing.T) {
	// Content without $as should pass through unchanged
	content := []byte(`{"name": "test", "value": 42}`)

	result, err := resolveAsRefs(content, t.TempDir(), make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(result) != string(content) {
		t.Errorf("expected content to pass through unchanged\ngot:  %s\nwant: %s", result, content)
	}
}

func TestResolveAsRefs_FalsePositiveSubstring(t *testing.T) {
	// Content containing "$as" as a substring in a value (not as a directive key)
	// should pass through byte-identical without re-serialization
	content := []byte(`{"message":"saved $as draft","status":"$asset_id"}`)

	result, err := resolveAsRefs(content, t.TempDir(), make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(result, content) {
		t.Errorf("content was mutated by false-positive quick-check\ngot:  %s\nwant: %s", result, content)
	}
}

func TestResolveAsRefs_InsideArray(t *testing.T) {
	// $as should work when nested inside an array element
	tempDir := t.TempDir()

	sourceFile := filepath.Join(tempDir, "splits.json")
	if err := os.WriteFile(sourceFile, []byte(`[{"x": 1}, {"y": 2}]`), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	content := []byte(`[
		{"name": "static"},
		{"$as": "object", "from": "{{ref:splits.json}}"}
	]`)

	result, err := resolveAsRefs(content, tempDir, make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed []interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	if len(parsed) != 2 {
		t.Fatalf("expected 2 items, got %d", len(parsed))
	}

	// First item should be unchanged
	first, ok := parsed[0].(map[string]interface{})
	if !ok {
		t.Fatalf("first item should be object, got %T", parsed[0])
	}
	if first["name"] != "static" {
		t.Errorf("expected name=static, got %v", first["name"])
	}

	// Second item should be the merged object
	second, ok := parsed[1].(map[string]interface{})
	if !ok {
		t.Fatalf("second item should be object, got %T", parsed[1])
	}
	if second["x"] != float64(1) {
		t.Errorf("expected x=1, got %v", second["x"])
	}
	if second["y"] != float64(2) {
		t.Errorf("expected y=2, got %v", second["y"])
	}
}

func TestResolveAsRefs_ErrorOnExtraFields(t *testing.T) {
	// Extra fields on the $as directive should be rejected
	content := []byte(`{
		"$as": "object",
		"from": "{{ref:splits.json}}",
		"description": "should not be here"
	}`)

	_, err := resolveAsRefs(content, t.TempDir(), make(map[string]bool), nil)
	if err == nil {
		t.Fatalf("expected error but got none")
	}
	if !strings.Contains(err.Error(), "unexpected field") {
		t.Errorf("expected error about unexpected field, got: %v", err)
	}
}

func TestResolveEachAndTemplate_Basic(t *testing.T) {
	// Setup temporary directory with test files
	tempDir := t.TempDir()

	// Create endpoints directory
	endpointsDir := filepath.Join(tempDir, "stubs", "endpoints")
	if err := os.MkdirAll(endpointsDir, 0755); err != nil {
		t.Fatalf("creating endpoints directory: %v", err)
	}

	// Create deployments directory
	deploymentsDir := filepath.Join(tempDir, "stubs", "deployments")
	if err := os.MkdirAll(deploymentsDir, 0755); err != nil {
		t.Fatalf("creating deployments directory: %v", err)
	}

	// Create template directory
	templatesDir := filepath.Join(tempDir, "stubs", "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("creating templates directory: %v", err)
	}

	// Create test endpoint files
	endpoint1 := `{
		"endpointId": "123",
		"name": "endpoint-1",
		"status": "active"
	}`
	if err := os.WriteFile(filepath.Join(endpointsDir, "123.json"), []byte(endpoint1), 0644); err != nil {
		t.Fatalf("writing endpoint1: %v", err)
	}

	endpoint2 := `{
		"endpointId": "456",
		"name": "endpoint-2", 
		"status": "ready"
	}`
	if err := os.WriteFile(filepath.Join(endpointsDir, "456.json"), []byte(endpoint2), 0644); err != nil {
		t.Fatalf("writing endpoint2: %v", err)
	}

	// Create test deployment files
	deployment1Dir := filepath.Join(deploymentsDir, "123")
	if err := os.MkdirAll(deployment1Dir, 0755); err != nil {
		t.Fatalf("creating deployment1 directory: %v", err)
	}
	deploy1 := `{
		"deploymentId": "dep-123",
		"modelDisplayName": "Model 1",
		"status": "running"
	}`
	if err := os.WriteFile(filepath.Join(deployment1Dir, "deploy1.json"), []byte(deploy1), 0644); err != nil {
		t.Fatalf("writing deployment1: %v", err)
	}

	deployment2Dir := filepath.Join(deploymentsDir, "456")
	if err := os.MkdirAll(deployment2Dir, 0755); err != nil {
		t.Fatalf("creating deployment2 directory: %v", err)
	}
	deploy2 := `{
		"deploymentId": "dep-456", 
		"modelDisplayName": "Model 2",
		"status": "deployed"
	}`
	if err := os.WriteFile(filepath.Join(deployment2Dir, "deploy2.json"), []byte(deploy2), 0644); err != nil {
		t.Fatalf("writing deployment2: %v", err)
	}

	// Create template file
	template := `{
		"id": "{{.deploymentId}}",
		"displayName": "{{.modelDisplayName}}",
		"status": "{{.status}}"
	}`
	if err := os.WriteFile(filepath.Join(templatesDir, "deployed-model.json"), []byte(template), 0644); err != nil {
		t.Fatalf("writing template: %v", err)
	}

	// Test $each + $template processing with $spread and deployedModels
	input := `{
		"$each": "{{ref:stubs/endpoints/}}",
		"$template": {
			"$spread": "{{.}}",
			"deployedModels": "{{ref:stubs/deployments/{.endpointId}/?template=stubs/templates/deployed-model.json}}"
		}
	}`

	result, err := resolveRefsWithContext([]byte(input), tempDir, make(map[string]bool), &RefContext{})
	if err != nil {
		t.Fatalf("resolving refs: %v", err)
	}

	// Parse result to verify structure
	var parsed []interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	// Should have 2 endpoints
	if len(parsed) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(parsed))
	}

	// Check first endpoint
	endpoint1Result, ok := parsed[0].(map[string]interface{})
	if !ok {
		t.Fatalf("first endpoint should be object, got %T", parsed[0])
	}

	// Should have spread original properties
	if endpoint1Result["endpointId"] != "123" {
		t.Errorf("expected endpointId 123, got %v", endpoint1Result["endpointId"])
	}
	if endpoint1Result["name"] != "endpoint-1" {
		t.Errorf("expected name endpoint-1, got %v", endpoint1Result["name"])
	}

	// Should have deployedModels array
	deployedModels, ok := endpoint1Result["deployedModels"].([]interface{})
	if !ok {
		t.Fatalf("deployedModels should be array, got %T", endpoint1Result["deployedModels"])
	}
	if len(deployedModels) != 1 {
		t.Fatalf("expected 1 deployed model, got %d", len(deployedModels))
	}

	// Check deployed model structure
	deployedModel, ok := deployedModels[0].(map[string]interface{})
	if !ok {
		t.Fatalf("deployed model should be object, got %T", deployedModels[0])
	}
	if deployedModel["id"] != "dep-123" {
		t.Errorf("expected id dep-123, got %v", deployedModel["id"])
	}
	if deployedModel["displayName"] != "Model 1" {
		t.Errorf("expected displayName 'Model 1', got %v", deployedModel["displayName"])
	}
}

func TestResolveEachAndTemplate_ErrorHandling(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "$each without $template",
			input:   `{"$each": "{{ref:stubs/endpoints/}}"}`,
			wantErr: "$each field requires a corresponding $template field",
		},
		{
			name:    "$each not a string",
			input:   `{"$each": 123, "$template": {}}`,
			wantErr: "$each field must be a string, got float64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveEachAndTemplateRefs([]byte(tt.input), t.TempDir(), make(map[string]bool), &RefContext{})
			if err == nil {
				t.Fatalf("expected error but got none")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestResolveEachAndTemplate_NilRefContext(t *testing.T) {
	// Setup temporary directory with test files (minimal setup)
	tempDir := t.TempDir()

	// Create endpoints directory with one test file
	endpointsDir := filepath.Join(tempDir, "stubs", "endpoints")
	if err := os.MkdirAll(endpointsDir, 0755); err != nil {
		t.Fatalf("creating endpoints directory: %v", err)
	}

	endpoint := `{"endpointId": "123", "name": "test"}`
	if err := os.WriteFile(filepath.Join(endpointsDir, "123.json"), []byte(endpoint), 0644); err != nil {
		t.Fatalf("writing endpoint: %v", err)
	}

	// Test $each + $template with nil RefContext (should not panic)
	input := `{
		"$each": "{{ref:stubs/endpoints/}}",
		"$template": {
			"id": "{.endpointId}",
			"name": "{.name}"
		}
	}`

	// This should not panic even with nil RefContext
	result, err := resolveEachAndTemplateRefs([]byte(input), tempDir, make(map[string]bool), nil)
	if err != nil {
		t.Fatalf("resolving with nil RefContext: %v", err)
	}

	// Parse result to verify it worked
	var parsed []interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	if len(parsed) != 1 {
		t.Fatalf("expected 1 item, got %d", len(parsed))
	}

	item, ok := parsed[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected object, got %T", parsed[0])
	}

	if item["id"] != "123" {
		t.Errorf("expected id 123, got %v", item["id"])
	}
}

func TestResolveEachAndTemplate_DoubleBraceSyntax(t *testing.T) {
	// This test verifies that {{.field}} (double-brace, Go template syntax)
	// works in $template fields — both as standalone values and embedded
	// inside {{ref:...}} tokens. This is the pattern the client used.
	tempDir := t.TempDir()

	// Create endpoints directory
	endpointsDir := filepath.Join(tempDir, "stubs", "endpoints")
	if err := os.MkdirAll(endpointsDir, 0755); err != nil {
		t.Fatalf("creating endpoints directory: %v", err)
	}

	// Create deployments directory
	deploymentsDir := filepath.Join(tempDir, "stubs", "deployments")
	if err := os.MkdirAll(deploymentsDir, 0755); err != nil {
		t.Fatalf("creating deployments directory: %v", err)
	}

	// Create template directory
	templatesDir := filepath.Join(tempDir, "stubs", "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("creating templates directory: %v", err)
	}

	// Create test endpoint files
	endpoint1 := `{
		"endpointId": "ep-100",
		"endpointDisplayName": "Endpoint One",
		"description": "first endpoint",
		"driftDetection": "enabled",
		"enablePredictRequestResponseLogging": false
	}`
	if err := os.WriteFile(filepath.Join(endpointsDir, "ep-100.json"), []byte(endpoint1), 0644); err != nil {
		t.Fatalf("writing endpoint1: %v", err)
	}

	endpoint2 := `{
		"endpointId": "ep-200",
		"endpointDisplayName": "Endpoint Two",
		"description": "second endpoint",
		"driftDetection": "disabled",
		"enablePredictRequestResponseLogging": true
	}`
	if err := os.WriteFile(filepath.Join(endpointsDir, "ep-200.json"), []byte(endpoint2), 0644); err != nil {
		t.Fatalf("writing endpoint2: %v", err)
	}

	// Create deployment files per endpoint
	dep1Dir := filepath.Join(deploymentsDir, "ep-100")
	if err := os.MkdirAll(dep1Dir, 0755); err != nil {
		t.Fatalf("creating dep1Dir: %v", err)
	}
	dep1 := `{
		"deploymentId": "dep-a",
		"modelDisplayName": "Model Alpha",
		"status": "Ready",
		"createTime": "2026-01-01T00:00:00Z",
		"deploymentSpec": {"machineType": "e2-standard-2"}
	}`
	if err := os.WriteFile(filepath.Join(dep1Dir, "dep-a.json"), []byte(dep1), 0644); err != nil {
		t.Fatalf("writing dep1: %v", err)
	}

	dep2Dir := filepath.Join(deploymentsDir, "ep-200")
	if err := os.MkdirAll(dep2Dir, 0755); err != nil {
		t.Fatalf("creating dep2Dir: %v", err)
	}
	dep2a := `{
		"deploymentId": "dep-b",
		"modelDisplayName": "Model Beta",
		"status": "Deploying",
		"createTime": "2026-02-01T00:00:00Z",
		"deploymentSpec": {"machineType": "n1-standard-4"}
	}`
	dep2b := `{
		"deploymentId": "dep-c",
		"modelDisplayName": "Model Gamma",
		"status": "Ready",
		"createTime": "2026-03-01T00:00:00Z",
		"deploymentSpec": {"machineType": "a2-highgpu-1g"}
	}`
	if err := os.WriteFile(filepath.Join(dep2Dir, "dep-b.json"), []byte(dep2a), 0644); err != nil {
		t.Fatalf("writing dep2a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dep2Dir, "dep-c.json"), []byte(dep2b), 0644); err != nil {
		t.Fatalf("writing dep2b: %v", err)
	}

	// Create template file (uses Go template {{.field}} syntax — resolved by ?template= processor)
	tmpl := `{
		"id": "{{.deploymentId}}",
		"displayName": "{{.modelDisplayName}}",
		"status": "{{.status}}"
	}`
	if err := os.WriteFile(filepath.Join(templatesDir, "deployed-model.json"), []byte(tmpl), 0644); err != nil {
		t.Fatalf("writing template: %v", err)
	}

	// THE CLIENT'S PATTERN: $each/$template using {{.field}} (double braces) in template values,
	// and {{.endpointId}} embedded inside a {{ref:...}} token.
	input := `{
		"$each": "{{ref:stubs/endpoints/}}",
		"$template": {
			"endpointId": "{{.endpointId}}",
			"endpointDisplayName": "{{.endpointDisplayName}}",
			"description": "{{.description}}",
			"driftDetection": "{{.driftDetection}}",
			"enablePredictRequestResponseLogging": "{{.enablePredictRequestResponseLogging}}",
			"deployedModels": "{{ref:stubs/deployments/{{.endpointId}}/?template=stubs/templates/deployed-model.json}}"
		}
	}`

	result, err := resolveRefsWithContext([]byte(input), tempDir, make(map[string]bool), &RefContext{})
	if err != nil {
		t.Fatalf("resolving refs: %v", err)
	}

	// Parse result
	var parsed []interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing result: %v\nraw result: %s", err, string(result))
	}

	// Should have 2 endpoints
	if len(parsed) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(parsed))
	}

	// Track which endpoint IDs we see to verify both are present exactly once
	seenEndpoints := make(map[string]bool)

	// Check each endpoint
	for _, item := range parsed {
		ep, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("endpoint should be object, got %T", item)
		}

		epId, _ := ep["endpointId"].(string)
		if epId == "" {
			t.Errorf("endpointId should not be empty, got %v", ep["endpointId"])
		}

		// Context variables should NOT be "<no value>" or the raw placeholder
		if epId == "<no value>" || epId == "{{.endpointId}}" {
			t.Errorf("endpointId was not resolved, got %q", epId)
		}

		if seenEndpoints[epId] {
			t.Errorf("duplicate endpoint %q found", epId)
		}
		seenEndpoints[epId] = true

		epName, _ := ep["endpointDisplayName"].(string)
		if epName == "" || epName == "<no value>" || epName == "{{.endpointDisplayName}}" {
			t.Errorf("endpointDisplayName was not resolved, got %q", epName)
		}

		// deployedModels should be an array (not a raw string)
		deployedModels, ok := ep["deployedModels"].([]interface{})
		if !ok {
			t.Fatalf("deployedModels for %s should be array, got %T (%v)", epId, ep["deployedModels"], ep["deployedModels"])
		}

		// Each deployed model should have resolved fields
		for j, dm := range deployedModels {
			model, ok := dm.(map[string]interface{})
			if !ok {
				t.Fatalf("deployed model %d should be object, got %T", j, dm)
			}
			if model["id"] == nil || model["id"] == "" || model["id"] == "<no value>" {
				t.Errorf("deployed model %d id not resolved: %v", j, model["id"])
			}
			if model["displayName"] == nil || model["displayName"] == "" || model["displayName"] == "<no value>" {
				t.Errorf("deployed model %d displayName not resolved: %v", j, model["displayName"])
			}
		}

		// Verify specific endpoint data
		switch epId {
		case "ep-100":
			if epName != "Endpoint One" {
				t.Errorf("expected 'Endpoint One', got %q", epName)
			}
			if len(deployedModels) != 1 {
				t.Errorf("ep-100 should have 1 deployment, got %d", len(deployedModels))
			}
		case "ep-200":
			if epName != "Endpoint Two" {
				t.Errorf("expected 'Endpoint Two', got %q", epName)
			}
			if len(deployedModels) != 2 {
				t.Errorf("ep-200 should have 2 deployments, got %d", len(deployedModels))
			}
		}
	}

	// Assert both expected endpoints were present
	for _, expectedId := range []string{"ep-100", "ep-200"} {
		if !seenEndpoints[expectedId] {
			t.Errorf("expected endpoint %q not found in results", expectedId)
		}
	}
}

func TestTemplateRestoresComplexValues(t *testing.T) {
	// When a ?template= file references a field whose source value is a
	// map or slice, Go's text/template renders it as "map[…]" or "[…]".
	// restoreComplexValues should patch the original structured data back.
	tempDir := t.TempDir()

	// Source data with nested objects and arrays
	dataDir := filepath.Join(tempDir, "stubs", "items")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	item := `{
		"id": "item-1",
		"name": "widget",
		"spec": {"cpu": 4, "memory": "16Gi", "gpus": [{"type": "A100", "count": 2}]},
		"tags": ["prod", "ml"]
	}`
	if err := os.WriteFile(filepath.Join(dataDir, "item-1.json"), []byte(item), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Template that references the nested fields
	tmplDir := filepath.Join(tempDir, "stubs", "templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tmpl := `{
		"itemId": "{{.id}}",
		"itemName": "{{.name}}",
		"itemSpec": "{{.spec}}",
		"itemTags": "{{.tags}}"
	}`
	if err := os.WriteFile(filepath.Join(tmplDir, "item.json"), []byte(tmpl), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Use {{ref:...?template=...}} to trigger applyTemplate
	input := `"{{ref:stubs/items/?template=stubs/templates/item.json}}"`

	result, err := resolveRefsWithContext([]byte(input), tempDir, make(map[string]bool), &RefContext{})
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}

	var parsed []interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parsing: %v\nraw: %s", err, string(result))
	}

	if len(parsed) != 1 {
		t.Fatalf("expected 1 item, got %d", len(parsed))
	}

	obj, ok := parsed[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected object, got %T", parsed[0])
	}

	// Scalars should be fine
	if obj["itemId"] != "item-1" {
		t.Errorf("itemId: got %v", obj["itemId"])
	}
	if obj["itemName"] != "widget" {
		t.Errorf("itemName: got %v", obj["itemName"])
	}

	// Nested object should be a proper map, NOT "map[cpu:4 memory:16Gi ...]"
	spec, ok := obj["itemSpec"].(map[string]interface{})
	if !ok {
		t.Fatalf("itemSpec should be object, got %T (%v)", obj["itemSpec"], obj["itemSpec"])
	}
	if spec["cpu"] != float64(4) {
		t.Errorf("spec.cpu: expected 4, got %v", spec["cpu"])
	}
	if spec["memory"] != "16Gi" {
		t.Errorf("spec.memory: expected 16Gi, got %v", spec["memory"])
	}

	// Nested array should be a proper slice, NOT "[prod ml]"
	tags, ok := obj["itemTags"].([]interface{})
	if !ok {
		t.Fatalf("itemTags should be array, got %T (%v)", obj["itemTags"], obj["itemTags"])
	}
	if len(tags) != 2 || tags[0] != "prod" || tags[1] != "ml" {
		t.Errorf("tags: expected [prod, ml], got %v", tags)
	}
}
