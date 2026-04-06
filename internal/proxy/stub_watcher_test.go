package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ridakaddir/apitwin/internal/persist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupWatcherTest creates a temp directory with the given file structure
// and returns the configDir path. Automatically cleans up persist listeners.
func setupWatcherTest(t *testing.T, files map[string]string) string {
	t.Helper()
	persist.ResetListeners()
	t.Cleanup(func() { persist.ResetListeners() })

	dir := t.TempDir()
	for relPath, content := range files {
		absPath := filepath.Join(dir, relPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(absPath), 0755))
		require.NoError(t, os.WriteFile(absPath, []byte(content), 0644))
	}
	return dir
}

// newTestWatcher creates a StubWatcher and registers t.Cleanup(sw.Stop) to
// ensure timers and listeners are cleaned up after the test.
func newTestWatcher(t *testing.T, opts StubWatcherOptions) *StubWatcher {
	t.Helper()
	sw := NewStubWatcher(opts)
	t.Cleanup(sw.Stop)
	return sw
}

func TestStubWatcher_DiscoversDependencies(t *testing.T) {
	dir := setupWatcherTest(t, map[string]string{
		"stubs/endpoints/ep-123.json": `{
			"endpointId": "ep-123",
			"deployedModels": "{{ref:stubs/deployments/{.endpointId}/?template=stubs/templates/deployed-model.json}}"
		}`,
		"stubs/endpoints/ep-456.json": `{
			"endpointId": "ep-456",
			"deployedModels": "{{ref:stubs/deployments/{.endpointId}/?template=stubs/templates/deployed-model.json}}"
		}`,
		"stubs/deployments/ep-123/dep-1.json": `{"deploymentId": "dep-1", "status": "Ready"}`,
		"stubs/templates/deployed-model.json": `{"id": "{{.deploymentId}}"}`,
	})

	sw := newTestWatcher(t, StubWatcherOptions{
		ConfigDir: dir,
	})

	// Should discover one dependency pattern: stubs/deployments/*/
	assert.Equal(t, 1, sw.DependencyCount(), "should discover one directory dependency pattern")

	// Both endpoint files should be tracked.
	deps := sw.DumpDeps()
	for _, files := range deps {
		assert.Len(t, files, 2, "both endpoint files should depend on the deployment pattern")
	}
}

func TestStubWatcher_NoDependencies(t *testing.T) {
	dir := setupWatcherTest(t, map[string]string{
		"stubs/endpoints/ep-123.json": `{"endpointId": "ep-123", "name": "test"}`,
	})

	sw := newTestWatcher(t, StubWatcherOptions{
		ConfigDir: dir,
	})

	assert.Equal(t, 0, sw.DependencyCount(), "should find no dependencies for plain files")
}

func TestStubWatcher_IgnoresFileRefs(t *testing.T) {
	// Refs to single files (not ending with /) should not be tracked
	// because they are immutable references, not directory aggregations.
	dir := setupWatcherTest(t, map[string]string{
		"stubs/data.json": `{
			"related": "{{ref:stubs/other.json}}"
		}`,
		"stubs/other.json": `{"key": "value"}`,
	})

	sw := newTestWatcher(t, StubWatcherOptions{
		ConfigDir: dir,
	})

	assert.Equal(t, 0, sw.DependencyCount(), "should not track single-file refs")
}

func TestStubWatcher_NotifiesOnDependencyChange(t *testing.T) {
	dir := setupWatcherTest(t, map[string]string{
		"stubs/endpoints/ep-123.json": `{
			"endpointId": "ep-123",
			"deployedModels": "{{ref:stubs/deployments/{.endpointId}/?template=stubs/templates/deployed-model.json}}"
		}`,
		"stubs/deployments/ep-123/.gitkeep": "",
	})

	var updateCount atomic.Int32

	sw := newTestWatcher(t, StubWatcherOptions{
		ConfigDir:   dir,
		OnUpdate:    func() { updateCount.Add(1) },
		BatchWindow: 50 * time.Millisecond,
	})
	_ = sw

	// Simulate creating a deployment file via persist.
	deployDir := filepath.Join(dir, "stubs", "deployments", "ep-123")
	_, _, err := persist.AppendToDir(deployDir, "deploymentId", map[string]interface{}{
		"deploymentId": "dep-new",
		"status":       "Deploying",
	})
	require.NoError(t, err)

	// Wait for batch window + processing.
	require.Eventually(t, func() bool {
		return updateCount.Load() > 0
	}, 2*time.Second, 25*time.Millisecond,
		"onUpdate should be called when a dependency directory changes")
}

func TestStubWatcher_NotifiesOnUpdate(t *testing.T) {
	dir := setupWatcherTest(t, map[string]string{
		"stubs/endpoints/ep-123.json": `{
			"endpointId": "ep-123",
			"deployedModels": "{{ref:stubs/deployments/{.endpointId}/?template=stubs/templates/deployed-model.json}}"
		}`,
	})

	deployDir := filepath.Join(dir, "stubs", "deployments", "ep-123")
	require.NoError(t, os.MkdirAll(deployDir, 0755))

	// Create initial deployment file.
	deployFile := filepath.Join(deployDir, "dep-1.json")
	require.NoError(t, os.WriteFile(deployFile, []byte(`{"deploymentId":"dep-1","status":"Deploying"}`), 0644))

	var updateCount atomic.Int32
	sw := newTestWatcher(t, StubWatcherOptions{
		ConfigDir:   dir,
		OnUpdate:    func() { updateCount.Add(1) },
		BatchWindow: 50 * time.Millisecond,
	})
	_ = sw

	// Simulate updating the deployment (e.g. transition to Ready).
	_, err := persist.Update(deployFile, map[string]interface{}{"status": "Ready"})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return updateCount.Load() > 0
	}, 2*time.Second, 25*time.Millisecond,
		"onUpdate should be called when a deployment file is updated")
}

func TestStubWatcher_NotifiesOnDelete(t *testing.T) {
	dir := setupWatcherTest(t, map[string]string{
		"stubs/endpoints/ep-123.json": `{
			"endpointId": "ep-123",
			"deployedModels": "{{ref:stubs/deployments/{.endpointId}/?template=stubs/templates/deployed-model.json}}"
		}`,
	})

	deployDir := filepath.Join(dir, "stubs", "deployments", "ep-123")
	require.NoError(t, os.MkdirAll(deployDir, 0755))

	deployFile := filepath.Join(deployDir, "dep-1.json")
	require.NoError(t, os.WriteFile(deployFile, []byte(`{"deploymentId":"dep-1","status":"Ready"}`), 0644))

	var updateCount atomic.Int32
	sw := newTestWatcher(t, StubWatcherOptions{
		ConfigDir:   dir,
		OnUpdate:    func() { updateCount.Add(1) },
		BatchWindow: 50 * time.Millisecond,
	})
	_ = sw

	// Simulate deleting a deployment.
	err := persist.DeleteFile(deployFile)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return updateCount.Load() > 0
	}, 2*time.Second, 25*time.Millisecond,
		"onUpdate should be called when a deployment file is deleted")
}

func TestStubWatcher_BatchesMultipleEvents(t *testing.T) {
	dir := setupWatcherTest(t, map[string]string{
		"stubs/endpoints/ep-123.json": `{
			"endpointId": "ep-123",
			"deployedModels": "{{ref:stubs/deployments/{.endpointId}/?template=stubs/templates/deployed-model.json}}"
		}`,
	})

	deployDir := filepath.Join(dir, "stubs", "deployments", "ep-123")
	require.NoError(t, os.MkdirAll(deployDir, 0755))

	var updateCount atomic.Int32
	sw := newTestWatcher(t, StubWatcherOptions{
		ConfigDir:   dir,
		OnUpdate:    func() { updateCount.Add(1) },
		BatchWindow: 200 * time.Millisecond,
	})
	_ = sw

	// Rapidly create multiple deployment files.
	for i := 0; i < 5; i++ {
		_, _, err := persist.AppendToDir(deployDir, "deploymentId", map[string]interface{}{
			"deploymentId": fmt.Sprintf("dep-%d", i),
			"status":       "Deploying",
		})
		require.NoError(t, err)
	}

	// Wait for batch processing, then verify batching occurred.
	require.Eventually(t, func() bool {
		return updateCount.Load() >= 1
	}, 2*time.Second, 25*time.Millisecond,
		"should have at least one update call")

	// Should have batched all events into a single (or very few) update calls.
	count := updateCount.Load()
	assert.LessOrEqual(t, count, int32(2),
		"multiple rapid events should be batched into few update calls, got %d", count)
}

func TestStubWatcher_IgnoresUnrelatedChanges(t *testing.T) {
	dir := setupWatcherTest(t, map[string]string{
		"stubs/endpoints/ep-123.json": `{
			"endpointId": "ep-123",
			"deployedModels": "{{ref:stubs/deployments/{.endpointId}/?template=stubs/templates/deployed-model.json}}"
		}`,
	})

	// Create a models directory (not referenced by any endpoint).
	modelsDir := filepath.Join(dir, "stubs", "models")
	require.NoError(t, os.MkdirAll(modelsDir, 0755))

	var updateCount atomic.Int32
	sw := newTestWatcher(t, StubWatcherOptions{
		ConfigDir:   dir,
		OnUpdate:    func() { updateCount.Add(1) },
		BatchWindow: 50 * time.Millisecond,
	})
	_ = sw

	// Create a file in an unrelated directory.
	_, _, err := persist.AppendToDir(modelsDir, "modelId", map[string]interface{}{
		"modelId": "model-1",
		"name":    "test-model",
	})
	require.NoError(t, err)

	// Verify no update is triggered within a reasonable window.
	require.Never(t, func() bool {
		return updateCount.Load() > 0
	}, 200*time.Millisecond, 10*time.Millisecond,
		"changes in unrelated directories should not trigger updates")
}

func TestStubWatcher_AddFile_RegistersNewDependency(t *testing.T) {
	dir := setupWatcherTest(t, map[string]string{
		// Initially no endpoints with refs.
		"stubs/deployments/.gitkeep": "",
	})

	sw := newTestWatcher(t, StubWatcherOptions{
		ConfigDir: dir,
	})

	assert.Equal(t, 0, sw.DependencyCount(), "should start with no dependencies")

	// Simulate creating a new endpoint file with a ref.
	newEndpoint := filepath.Join(dir, "stubs", "endpoints", "ep-new.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(newEndpoint), 0755))
	require.NoError(t, os.WriteFile(newEndpoint, []byte(`{
		"endpointId": "ep-new",
		"deployedModels": "{{ref:stubs/deployments/{.endpointId}/?template=stubs/templates/deployed-model.json}}"
	}`), 0644))

	sw.AddFile(newEndpoint)

	assert.Equal(t, 1, sw.DependencyCount(),
		"AddFile should register the new dependency")
}

func TestStubWatcher_RemoveFile(t *testing.T) {
	dir := setupWatcherTest(t, map[string]string{
		"stubs/endpoints/ep-123.json": `{
			"endpointId": "ep-123",
			"deployedModels": "{{ref:stubs/deployments/{.endpointId}/?template=stubs/templates/deployed-model.json}}"
		}`,
	})

	sw := newTestWatcher(t, StubWatcherOptions{
		ConfigDir: dir,
	})

	require.Equal(t, 1, sw.DependencyCount())
	deps := sw.DumpDeps()
	for _, files := range deps {
		require.Len(t, files, 1)
	}

	sw.RemoveFile(filepath.Join(dir, "stubs", "endpoints", "ep-123.json"))

	// Pattern still exists but has no files.
	deps = sw.DumpDeps()
	for _, files := range deps {
		assert.Len(t, files, 0, "removed file should no longer be tracked")
	}
}

func TestStubWatcher_AffectedFiles(t *testing.T) {
	dir := setupWatcherTest(t, map[string]string{
		"stubs/endpoints/ep-123.json": `{
			"endpointId": "ep-123",
			"deployedModels": "{{ref:stubs/deployments/{.endpointId}/?template=stubs/templates/deployed-model.json}}"
		}`,
		"stubs/endpoints/ep-456.json": `{
			"endpointId": "ep-456",
			"deployedModels": "{{ref:stubs/deployments/{.endpointId}/?template=stubs/templates/deployed-model.json}}"
		}`,
	})

	sw := newTestWatcher(t, StubWatcherOptions{
		ConfigDir: dir,
	})

	// Both endpoints reference the same pattern, so changes to any
	// concrete deployment directory should affect both.
	affected := sw.AffectedFiles(filepath.Join(dir, "stubs", "deployments", "ep-123"))
	assert.Len(t, affected, 2, "both endpoint files should be affected")

	affected = sw.AffectedFiles(filepath.Join(dir, "stubs", "deployments", "ep-456"))
	assert.Len(t, affected, 2, "both endpoint files should be affected by any deployment dir")
}

func TestStubWatcher_ConcurrentAccess(t *testing.T) {
	dir := setupWatcherTest(t, map[string]string{
		"stubs/endpoints/ep-1.json": `{
			"deployedModels": "{{ref:stubs/deployments/{.endpointId}/}}"
		}`,
	})

	sw := newTestWatcher(t, StubWatcherOptions{
		ConfigDir:   dir,
		OnUpdate:    func() {},
		BatchWindow: 10 * time.Millisecond,
	})

	deployDir := filepath.Join(dir, "stubs", "deployments", "ep-1")
	require.NoError(t, os.MkdirAll(deployDir, 0755))

	// Concurrent operations should not race.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _, _ = persist.AppendToDir(deployDir, "id", map[string]interface{}{
				"id":     idx,
				"status": "test",
			})
		}(i)
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sw.DependencyCount()
			sw.DumpDeps()
		}()
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Full integration: endpoint → deployment hot-reload via StubWatcher
// ---------------------------------------------------------------------------

func TestStubWatcher_EndToEnd_DeploymentCreation(t *testing.T) {
	dir := setupWatcherTest(t, map[string]string{
		"stubs/endpoints/ep-123.json": `{
			"endpointId": "ep-123",
			"endpointDisplayName": "test-endpoint",
			"deployedModels": "{{ref:stubs/deployments/{.endpointId}/?template=stubs/templates/deployed-model.json}}"
		}`,
		"stubs/templates/deployed-model.json": `{
			"id": "{{.deploymentId}}",
			"displayName": "{{.modelDisplayName}}",
			"status": "{{.status}}"
		}`,
	})

	deployDir := filepath.Join(dir, "stubs", "deployments", "ep-123")
	require.NoError(t, os.MkdirAll(deployDir, 0755))

	var updateCalled atomic.Int32
	sw := newTestWatcher(t, StubWatcherOptions{
		ConfigDir:   dir,
		OnUpdate:    func() { updateCalled.Add(1) },
		BatchWindow: 50 * time.Millisecond,
	})
	_ = sw

	// 1. Read the endpoint — deployedModels should be empty array.
	endpointData, err := os.ReadFile(filepath.Join(dir, "stubs", "endpoints", "ep-123.json"))
	require.NoError(t, err)

	// The raw file still has the ref, but when the proxy resolves it,
	// the deployment directory is empty → empty array.
	assert.Contains(t, string(endpointData), "{{ref:")

	// 2. Create a deployment via persist.
	_, _, err = persist.AppendToDir(deployDir, "deploymentId", map[string]interface{}{
		"deploymentId":     "dep-001",
		"modelDisplayName": "my-model",
		"status":           "Deploying",
	})
	require.NoError(t, err)

	// 3. Verify the stub watcher detected the change.
	require.Eventually(t, func() bool {
		return updateCalled.Load() > 0
	}, 2*time.Second, 25*time.Millisecond,
		"stub watcher should detect deployment creation")

	// 4. Read the deployment directory directly to verify.
	deployData, err := persist.ReadDir(deployDir)
	require.NoError(t, err)

	var deployments []map[string]interface{}
	require.NoError(t, json.Unmarshal(deployData, &deployments))
	require.Len(t, deployments, 1)
	assert.Equal(t, "dep-001", deployments[0]["deploymentId"])

	// 5. Update deployment status (transition).
	deployFile := filepath.Join(deployDir, "dep-001.json")
	updateCalled.Store(0)

	_, err = persist.Update(deployFile, map[string]interface{}{"status": "Ready"})
	require.NoError(t, err)

	// 6. Verify the stub watcher detected the update.
	require.Eventually(t, func() bool {
		return updateCalled.Load() > 0
	}, 2*time.Second, 25*time.Millisecond,
		"stub watcher should detect deployment status update")

	// 7. Read updated deployment.
	updatedData := readJSONFile(t, deployFile)
	assert.Equal(t, "Ready", updatedData["status"])
}
