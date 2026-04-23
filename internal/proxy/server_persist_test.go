package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	apitwinruntime "github.com/ridakaddir/apitwin/internal/runtime"
	runtimepersist "github.com/ridakaddir/apitwin/internal/runtime/persist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verifies the NewServer bootstrap threads persistence end-to-end:
// transitions.json and scheduled.json at the meta dir are loaded, and
// runtime-only files survive a second NewServer call (overlay semantics).

func TestNewServer_OverlayPreservesRuntimeOnlyStubs(t *testing.T) {
	configDir := t.TempDir()
	// Minimal valid config.
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "apitwin.toml"),
		[]byte(`[[routes]]
method = "GET"
match = "/ping"
fallback = "ok"
[routes.cases.ok]
status = 200
`), 0644))

	srv1, err := NewServer(ServerOptions{ConfigPath: configDir, Port: 0})
	require.NoError(t, err)

	runtimeDir := srv1.RuntimeDir()
	require.NotEmpty(t, runtimeDir)

	// Simulate a runtime POST creating a stub that does not exist in seed.
	runtimeOnly := filepath.Join(runtimeDir, "stubs", "continents", "runtime-only.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(runtimeOnly), 0755))
	require.NoError(t, os.WriteFile(runtimeOnly, []byte(`{"id":"runtime-only"}`), 0644))

	// Release resources so the next NewServer can touch the same paths.
	srv1.loader.Close()
	srv1.stubWatcher.Stop()
	srv1.scheduler.Stop()

	// Second NewServer on the same config dir — runtime-only file must
	// still be there after OverlaySeed.
	srv2, err := NewServer(ServerOptions{ConfigPath: configDir, Port: 0})
	require.NoError(t, err)
	defer func() {
		srv2.loader.Close()
		srv2.stubWatcher.Stop()
		srv2.scheduler.Stop()
	}()

	_, err = os.Stat(runtimeOnly)
	assert.NoError(t, err, "runtime-only stub should survive a restart")
}

func TestNewServer_ResetRuntimeWipesRuntimeOnlyStubs(t *testing.T) {
	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "apitwin.toml"),
		[]byte(`[[routes]]
method = "GET"
match = "/ping"
fallback = "ok"
[routes.cases.ok]
status = 200
`), 0644))

	srv1, err := NewServer(ServerOptions{ConfigPath: configDir, Port: 0})
	require.NoError(t, err)

	runtimeDir := srv1.RuntimeDir()
	runtimeOnly := filepath.Join(runtimeDir, "stubs", "only.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(runtimeOnly), 0755))
	require.NoError(t, os.WriteFile(runtimeOnly, []byte(`{}`), 0644))

	srv1.loader.Close()
	srv1.stubWatcher.Stop()
	srv1.scheduler.Stop()

	// Reset via the flag — runtime-only file must be gone.
	srv2, err := NewServer(ServerOptions{ConfigPath: configDir, Port: 0, ResetRuntime: true})
	require.NoError(t, err)
	defer func() {
		srv2.loader.Close()
		srv2.stubWatcher.Stop()
		srv2.scheduler.Stop()
	}()

	_, err = os.Stat(runtimeOnly)
	assert.True(t, os.IsNotExist(err), "runtime-only stub should be wiped by --reset-runtime")
}

func TestNewServer_RearmsPersistedScheduleOnStart(t *testing.T) {
	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "apitwin.toml"),
		[]byte(`[[routes]]
method = "GET"
match = "/ping"
fallback = "ok"
[routes.cases.ok]
status = 200
`), 0644))

	// Bootstrap once to establish the runtime dir.
	srv1, err := NewServer(ServerOptions{ConfigPath: configDir, Port: 0})
	require.NoError(t, err)
	runtimeDir := srv1.RuntimeDir()
	srv1.loader.Close()
	srv1.stubWatcher.Stop()
	srv1.scheduler.Stop()

	// Prime a past-due persisted mutation. It should be applied
	// synchronously during the next NewServer bootstrap.
	targetFile := filepath.Join(runtimeDir, "stubs", "primed.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(targetFile), 0755))
	require.NoError(t, os.WriteFile(targetFile, []byte(`{"status":"Pending"}`), 0644))

	metaDir := apitwinruntime.MetaDir(runtimeDir)
	require.NoError(t, os.MkdirAll(metaDir, 0755))
	schedPersister := runtimepersist.NewScheduledPersister(
		filepath.Join(metaDir, "scheduled.json"), runtimeDir)

	payload, _ := json.Marshal(map[string]interface{}{"status": "Ready"})
	require.NoError(t, schedPersister.Add(runtimepersist.ScheduledItem{
		ID:              "past-due-restart",
		Scope:           "rest",
		RouteKey:        "POST /primed",
		FilePath:        targetFile,
		FireAt:          time.Now().Add(-5 * time.Second),
		ResolvedPayload: payload,
	}))

	srv2, err := NewServer(ServerOptions{ConfigPath: configDir, Port: 0})
	require.NoError(t, err)
	defer func() {
		srv2.loader.Close()
		srv2.stubWatcher.Stop()
		srv2.scheduler.Stop()
	}()

	// After NewServer returns (it calls Rearm internally), the past-due
	// mutation should have been applied.
	data, err := os.ReadFile(targetFile)
	require.NoError(t, err)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "Ready", got["status"], "past-due rearm should apply at NewServer bootstrap")

	// And been removed from the persisted schedule.
	f, err := schedPersister.Load()
	require.NoError(t, err)
	assert.Empty(t, f.Items, "rearmed past-due item should be pruned")
}
