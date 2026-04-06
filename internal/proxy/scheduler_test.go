package proxy

import (
	"bytes"
	"context"
	"encoding/json"
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

// stubConfigLoader is a minimal configLoader implementation for handler tests.
type stubConfigLoader struct {
	cfg       *config.Config
	configDir string
}

func (s *stubConfigLoader) Get() *config.Config         { return s.cfg }
func (s *stubConfigLoader) AddRoute(route config.Route) {}
func (s *stubConfigLoader) ConfigDir() string           { return s.configDir }

// writeJSONFile writes a map to a JSON file.
func writeJSONFile(t *testing.T, path string, data map[string]interface{}) {
	t.Helper()
	b, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, b, 0644))
}

// waitForStatus polls the stub file until the "status" field matches want
// or the timeout expires.
func waitForStatus(t *testing.T, path, field, want string) {
	t.Helper()
	require.Eventually(t, func() bool {
		data := readJSONFile(t, path)
		return data[field] == want
	}, 3*time.Second, 50*time.Millisecond,
		"expected %s=%q in %s", field, want, filepath.Base(path))
}

func TestScheduler_FiresAfterDuration(t *testing.T) {
	dir := t.TempDir()

	stubFile := filepath.Join(dir, "dep-123.json")
	writeJSONFile(t, stubFile, map[string]interface{}{
		"id":     "dep-123",
		"status": "Deploying",
	})

	defaultsFile := filepath.Join(dir, "ready-defaults.json")
	require.NoError(t, os.WriteFile(defaultsFile, []byte(`{"status": "Ready"}`), 0644))

	route := &config.Route{
		Method:   "POST",
		Match:    "/deployments",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "deploying", Duration: 1},
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"ready":   {Persist: true, Merge: "update", Defaults: "ready-defaults.json"},
		},
	}

	sched := newTransitionScheduler(context.Background())
	defer sched.Stop()

	sched.Schedule(route, stubFile, dir, nil)

	// Verify file is still "Deploying" immediately.
	data := readJSONFile(t, stubFile)
	assert.Equal(t, "Deploying", data["status"], "should still be Deploying immediately")

	// Poll until the transition fires.
	waitForStatus(t, stubFile, "status", "Ready")

	data = readJSONFile(t, stubFile)
	assert.Equal(t, "dep-123", data["id"], "original fields should be preserved")
}

func TestScheduler_CancelledOnStop(t *testing.T) {
	dir := t.TempDir()

	stubFile := filepath.Join(dir, "resource.json")
	writeJSONFile(t, stubFile, map[string]interface{}{"status": "initial"})

	defaultsFile := filepath.Join(dir, "update.json")
	require.NoError(t, os.WriteFile(defaultsFile, []byte(`{"status": "updated"}`), 0644))

	route := &config.Route{
		Method:   "POST",
		Match:    "/resources",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "initial", Duration: 5},
			{Case: "updated"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"updated": {Persist: true, Merge: "update", Defaults: "update.json"},
		},
	}

	sched := newTransitionScheduler(context.Background())
	sched.Schedule(route, stubFile, dir, nil)

	// Stop immediately — should cancel the pending mutation.
	sched.Stop()

	data := readJSONFile(t, stubFile)
	assert.Equal(t, "initial", data["status"], "mutation should have been cancelled")
}

func TestScheduler_CancelledOnReset(t *testing.T) {
	dir := t.TempDir()

	stubFile := filepath.Join(dir, "resource.json")
	writeJSONFile(t, stubFile, map[string]interface{}{"status": "initial"})

	defaultsFile := filepath.Join(dir, "update.json")
	require.NoError(t, os.WriteFile(defaultsFile, []byte(`{"status": "updated"}`), 0644))

	route := &config.Route{
		Method:   "POST",
		Match:    "/resources",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "initial", Duration: 5},
			{Case: "updated"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"updated": {Persist: true, Merge: "update", Defaults: "update.json"},
		},
	}

	sched := newTransitionScheduler(context.Background())
	sched.Schedule(route, stubFile, dir, nil)

	// Reset (simulating hot-reload) — should cancel the pending mutation.
	sched.Reset(context.Background())

	data := readJSONFile(t, stubFile)
	assert.Equal(t, "initial", data["status"], "mutation should have been cancelled on reset")

	// Scheduler should still be usable after reset.
	sched.Stop()
}

func TestScheduler_MultipleTransitions(t *testing.T) {
	dir := t.TempDir()

	stubFile := filepath.Join(dir, "item.json")
	writeJSONFile(t, stubFile, map[string]interface{}{
		"id":    "item-1",
		"phase": "building",
	})

	phase2File := filepath.Join(dir, "phase2.json")
	require.NoError(t, os.WriteFile(phase2File, []byte(`{"phase": "testing"}`), 0644))

	phase3File := filepath.Join(dir, "phase3.json")
	require.NoError(t, os.WriteFile(phase3File, []byte(`{"phase": "done"}`), 0644))

	// Three states: building (1s) → testing (1s) → done (terminal)
	route := &config.Route{
		Method:   "POST",
		Match:    "/items",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "building", Duration: 1},
			{Case: "testing", Duration: 1},
			{Case: "done"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"testing": {Persist: true, Merge: "update", Defaults: "phase2.json"},
			"done":    {Persist: true, Merge: "update", Defaults: "phase3.json"},
		},
	}

	sched := newTransitionScheduler(context.Background())
	defer sched.Stop()

	sched.Schedule(route, stubFile, dir, nil)

	// At t=0, should be "building".
	data := readJSONFile(t, stubFile)
	assert.Equal(t, "building", data["phase"])

	// Poll until "testing" (first transition at t=1s).
	waitForStatus(t, stubFile, "phase", "testing")

	// Poll until "done" (second transition at t=2s).
	waitForStatus(t, stubFile, "phase", "done")

	data = readJSONFile(t, stubFile)
	assert.Equal(t, "item-1", data["id"], "original fields should be preserved")
}

func TestScheduler_NoPersistCaseSkipped(t *testing.T) {
	dir := t.TempDir()

	stubFile := filepath.Join(dir, "resource.json")
	writeJSONFile(t, stubFile, map[string]interface{}{"status": "initial"})

	defaultsFile := filepath.Join(dir, "update.json")
	require.NoError(t, os.WriteFile(defaultsFile, []byte(`{"status": "should-not-appear"}`), 0644))

	// The "updated" case does NOT have persist=true — it should be skipped.
	route := &config.Route{
		Method:   "POST",
		Match:    "/resources",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "initial", Duration: 1},
			{Case: "updated"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"updated": {Persist: false, Merge: "update", Defaults: "update.json"},
		},
	}

	sched := newTransitionScheduler(context.Background())
	defer sched.Stop()

	sched.Schedule(route, stubFile, dir, nil)

	// Give enough time for it to have fired if it were scheduled.
	time.Sleep(1500 * time.Millisecond)

	data := readJSONFile(t, stubFile)
	assert.Equal(t, "initial", data["status"], "non-persist case should not trigger mutation")
}

func TestScheduler_NoDefaultsSkipped(t *testing.T) {
	dir := t.TempDir()

	stubFile := filepath.Join(dir, "resource.json")
	writeJSONFile(t, stubFile, map[string]interface{}{"status": "initial"})

	// The "updated" case has persist=true but NO defaults — nothing to merge.
	route := &config.Route{
		Method:   "POST",
		Match:    "/resources",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "initial", Duration: 1},
			{Case: "updated"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"updated": {Persist: true, Merge: "update", Defaults: ""},
		},
	}

	sched := newTransitionScheduler(context.Background())
	defer sched.Stop()

	sched.Schedule(route, stubFile, dir, nil)

	time.Sleep(1500 * time.Millisecond)

	data := readJSONFile(t, stubFile)
	assert.Equal(t, "initial", data["status"], "no-defaults case should not trigger mutation")
}

func TestScheduler_DeletedFileBeforeTransition(t *testing.T) {
	dir := t.TempDir()

	stubFile := filepath.Join(dir, "resource.json")
	writeJSONFile(t, stubFile, map[string]interface{}{"status": "initial"})

	defaultsFile := filepath.Join(dir, "update.json")
	require.NoError(t, os.WriteFile(defaultsFile, []byte(`{"status": "updated"}`), 0644))

	route := &config.Route{
		Method:   "POST",
		Match:    "/resources",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "initial", Duration: 1},
			{Case: "updated"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"updated": {Persist: true, Merge: "update", Defaults: "update.json"},
		},
	}

	sched := newTransitionScheduler(context.Background())
	defer sched.Stop()

	sched.Schedule(route, stubFile, dir, nil)

	// Delete the file before the transition fires.
	require.NoError(t, os.Remove(stubFile))

	// Wait past the transition time; the file should not be recreated.
	time.Sleep(1500 * time.Millisecond)
	assert.NoFileExists(t, stubFile, "deleted file should not be recreated")
}

func TestScheduler_NonUpdateMergeSkipped(t *testing.T) {
	dir := t.TempDir()

	stubFile := filepath.Join(dir, "resource.json")
	writeJSONFile(t, stubFile, map[string]interface{}{"status": "initial"})

	defaultsFile := filepath.Join(dir, "update.json")
	require.NoError(t, os.WriteFile(defaultsFile, []byte(`{"status": "appended"}`), 0644))

	// The "updated" case has merge="append" instead of "update" — should be skipped.
	route := &config.Route{
		Method:   "POST",
		Match:    "/resources",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "initial", Duration: 1},
			{Case: "updated"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"updated": {Persist: true, Merge: "append", Defaults: "update.json"},
		},
	}

	sched := newTransitionScheduler(context.Background())
	defer sched.Stop()

	sched.Schedule(route, stubFile, dir, nil)

	time.Sleep(1500 * time.Millisecond)

	data := readJSONFile(t, stubFile)
	assert.Equal(t, "initial", data["status"], "non-update merge should not trigger mutation")
}

// ---------------------------------------------------------------------------
// Full integration: PATCH → background transition → GET reads updated file
// ---------------------------------------------------------------------------

func TestPatchUpdatesAndTransitionsInBackground(t *testing.T) {
	dir := t.TempDir()

	itemsDir := filepath.Join(dir, "items")
	require.NoError(t, os.MkdirAll(itemsDir, 0755))

	defaultsDir := filepath.Join(dir, "defaults")
	require.NoError(t, os.MkdirAll(defaultsDir, 0755))

	// Seed a pre-existing item file (PATCH operates on existing resources).
	writeJSONFile(t, filepath.Join(itemsDir, "item-1.json"), map[string]interface{}{
		"itemId": "item-1",
		"name":   "widget",
		"status": map[string]interface{}{"ready": true, "inProgress": false, "message": "Active"},
	})

	require.NoError(t, os.WriteFile(
		filepath.Join(defaultsDir, "item-updating.json"),
		[]byte(`{"status": {"ready": false, "inProgress": true, "message": "Updating"}}`), 0644))

	require.NoError(t, os.WriteFile(
		filepath.Join(defaultsDir, "item-ready.json"),
		[]byte(`{"status": {"ready": true, "inProgress": false, "message": "Active"}}`), 0644))

	cfg := &config.Config{
		Routes: []config.Route{
			{
				Method:   "PATCH",
				Match:    "/api/items/{itemId}",
				Fallback: "updated",
				Transitions: []config.Transition{
					{Case: "updating", Duration: 1},
					{Case: "ready"},
				},
				Cases: map[string]config.Case{
					"updated": {
						Status: 200, File: "items/{path.itemId}.json",
						Persist: true, Merge: "update",
						Defaults: "defaults/item-updating.json",
					},
					"updating": {
						Persist: true, Merge: "update",
						File:     "items/{path.itemId}.json",
						Defaults: "defaults/item-updating.json",
					},
					"ready": {
						Persist: true, Merge: "update",
						File:     "items/{path.itemId}.json",
						Defaults: "defaults/item-ready.json",
					},
				},
			},
			{
				Method: "GET", Fallback: "success",
				Match: "/api/items/{itemId}",
				Cases: map[string]config.Case{
					"success": {Status: 200, File: "items/{path.itemId}.json"},
				},
			},
		},
	}

	loader := &stubConfigLoader{cfg: cfg, configDir: dir}
	sched := newTransitionScheduler(context.Background())
	defer sched.Stop()

	handler := NewHandlerWithTransitions(loader, nil, false, "", newTransitionState(), sched, nil)

	// --- PATCH updates the item ---

	patchBody := bytes.NewBufferString(`{"name": "updated-widget"}`)
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/items/item-1", patchBody)
	patchReq.Header.Set("Content-Type", "application/json")
	patchW := httptest.NewRecorder()
	handler.ServeHTTP(patchW, patchReq)

	require.Equal(t, http.StatusOK, patchW.Code)
	var patchResp map[string]interface{}
	require.NoError(t, json.Unmarshal(patchW.Body.Bytes(), &patchResp))
	// The PATCH response should have the updating defaults applied.
	statusMap, ok := patchResp["status"].(map[string]interface{})
	require.True(t, ok, "status should be a map")
	assert.Equal(t, false, statusMap["ready"])
	assert.Equal(t, true, statusMap["inProgress"])
	assert.Equal(t, "Updating", statusMap["message"])
	assert.Equal(t, "updated-widget", patchResp["name"])

	itemFile := filepath.Join(itemsDir, "item-1.json")

	// --- GET immediately → should be Updating ---

	getW := httptest.NewRecorder()
	handler.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "/api/items/item-1", nil))
	require.Equal(t, http.StatusOK, getW.Code)
	var getResp map[string]interface{}
	require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &getResp))
	statusMap = getResp["status"].(map[string]interface{})
	assert.Equal(t, "Updating", statusMap["message"])

	// --- Wait for background transition (should fire after ~1s) ---

	// Poll the file until status.message becomes "Active" (the "ready" transition).
	require.Eventually(t, func() bool {
		data := readJSONFile(t, itemFile)
		s, ok := data["status"].(map[string]interface{})
		if !ok {
			return false
		}
		return s["message"] == "Active"
	}, 3*time.Second, 50*time.Millisecond,
		"expected status.message=\"Active\" after background transition")

	// --- GET after transition → should be Active/Ready ---

	getW2 := httptest.NewRecorder()
	handler.ServeHTTP(getW2, httptest.NewRequest(http.MethodGet, "/api/items/item-1", nil))
	require.Equal(t, http.StatusOK, getW2.Code)
	var getResp2 map[string]interface{}
	require.NoError(t, json.Unmarshal(getW2.Body.Bytes(), &getResp2))
	statusMap2 := getResp2["status"].(map[string]interface{})
	assert.Equal(t, true, statusMap2["ready"])
	assert.Equal(t, false, statusMap2["inProgress"])
	assert.Equal(t, "Active", statusMap2["message"])
	// Original fields should be preserved.
	assert.Equal(t, "item-1", getResp2["itemId"])
	assert.Equal(t, "updated-widget", getResp2["name"])
}

// ---------------------------------------------------------------------------
// Full integration: POST → background transition → GET reads updated file
// ---------------------------------------------------------------------------

func TestPostCreatesAndTransitionsInBackground(t *testing.T) {
	dir := t.TempDir()

	deploymentsDir := filepath.Join(dir, "deployments", "ep-123")
	require.NoError(t, os.MkdirAll(deploymentsDir, 0755))

	defaultsDir := filepath.Join(dir, "defaults")
	require.NoError(t, os.MkdirAll(defaultsDir, 0755))

	require.NoError(t, os.WriteFile(
		filepath.Join(defaultsDir, "deployment.json"),
		[]byte(`{"status": "Deploying", "region": "us-east-1"}`), 0644))

	require.NoError(t, os.WriteFile(
		filepath.Join(defaultsDir, "deployment-ready.json"),
		[]byte(`{"status": "Ready"}`), 0644))

	cfg := &config.Config{
		Routes: []config.Route{
			{
				Method:   "POST",
				Match:    "/api/v1/*/environments/*/endpoint/{endpointId}/deployment",
				Fallback: "created",
				Transitions: []config.Transition{
					{Case: "deploying", Duration: 1},
					{Case: "ready"},
				},
				Cases: map[string]config.Case{
					"created": {
						Status: 201, File: "deployments/{path.endpointId}/",
						Persist: true, Merge: "append", Key: "deploymentId",
						Defaults: "defaults/deployment.json",
					},
					"ready": {
						Persist: true, Merge: "update",
						Defaults: "defaults/deployment-ready.json",
					},
				},
			},
			{
				Method: "GET", Fallback: "success",
				Match: "/api/v1/*/environments/*/endpoint/{endpointId}/deployment/{deploymentId}",
				Cases: map[string]config.Case{
					"success": {Status: 200, File: "deployments/{path.endpointId}/{path.deploymentId}.json"},
				},
			},
			{
				Method: "GET", Fallback: "success",
				Match: "/api/v1/*/environments/*/endpoint/{endpointId}/deployment",
				Cases: map[string]config.Case{
					"success": {Status: 200, File: "deployments/{path.endpointId}/"},
				},
			},
		},
	}

	loader := &stubConfigLoader{cfg: cfg, configDir: dir}
	sched := newTransitionScheduler(context.Background())
	defer sched.Stop()

	handler := NewHandlerWithTransitions(loader, nil, false, "", newTransitionState(), sched, nil)

	// --- POST creates the deployment ---

	postBody := bytes.NewBufferString(`{"deploymentId": "dep-456", "name": "my-deployment"}`)
	postReq := httptest.NewRequest(http.MethodPost,
		"/api/v1/org1/environments/staging/endpoint/ep-123/deployment", postBody)
	postReq.Header.Set("Content-Type", "application/json")
	postW := httptest.NewRecorder()
	handler.ServeHTTP(postW, postReq)

	require.Equal(t, http.StatusCreated, postW.Code)
	var postResp map[string]interface{}
	require.NoError(t, json.Unmarshal(postW.Body.Bytes(), &postResp))
	assert.Equal(t, "Deploying", postResp["status"])
	assert.Equal(t, "dep-456", postResp["deploymentId"])

	createdFile := filepath.Join(deploymentsDir, "dep-456.json")
	require.FileExists(t, createdFile)

	// --- GET immediately → Deploying ---

	getW := httptest.NewRecorder()
	handler.ServeHTTP(getW, httptest.NewRequest(http.MethodGet,
		"/api/v1/org1/environments/staging/endpoint/ep-123/deployment/dep-456", nil))
	require.Equal(t, http.StatusOK, getW.Code)
	var getResp map[string]interface{}
	require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &getResp))
	assert.Equal(t, "Deploying", getResp["status"])

	// --- Wait for background transition ---

	waitForStatus(t, createdFile, "status", "Ready")

	// --- GET by ID → Ready ---

	getW2 := httptest.NewRecorder()
	handler.ServeHTTP(getW2, httptest.NewRequest(http.MethodGet,
		"/api/v1/org1/environments/staging/endpoint/ep-123/deployment/dep-456", nil))
	require.Equal(t, http.StatusOK, getW2.Code)
	var getResp2 map[string]interface{}
	require.NoError(t, json.Unmarshal(getW2.Body.Bytes(), &getResp2))
	assert.Equal(t, "Ready", getResp2["status"])
	assert.Equal(t, "dep-456", getResp2["deploymentId"])
	assert.Equal(t, "my-deployment", getResp2["name"])

	// --- GET list → also Ready ---

	listW := httptest.NewRecorder()
	handler.ServeHTTP(listW, httptest.NewRequest(http.MethodGet,
		"/api/v1/org1/environments/staging/endpoint/ep-123/deployment", nil))
	require.Equal(t, http.StatusOK, listW.Code)
	var listResp []map[string]interface{}
	require.NoError(t, json.Unmarshal(listW.Body.Bytes(), &listResp))
	require.Len(t, listResp, 1)
	assert.Equal(t, "Ready", listResp[0]["status"])
}
