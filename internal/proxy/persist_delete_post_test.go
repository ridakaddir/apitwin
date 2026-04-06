package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostDeletePost_Basic verifies that calling POST (append) → DELETE →
// POST (append) works correctly without transitions.
func TestPostDeletePost_Basic(t *testing.T) {
	dir := t.TempDir()
	stubDir := filepath.Join(dir, "stubs", "items")
	require.NoError(t, os.MkdirAll(stubDir, 0755))

	cfg := &config.Config{
		Routes: []config.Route{
			{
				Method:   "POST",
				Match:    "/items",
				Fallback: "created",
				Cases: map[string]config.Case{
					"created": {
						Status:  201,
						File:    "stubs/items/",
						Persist: true,
						Merge:   "append",
						Key:     "id",
					},
				},
			},
			{
				Method:   "DELETE",
				Match:    "/items/{id}",
				Fallback: "deleted",
				Cases: map[string]config.Case{
					"deleted": {
						Status:  204,
						File:    "stubs/items/{path.id}.json",
						Persist: true,
						Merge:   "delete",
					},
				},
			},
		},
	}

	loader := &stubConfigLoader{cfg: cfg, configDir: dir}
	handler := NewHandler(loader, nil, false, "")

	// Step 1: POST
	body1 := []byte(`{"id": "item-1", "name": "First Item"}`)
	req1 := httptest.NewRequest(http.MethodPost, "/items", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusCreated, w1.Code, "first POST should return 201")

	// Step 2: DELETE
	req2 := httptest.NewRequest(http.MethodDelete, "/items/item-1", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusNoContent, w2.Code, "DELETE should return 204")

	// Step 3: POST again
	body3 := []byte(`{"id": "item-2", "name": "Second Item"}`)
	req3 := httptest.NewRequest(http.MethodPost, "/items", bytes.NewReader(body3))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusCreated, w3.Code,
		"second POST after DELETE should return 201, got %d; body: %s", w3.Code, w3.Body.String())

	secondFile := filepath.Join(stubDir, "item-2.json")
	assert.FileExists(t, secondFile)
}

// TestPostDeletePost_TransitionResolvesToUpdateCase reproduces the exact bug:
//
// Configuration (modeled after the user's batch-config.toml):
//
//	POST route with transitions:
//	  - "creating" (duration 10s) → used for background scheduling only
//	  - "ready" (terminal)        → merge=update on the specific file
//	  fallback = "created"        → merge=append to directory
//
// Flow:
//  1. POST creates file via "created" (append). Transition records t0.
//  2. DELETE removes the file.
//  3. After the transition duration expires (>10s), a new POST arrives.
//     resolveCase() sees transitions are defined and resolve() returns "ready"
//     (the terminal case). Since "ready" exists in route.Cases, the handler
//     uses it instead of the fallback "created".
//  4. The "ready" case has merge=update pointing at a specific file that
//     was deleted → persist.Update returns NotFoundError → 404.
//
// The fix: the transition state for a route should reset when a DELETE
// removes the resource, so subsequent POSTs start fresh with the fallback.
func TestPostDeletePost_TransitionResolvesToUpdateCase(t *testing.T) {
	dir := t.TempDir()
	stubDir := filepath.Join(dir, "stubs", "batch-configs")
	defaultsDir := filepath.Join(dir, "stubs", "defaults")
	require.NoError(t, os.MkdirAll(stubDir, 0755))
	require.NoError(t, os.MkdirAll(defaultsDir, 0755))

	// Write defaults files.
	require.NoError(t, os.WriteFile(
		filepath.Join(defaultsDir, "batch-config.json"),
		[]byte(`{"status": {"ready": false, "inProgress": true}}`),
		0644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(defaultsDir, "batch-config-ready.json"),
		[]byte(`{"status": {"ready": true, "inProgress": false}}`),
		0644,
	))

	cfg := &config.Config{
		Routes: []config.Route{
			{
				Method:   "POST",
				Match:    "/api/v1/*/environments/*/model/{modelId}/batchConfig",
				Fallback: "created",
				Transitions: []config.Transition{
					{Case: "creating", Duration: 1}, // 1 second for test speed
					{Case: "ready"},                 // terminal
				},
				Cases: map[string]config.Case{
					"created": {
						Status:   201,
						File:     "stubs/batch-configs/",
						Persist:  true,
						Merge:    "append",
						Key:      "modelId",
						Defaults: "stubs/defaults/batch-config.json",
					},
					"ready": {
						Persist:  true,
						Merge:    "update",
						File:     "stubs/batch-configs/{path.modelId}.json",
						Defaults: "stubs/defaults/batch-config-ready.json",
					},
				},
			},
			{
				Method:   "DELETE",
				Match:    "/api/v1/*/environments/*/model/{modelId}/batchConfig",
				Fallback: "deleted",
				Cases: map[string]config.Case{
					"deleted": {
						Status:  200,
						File:    "stubs/batch-configs/{path.modelId}.json",
						Persist: true,
						Merge:   "delete",
					},
				},
			},
		},
	}

	loader := &stubConfigLoader{cfg: cfg, configDir: dir}
	ts := newTransitionState()
	handler := NewHandlerWithTransitions(loader, nil, false, "", ts, nil, nil)

	modelId := "8979323336441987072"
	basePath := "/api/v1/org123/environments/env456/model/" + modelId + "/batchConfig"

	// ── Step 1: POST creates the batch config ──────────────────────────
	body1 := []byte(`{"rateLimitCount": 100}`)
	req1 := httptest.NewRequest(http.MethodPost, basePath, bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	require.Equal(t, http.StatusCreated, w1.Code, "first POST should return 201")

	createdFile := filepath.Join(stubDir, modelId+".json")
	require.FileExists(t, createdFile)

	// ── Step 2: DELETE removes the batch config ────────────────────────
	req2 := httptest.NewRequest(http.MethodDelete, basePath, nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	require.Equal(t, http.StatusOK, w2.Code, "DELETE should return 200")
	require.NoFileExists(t, createdFile, "file should be deleted")

	// ── Step 3: Wait for transition to expire ──────────────────────────
	// After this, resolveCase will return "ready" (the terminal case)
	// instead of the fallback "created".
	time.Sleep(1100 * time.Millisecond)

	// ── Step 4: POST again — THIS IS WHERE THE BUG MANIFESTS ───────────
	// Without the fix, resolveCase returns "ready" (merge=update on
	// a deleted file) → 404 {"error":"file not found"}.
	// With the fix, the transition resets on DELETE, so resolveCase
	// returns the fallback "created" (merge=append) → 201.
	body4 := []byte(`{"rateLimitCount": 200}`)
	req4 := httptest.NewRequest(http.MethodPost, basePath, bytes.NewReader(body4))
	req4.Header.Set("Content-Type", "application/json")
	w4 := httptest.NewRecorder()
	handler.ServeHTTP(w4, req4)

	assert.Equal(t, http.StatusCreated, w4.Code,
		"POST after DELETE + transition expiry should return 201 (created), "+
			"got %d; body: %s", w4.Code, w4.Body.String())

	// Verify the file was re-created.
	assert.FileExists(t, createdFile, "batch config file should be re-created")

	content := readJSONFile(t, createdFile)
	assert.Equal(t, modelId, content["modelId"])
}

// TestPostDeletePost_TransitionResetsOnDelete verifies that the transition
// state is properly reset when DELETE is called, so that subsequent POSTs
// start the transition sequence from the beginning.
func TestPostDeletePost_TransitionResetsOnDelete(t *testing.T) {
	dir := t.TempDir()
	stubDir := filepath.Join(dir, "stubs", "resources")
	require.NoError(t, os.MkdirAll(stubDir, 0755))

	cfg := &config.Config{
		Routes: []config.Route{
			{
				Method:   "POST",
				Match:    "/resources/{id}",
				Fallback: "created",
				Transitions: []config.Transition{
					{Case: "provisioning", Duration: 1},
					{Case: "active"}, // terminal
				},
				Cases: map[string]config.Case{
					"created": {
						Status:  201,
						File:    "stubs/resources/",
						Persist: true,
						Merge:   "append",
						Key:     "id",
					},
					"active": {
						Persist: true,
						Merge:   "update",
						File:    "stubs/resources/{path.id}.json",
					},
				},
			},
			{
				Method:   "DELETE",
				Match:    "/resources/{id}",
				Fallback: "deleted",
				Cases: map[string]config.Case{
					"deleted": {
						Status:  200,
						File:    "stubs/resources/{path.id}.json",
						Persist: true,
						Merge:   "delete",
					},
				},
			},
		},
	}

	loader := &stubConfigLoader{cfg: cfg, configDir: dir}
	ts := newTransitionState()
	handler := NewHandlerWithTransitions(loader, nil, false, "", ts, nil, nil)

	// POST — creates resource, transition starts.
	body1 := []byte(`{"id": "res-1", "name": "Resource One"}`)
	req1 := httptest.NewRequest(http.MethodPost, "/resources/res-1", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusCreated, w1.Code)

	// Wait for transition to expire.
	time.Sleep(1100 * time.Millisecond)

	// DELETE — removes the resource.
	req2 := httptest.NewRequest(http.MethodDelete, "/resources/res-1", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)

	// POST again — should start fresh with "created", not "active".
	body3 := []byte(`{"id": "res-1", "name": "Resource One Recreated"}`)
	req3 := httptest.NewRequest(http.MethodPost, "/resources/res-1", bytes.NewReader(body3))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusCreated, w3.Code,
		"POST after DELETE should use fallback 'created' case, got %d; body: %s",
		w3.Code, w3.Body.String())

	createdFile := filepath.Join(stubDir, "res-1.json")
	assert.FileExists(t, createdFile)

	content := readJSONFile(t, createdFile)
	assert.Equal(t, "Resource One Recreated", content["name"])
}

// TestPostDifferentResource_TransitionLeaksBetweenResources verifies that
// creating resource A, waiting for the transition to expire, and then creating
// resource B works correctly. The transition state is keyed by route pattern
// (shared across all resource IDs), so after the transition expires for A,
// creating B would previously resolve to the terminal "ready" case (merge=update)
// instead of the fallback "created" case (merge=append), causing a 404 because
// B's file doesn't exist yet.
func TestPostDifferentResource_TransitionLeaksBetweenResources(t *testing.T) {
	dir := t.TempDir()
	stubDir := filepath.Join(dir, "stubs", "batch-configs")
	defaultsDir := filepath.Join(dir, "stubs", "defaults")
	require.NoError(t, os.MkdirAll(stubDir, 0755))
	require.NoError(t, os.MkdirAll(defaultsDir, 0755))

	// Write defaults files.
	require.NoError(t, os.WriteFile(
		filepath.Join(defaultsDir, "batch-config.json"),
		[]byte(`{"status": {"ready": false, "inProgress": true}}`),
		0644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(defaultsDir, "batch-config-ready.json"),
		[]byte(`{"status": {"ready": true, "inProgress": false}}`),
		0644,
	))

	cfg := &config.Config{
		Routes: []config.Route{
			{
				Method:   "POST",
				Match:    "/api/v1/*/environments/*/model/{modelId}/batchConfig",
				Fallback: "created",
				Transitions: []config.Transition{
					{Case: "creating", Duration: 1}, // 1 second for test speed
					{Case: "ready"},                 // terminal
				},
				Cases: map[string]config.Case{
					"created": {
						Status:   201,
						File:     "stubs/batch-configs/",
						Persist:  true,
						Merge:    "append",
						Key:      "modelId",
						Defaults: "stubs/defaults/batch-config.json",
					},
					"ready": {
						Persist:  true,
						Merge:    "update",
						File:     "stubs/batch-configs/{path.modelId}.json",
						Defaults: "stubs/defaults/batch-config-ready.json",
					},
				},
			},
		},
	}

	loader := &stubConfigLoader{cfg: cfg, configDir: dir}
	ts := newTransitionState()
	handler := NewHandlerWithTransitions(loader, nil, false, "", ts, nil, nil)

	basePath := "/api/v1/org123/environments/env456/model"

	// ── Step 1: POST creates resource A ──────────────────────────
	modelA := "model-A"
	body1 := []byte(`{"rateLimitCount": 100}`)
	req1 := httptest.NewRequest(http.MethodPost, basePath+"/"+modelA+"/batchConfig", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusCreated, w1.Code, "POST model A should return 201")
	require.FileExists(t, filepath.Join(stubDir, modelA+".json"))

	// ── Step 2: Wait for transition to expire ──────────────────────────
	time.Sleep(1100 * time.Millisecond)

	// ── Step 3: POST creates resource B (different model ID) ──────────
	// Without the fix, the transition state resolves to "ready" (terminal)
	// which tries merge=update on model-B's file that doesn't exist → 404.
	modelB := "model-B"
	body3 := []byte(`{"rateLimitCount": 200}`)
	req3 := httptest.NewRequest(http.MethodPost, basePath+"/"+modelB+"/batchConfig", bytes.NewReader(body3))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusCreated, w3.Code,
		"POST model B should return 201 (created), not 404; "+
			"transition state should not leak between resources. body: %s", w3.Body.String())

	// Verify both files exist.
	assert.FileExists(t, filepath.Join(stubDir, modelA+".json"))
	assert.FileExists(t, filepath.Join(stubDir, modelB+".json"))

	contentB := readJSONFile(t, filepath.Join(stubDir, modelB+".json"))
	assert.Equal(t, modelB, contentB["modelId"])
}

// TestPostDeletePost_TransitionExpired_SameResource verifies the full cycle:
// POST A → wait for transition → DELETE A → POST A again.
// The existing ResetMatch fix handles the DELETE reset, but this test ensures
// the combination of transition expiry + delete + re-create works end-to-end.
func TestPostDeletePost_TransitionExpired_SameResource(t *testing.T) {
	dir := t.TempDir()
	stubDir := filepath.Join(dir, "stubs", "batch-configs")
	defaultsDir := filepath.Join(dir, "stubs", "defaults")
	require.NoError(t, os.MkdirAll(stubDir, 0755))
	require.NoError(t, os.MkdirAll(defaultsDir, 0755))

	require.NoError(t, os.WriteFile(
		filepath.Join(defaultsDir, "batch-config.json"),
		[]byte(`{"status": {"ready": false, "inProgress": true}}`),
		0644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(defaultsDir, "batch-config-ready.json"),
		[]byte(`{"status": {"ready": true, "inProgress": false}}`),
		0644,
	))

	cfg := &config.Config{
		Routes: []config.Route{
			{
				Method:   "POST",
				Match:    "/api/v1/*/environments/*/model/{modelId}/batchConfig",
				Fallback: "created",
				Transitions: []config.Transition{
					{Case: "creating", Duration: 1},
					{Case: "ready"},
				},
				Cases: map[string]config.Case{
					"created": {
						Status:   201,
						File:     "stubs/batch-configs/",
						Persist:  true,
						Merge:    "append",
						Key:      "modelId",
						Defaults: "stubs/defaults/batch-config.json",
					},
					"ready": {
						Persist:  true,
						Merge:    "update",
						File:     "stubs/batch-configs/{path.modelId}.json",
						Defaults: "stubs/defaults/batch-config-ready.json",
					},
				},
			},
			{
				Method:   "DELETE",
				Match:    "/api/v1/*/environments/*/model/{modelId}/batchConfig",
				Fallback: "deleted",
				Cases: map[string]config.Case{
					"deleted": {
						Status:  200,
						File:    "stubs/batch-configs/{path.modelId}.json",
						Persist: true,
						Merge:   "delete",
					},
				},
			},
		},
	}

	loader := &stubConfigLoader{cfg: cfg, configDir: dir}
	ts := newTransitionState()
	handler := NewHandlerWithTransitions(loader, nil, false, "", ts, nil, nil)

	modelId := "8979323336441987072"
	basePath := "/api/v1/org123/environments/env456/model/" + modelId + "/batchConfig"
	createdFile := filepath.Join(stubDir, modelId+".json")

	// POST creates the resource.
	body1 := []byte(`{"rateLimitCount": 100}`)
	req1 := httptest.NewRequest(http.MethodPost, basePath, bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusCreated, w1.Code)
	require.FileExists(t, createdFile)

	// Wait for transition to reach terminal "ready" state.
	time.Sleep(1100 * time.Millisecond)

	// DELETE removes the resource and resets transitions.
	req2 := httptest.NewRequest(http.MethodDelete, basePath, nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
	require.NoFileExists(t, createdFile)

	// POST again — should use fallback "created" (append).
	body3 := []byte(`{"rateLimitCount": 300}`)
	req3 := httptest.NewRequest(http.MethodPost, basePath, bytes.NewReader(body3))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusCreated, w3.Code,
		"POST after DELETE+transition should return 201; got %d; body: %s",
		w3.Code, w3.Body.String())
	assert.FileExists(t, createdFile)
}
