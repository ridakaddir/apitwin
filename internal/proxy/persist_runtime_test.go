package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRuntimeLoader is a configLoader where ConfigDir() and StubRoot() can
// differ, mirroring what NewServer does with a runtime mirror directory.
type stubRuntimeLoader struct {
	cfg        *config.Config
	configDir  string
	stubRoot   string
	addedRoute *config.Route
}

func (s *stubRuntimeLoader) Get() *config.Config { return s.cfg }
func (s *stubRuntimeLoader) AddRoute(route config.Route) {
	s.addedRoute = &route
}
func (s *stubRuntimeLoader) ConfigDir() string { return s.configDir }
func (s *stubRuntimeLoader) StubRoot() string {
	if s.stubRoot != "" {
		return s.stubRoot
	}
	return s.configDir
}

// TestRuntimeDir_AppendKeepsSeedClean verifies that when a handler is wired
// with a runtime mirror directory distinct from the seed dir, an append
// mutation lands in the runtime dir and the seed dir stays untouched —
// which is the whole point of the feature (git status stays clean).
func TestRuntimeDir_AppendKeepsSeedClean(t *testing.T) {
	seed := t.TempDir()
	runtimeDir := filepath.Join(seed, ".apitwin", "state")

	// Mirror both sides of the directory the route will write to.
	require.NoError(t, os.MkdirAll(filepath.Join(seed, "stubs", "items"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(runtimeDir, "stubs", "items"), 0755))

	cfg := &config.Config{
		Routes: []config.Route{{
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
		}},
	}

	loader := &stubRuntimeLoader{
		cfg:       cfg,
		configDir: seed,
		stubRoot:  runtimeDir,
	}
	handler := NewHandler(loader, nil, false, "")

	body := []byte(`{"id":"item-1","name":"First"}`)
	req := httptest.NewRequest(http.MethodPost, "/items", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	// Runtime dir gets the new record.
	runtimeFile := filepath.Join(runtimeDir, "stubs", "items", "item-1.json")
	assert.FileExists(t, runtimeFile, "new record should land in runtime dir")

	// Seed dir must NOT have the file — this is the whole fix.
	seedFile := filepath.Join(seed, "stubs", "items", "item-1.json")
	if _, err := os.Stat(seedFile); !os.IsNotExist(err) {
		t.Errorf("seed dir was dirtied: %q exists (err=%v)", seedFile, err)
	}

	// And the seed stubs dir should still be empty.
	entries, err := os.ReadDir(filepath.Join(seed, "stubs", "items"))
	require.NoError(t, err)
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("seed stubs/items should be empty, found: %v", names)
	}
}

// TestRuntimeDir_UpdateMutatesRuntimeOnly verifies that an update-merge
// mutation over a file that already exists in both seed and runtime writes
// only to the runtime copy — the seed copy stays byte-identical.
func TestRuntimeDir_UpdateMutatesRuntimeOnly(t *testing.T) {
	seed := t.TempDir()
	runtimeDir := filepath.Join(seed, ".apitwin", "state")

	// Same file exists in both trees (that's how a real mirror starts).
	require.NoError(t, os.MkdirAll(filepath.Join(seed, "stubs"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(runtimeDir, "stubs"), 0755))

	seedFile := filepath.Join(seed, "stubs", "user.json")
	runtimeFile := filepath.Join(runtimeDir, "stubs", "user.json")
	original := []byte(`{"name":"Alice","role":"user"}`)
	require.NoError(t, os.WriteFile(seedFile, original, 0644))
	require.NoError(t, os.WriteFile(runtimeFile, original, 0644))

	cfg := &config.Config{
		Routes: []config.Route{{
			Method:   "PATCH",
			Match:    "/user",
			Fallback: "updated",
			Cases: map[string]config.Case{
				"updated": {
					Status:  200,
					File:    "stubs/user.json",
					Persist: true,
					Merge:   "update",
				},
			},
		}},
	}

	loader := &stubRuntimeLoader{
		cfg:       cfg,
		configDir: seed,
		stubRoot:  runtimeDir,
	}
	handler := NewHandler(loader, nil, false, "")

	body := []byte(`{"role":"admin"}`)
	req := httptest.NewRequest(http.MethodPatch, "/user", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	// Seed copy must be byte-identical to the original.
	seedBytes, err := os.ReadFile(seedFile)
	require.NoError(t, err)
	assert.Equal(t, string(original), string(seedBytes), "seed file must not be mutated")

	// Runtime copy must reflect the PATCH.
	runtimeMap := readJSONFile(t, runtimeFile)
	assert.Equal(t, "admin", runtimeMap["role"])
	assert.Equal(t, "Alice", runtimeMap["name"])
}

// TestRuntimeDir_DeleteLeavesSeedIntact verifies that a delete-merge
// operation targets the runtime copy only — the seed copy survives the
// delete and re-runs will still have it available.
func TestRuntimeDir_DeleteLeavesSeedIntact(t *testing.T) {
	seed := t.TempDir()
	runtimeDir := filepath.Join(seed, ".apitwin", "state")

	require.NoError(t, os.MkdirAll(filepath.Join(seed, "stubs", "items"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(runtimeDir, "stubs", "items"), 0755))

	seedFile := filepath.Join(seed, "stubs", "items", "gone.json")
	runtimeFile := filepath.Join(runtimeDir, "stubs", "items", "gone.json")
	require.NoError(t, os.WriteFile(seedFile, []byte(`{"id":"gone"}`), 0644))
	require.NoError(t, os.WriteFile(runtimeFile, []byte(`{"id":"gone"}`), 0644))

	cfg := &config.Config{
		Routes: []config.Route{{
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
		}},
	}

	loader := &stubRuntimeLoader{
		cfg:       cfg,
		configDir: seed,
		stubRoot:  runtimeDir,
	}
	handler := NewHandler(loader, nil, false, "")

	req := httptest.NewRequest(http.MethodDelete, "/items/gone", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	// Runtime copy is gone.
	if _, err := os.Stat(runtimeFile); !os.IsNotExist(err) {
		t.Errorf("runtime file should be deleted, err=%v", err)
	}

	// Seed copy is still there.
	assert.FileExists(t, seedFile, "seed copy must survive DELETE")
}

// TestRuntimeDir_ReadFallback verifies that a GET reads from the runtime
// dir when StubRoot is configured. This is the flip side of write isolation:
// without it, reads would hit the seed dir and miss any mutations.
func TestRuntimeDir_ReadFallback(t *testing.T) {
	seed := t.TempDir()
	runtimeDir := filepath.Join(seed, ".apitwin", "state")

	require.NoError(t, os.MkdirAll(filepath.Join(seed, "stubs"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(runtimeDir, "stubs"), 0755))

	// Seed and runtime hold DIFFERENT content to prove reads come from runtime.
	require.NoError(t, os.WriteFile(filepath.Join(seed, "stubs", "user.json"), []byte(`{"who":"seed"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(runtimeDir, "stubs", "user.json"), []byte(`{"who":"runtime"}`), 0644))

	cfg := &config.Config{
		Routes: []config.Route{{
			Method:   "GET",
			Match:    "/user",
			Fallback: "ok",
			Cases: map[string]config.Case{
				"ok": {
					Status: 200,
					File:   "stubs/user.json",
				},
			},
		}},
	}

	loader := &stubRuntimeLoader{
		cfg:       cfg,
		configDir: seed,
		stubRoot:  runtimeDir,
	}
	handler := NewHandler(loader, nil, false, "")

	req := httptest.NewRequest(http.MethodGet, "/user", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"runtime"`)
	assert.NotContains(t, w.Body.String(), `"seed"`)
}
