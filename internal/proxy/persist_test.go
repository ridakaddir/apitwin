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

// -----------------------------------------------------------------------
// applyPersist with source
// -----------------------------------------------------------------------

func TestUpdateWithSource(t *testing.T) {
	dir := t.TempDir()
	stubFile := filepath.Join(dir, "france.json")
	require.NoError(t, os.WriteFile(stubFile, []byte(`{"name":"France","code":"FR","continent":"Europe"}`), 0644))

	c := config.Case{
		Status:  200,
		File:    stubFile,
		Persist: true,
		Merge:   "update",
		Source:  "country",
	}

	body := []byte(`{"continent":"Europe","region":"Western","country":{"name":"France","code":"FRA"}}`)
	req := httptest.NewRequest(http.MethodPut, "/countries/FR", nil)
	w := httptest.NewRecorder()

	handled, _ := applyPersist(w, req, c, body, "/countries/*", "", map[string]string{})
	require.True(t, handled)
	assert.Equal(t, http.StatusOK, w.Code)

	result := readJSONFile(t, stubFile)
	// source extracted only "country" sub-field, so continent/region should NOT leak.
	assert.Nil(t, result["continent_root"], "top-level continent should not leak into stub")
	assert.Nil(t, result["region"], "region should not leak into stub")
	// Merged data from country sub-field.
	assert.Equal(t, "France", result["name"])
	assert.Equal(t, "FRA", result["code"])
	// Original field preserved.
	assert.Equal(t, "Europe", result["continent"])
}

// -----------------------------------------------------------------------
// POST response directory-ref resolution
// -----------------------------------------------------------------------

// TestAppendResponseResolvesDirectoryRefs pins the DX contract that POST
// responses resolve directory refs so clients see the same shape a
// subsequent GET would return. The on-disk stub still keeps the live
// {{ref:.../}} template unchanged (so GETs continue to re-resolve on each
// read), but the response body shows the expanded array.
//
// Before this fix: clients got back `"models": "{{ref:stubs/models/}}"`
// as a literal string in the POST response and had to do a follow-up GET
// just to see the data — confusing and inconsistent with GET behavior.
func TestAppendResponseResolvesDirectoryRefs(t *testing.T) {
	configDir := t.TempDir()

	// Create a small models directory the ref will expand.
	modelsDir := filepath.Join(configDir, "stubs", "models")
	require.NoError(t, os.MkdirAll(modelsDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(modelsDir, "m1.json"),
		[]byte(`{"id":"m1","name":"GPT-4"}`),
		0644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(modelsDir, "m2.json"),
		[]byte(`{"id":"m2","name":"Claude"}`),
		0644,
	))

	// Append target directory for the POST.
	endpointsDir := filepath.Join(configDir, "stubs", "endpoints")
	require.NoError(t, os.MkdirAll(endpointsDir, 0755))

	// Defaults file with a live directory ref. resolveFileRefs inside
	// loadDefaults intentionally leaves directory refs raw so the on-disk
	// stub stays reactive; resolveResponseRefs must expand them for the
	// client response.
	defaultsPath := filepath.Join(configDir, "stubs", "defaults", "endpoint.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(defaultsPath), 0755))
	require.NoError(t, os.WriteFile(
		defaultsPath,
		[]byte(`{"status":"Deploying","models":"{{ref:stubs/models/}}"}`),
		0644,
	))

	c := config.Case{
		Status:   201,
		File:     endpointsDir + "/",
		Persist:  true,
		Merge:    "append",
		Key:      "endpointId",
		Defaults: "stubs/defaults/endpoint.json",
	}

	body := []byte(`{"endpointId":"ep-1","region":"us-east-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/endpoints", nil)
	w := httptest.NewRecorder()

	handled, createdPath := applyPersist(w, req, c, body, "/endpoints", configDir, map[string]string{})
	require.True(t, handled)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Response body: the directory ref MUST be expanded into an array.
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	models, ok := resp["models"].([]interface{})
	require.True(t, ok, "models must be a JSON array in the response, got %T: %v", resp["models"], resp["models"])
	assert.Len(t, models, 2, "both model files must be included in the expanded ref")

	// Sanity: the expanded entries look like the model files we wrote.
	names := map[string]bool{}
	for _, m := range models {
		if rec, ok := m.(map[string]interface{}); ok {
			if n, ok := rec["name"].(string); ok {
				names[n] = true
			}
		}
	}
	assert.True(t, names["GPT-4"], "GPT-4 must be in the expanded models list")
	assert.True(t, names["Claude"], "Claude must be in the expanded models list")

	// On-disk stub: the live directory ref MUST be preserved raw so
	// subsequent GETs continue to re-resolve against the current state of
	// stubs/models/. This is the invariant the fix must NOT break.
	stub := readJSONFile(t, createdPath)
	stubModels, ok := stub["models"].(string)
	require.True(t, ok, "on-disk stub must keep models as a raw ref string, got %T: %v", stub["models"], stub["models"])
	assert.Contains(t, stubModels, "{{ref:stubs/models/}}",
		"directory ref must be kept live in the persisted stub file")

	// The non-ref fields from the defaults and the request body should both
	// be present in the response and on disk.
	assert.Equal(t, "Deploying", resp["status"])
	assert.Equal(t, "Deploying", stub["status"])
	assert.Equal(t, "ep-1", resp["endpointId"])
	assert.Equal(t, "us-east-1", resp["region"])
}
