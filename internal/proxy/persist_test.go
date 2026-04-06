package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readJSONFile reads a JSON file and returns the parsed map.
func readJSONFile(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &result))
	return result
}

// TestAppendKeyResolutionFromNamedPathParam verifies that when merge=append
// and key references a named path parameter that is NOT in the request body,
// the key value is resolved from the path parameter and used as the filename.
//
// This is the exact scenario from the bug report:
//
//	match = "/api/v1/*/environments/*/endpoint/{endpointId}/publication"
//	key   = "endpointId"
//	POST  /api/v1/org123/environments/env456/endpoint/6053b5e4-.../publication
//	Body: {"rateLimitCount": 100, ...}  (no endpointId field)
//
// Expected: file created as <dir>/6053b5e4-....json (from path param)
// Before fix: file was created as <dir>/<random-uuid>.json
func TestAppendKeyResolutionFromNamedPathParam(t *testing.T) {
	dir := t.TempDir()
	stubDir := filepath.Join(dir, "publications")
	require.NoError(t, os.MkdirAll(stubDir, 0755))

	c := config.Case{
		Status:  201,
		File:    stubDir + "/",
		Persist: true,
		Merge:   "append",
		Key:     "endpointId",
	}

	body := []byte(`{"rateLimitCount": 100, "subDomain": "gemma"}`)
	routePattern := "/api/v1/*/environments/*/endpoint/{endpointId}/publication"
	pathParams := map[string]string{
		"endpointId": "6053b5e4-cdbf-40c0-a6c8-cb41f29fe6bf",
	}

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/org123/environments/env456/endpoint/6053b5e4-cdbf-40c0-a6c8-cb41f29fe6bf/publication",
		nil,
	)
	w := httptest.NewRecorder()

	handled, _ := applyPersist(w, req, c, body, routePattern, "", pathParams)
	require.True(t, handled, "applyPersist should handle the request")
	assert.Equal(t, http.StatusCreated, w.Code)

	// The file should be named after the path parameter value.
	expectedFile := filepath.Join(stubDir, "6053b5e4-cdbf-40c0-a6c8-cb41f29fe6bf.json")
	assert.FileExists(t, expectedFile, "file should be named using the path parameter value, not a random UUID")

	// Verify the key was injected into the written record.
	content := readJSONFile(t, expectedFile)
	assert.Equal(t, "6053b5e4-cdbf-40c0-a6c8-cb41f29fe6bf", content["endpointId"])
	assert.Equal(t, float64(100), content["rateLimitCount"])
	assert.Equal(t, "gemma", content["subDomain"])
}

// TestAppendKeyResolutionBodyWinsOverPathParam verifies that when the key
// field exists in BOTH the request body and as a named path parameter,
// the body value takes priority (body wins if present).
func TestAppendKeyResolutionBodyWinsOverPathParam(t *testing.T) {
	dir := t.TempDir()
	stubDir := filepath.Join(dir, "items")
	require.NoError(t, os.MkdirAll(stubDir, 0755))

	c := config.Case{
		Status:  201,
		File:    stubDir + "/",
		Persist: true,
		Merge:   "append",
		Key:     "itemId",
	}

	// Body contains itemId — this value should win.
	body := []byte(`{"itemId": "body-value-123", "name": "Widget"}`)
	routePattern := "/items/{itemId}/copies"
	pathParams := map[string]string{
		"itemId": "path-value-456",
	}

	req := httptest.NewRequest(http.MethodPost, "/items/path-value-456/copies", nil)
	w := httptest.NewRecorder()

	handled, _ := applyPersist(w, req, c, body, routePattern, "", pathParams)
	require.True(t, handled)
	assert.Equal(t, http.StatusCreated, w.Code)

	// File should use the body value, NOT the path parameter.
	expectedFile := filepath.Join(stubDir, "body-value-123.json")
	assert.FileExists(t, expectedFile, "body value should take priority over path param for key resolution")

	// Path-param-named file should NOT exist.
	pathParamFile := filepath.Join(stubDir, "path-value-456.json")
	assert.NoFileExists(t, pathParamFile, "path param value should not be used when body has the key field")

	content := readJSONFile(t, expectedFile)
	assert.Equal(t, "body-value-123", content["itemId"])
	assert.Equal(t, "Widget", content["name"])
}

// TestAppendKeyResolutionFromQueryParam verifies that when the key field is
// missing from both the request body and path parameters, it falls back to
// query parameters.
func TestAppendKeyResolutionFromQueryParam(t *testing.T) {
	dir := t.TempDir()
	stubDir := filepath.Join(dir, "entries")
	require.NoError(t, os.MkdirAll(stubDir, 0755))

	c := config.Case{
		Status:  201,
		File:    stubDir + "/",
		Persist: true,
		Merge:   "append",
		Key:     "entryId",
	}

	body := []byte(`{"title": "Hello"}`)
	routePattern := "/entries"
	pathParams := map[string]string{} // no named path params

	req := httptest.NewRequest(http.MethodPost, "/entries?entryId=query-789", nil)
	w := httptest.NewRecorder()

	handled, _ := applyPersist(w, req, c, body, routePattern, "", pathParams)
	require.True(t, handled)
	assert.Equal(t, http.StatusCreated, w.Code)

	expectedFile := filepath.Join(stubDir, "query-789.json")
	assert.FileExists(t, expectedFile, "should fall back to query param when body and path params lack the key")

	content := readJSONFile(t, expectedFile)
	assert.Equal(t, "query-789", content["entryId"])
	assert.Equal(t, "Hello", content["title"])
}

// TestAppendKeyResolutionFallsBackToUUID verifies that when the key field is
// missing from all sources (body, path params, query params), a UUID is
// generated as before.
func TestAppendKeyResolutionFallsBackToUUID(t *testing.T) {
	dir := t.TempDir()
	stubDir := filepath.Join(dir, "records")
	require.NoError(t, os.MkdirAll(stubDir, 0755))

	c := config.Case{
		Status:  201,
		File:    stubDir + "/",
		Persist: true,
		Merge:   "append",
		Key:     "recordId",
	}

	body := []byte(`{"data": "value"}`)
	routePattern := "/records"
	pathParams := map[string]string{}

	req := httptest.NewRequest(http.MethodPost, "/records", nil)
	w := httptest.NewRecorder()

	handled, _ := applyPersist(w, req, c, body, routePattern, "", pathParams)
	require.True(t, handled)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Should create exactly one file with a UUID-based name.
	entries, err := os.ReadDir(stubDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "should create exactly one file")

	// Read it and verify the auto-generated key was injected.
	filePath := filepath.Join(stubDir, entries[0].Name())
	content := readJSONFile(t, filePath)
	assert.NotEmpty(t, content["recordId"], "auto-generated key should be injected into the record")
	assert.Equal(t, "value", content["data"])
}

// TestAppendKeyResolutionFromWildcard verifies that when the key field is
// missing from the body, wildcard values in the path are used as fallback
// (via extractValue's wildcard extraction).
func TestAppendKeyResolutionFromWildcard(t *testing.T) {
	dir := t.TempDir()
	stubDir := filepath.Join(dir, "wildcard")
	require.NoError(t, os.MkdirAll(stubDir, 0755))

	c := config.Case{
		Status:  201,
		File:    stubDir + "/",
		Persist: true,
		Merge:   "append",
		Key:     "tenantId",
	}

	body := []byte(`{"action": "create"}`)
	// Wildcard pattern — extractValue("path", "tenantId", ...) will get the
	// first * match from the URL.
	routePattern := "/tenants/*/resources"
	pathParams := map[string]string{} // no named params

	req := httptest.NewRequest(http.MethodPost, "/tenants/tenant-abc/resources", nil)
	w := httptest.NewRecorder()

	handled, _ := applyPersist(w, req, c, body, routePattern, "", pathParams)
	require.True(t, handled)
	assert.Equal(t, http.StatusCreated, w.Code)

	expectedFile := filepath.Join(stubDir, "tenant-abc.json")
	assert.FileExists(t, expectedFile, "should use wildcard match from the path")

	content := readJSONFile(t, expectedFile)
	assert.Equal(t, "tenant-abc", content["tenantId"])
	assert.Equal(t, "create", content["action"])
}
