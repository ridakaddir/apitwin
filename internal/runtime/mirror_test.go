package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOverlaySeed_CopiesStubTreeAndExcludesConfigFiles(t *testing.T) {
	seed := t.TempDir()

	// Seed tree:
	//   apitwin.toml        (top-level config — MUST be excluded)
	//   stubs/a.json        (committed seed stub — MUST be copied)
	//   fixtures/nested/b.json (committed fixture — MUST be copied)
	//   .apitwin/stale.txt  (previous runtime state — MUST be excluded)
	writeSeed(t, seed, "apitwin.toml", `# config`)
	writeSeed(t, seed, filepath.Join("stubs", "a.json"), `{"id":"a"}`)
	writeSeed(t, seed, filepath.Join("fixtures", "nested", "b.json"), `{"id":"b"}`)
	writeSeed(t, seed, filepath.Join(".apitwin", "stale.txt"), `old`)

	runtimeDir, initialised, err := OverlaySeed(seed)
	if err != nil {
		t.Fatalf("OverlaySeed: %v", err)
	}
	if !initialised {
		t.Error("expected initialised=true on first call")
	}

	expectedRuntime := filepath.Join(seed, ".apitwin", "state")
	if runtimeDir != expectedRuntime {
		t.Errorf("runtimeDir = %q, want %q", runtimeDir, expectedRuntime)
	}

	// Mirrored files should exist.
	assertExists(t, filepath.Join(runtimeDir, "stubs", "a.json"))
	assertExists(t, filepath.Join(runtimeDir, "fixtures", "nested", "b.json"))

	// Sentinel should exist after first-run init.
	assertExists(t, filepath.Join(runtimeDir, sentinelFilename))

	// Excluded files must NOT exist in the runtime dir.
	assertNotExists(t, filepath.Join(runtimeDir, "apitwin.toml"))
	assertNotExists(t, filepath.Join(runtimeDir, ".apitwin", "stale.txt"))
}

func TestOverlaySeed_PreservesRuntimeOnlyFiles(t *testing.T) {
	seed := t.TempDir()
	writeSeed(t, seed, filepath.Join("stubs", "a.json"), `{"id":"a"}`)

	// First run: initialise the runtime dir.
	runtimeDir, initialised, err := OverlaySeed(seed)
	if err != nil {
		t.Fatalf("OverlaySeed #1: %v", err)
	}
	if !initialised {
		t.Error("first call should be initialising")
	}

	// Simulate a runtime POST that created a new stub which does not
	// exist in seed.
	runtimeOnly := filepath.Join(runtimeDir, "stubs", "runtime-created.json")
	if err := os.WriteFile(runtimeOnly, []byte(`{"id":"runtime"}`), 0644); err != nil {
		t.Fatalf("writing runtime-only stub: %v", err)
	}

	// Second run: runtime dir already initialised.
	_, initialised2, err := OverlaySeed(seed)
	if err != nil {
		t.Fatalf("OverlaySeed #2: %v", err)
	}
	if initialised2 {
		t.Error("second call should not be initialising")
	}

	assertExists(t, runtimeOnly)
	assertExists(t, filepath.Join(runtimeDir, "stubs", "a.json"))
}

func TestOverlaySeed_SeedWinsOnOverlap(t *testing.T) {
	seed := t.TempDir()
	seedStubRel := filepath.Join("stubs", "a.json")
	writeSeed(t, seed, seedStubRel, `{"id":"a","name":"original"}`)

	runtimeDir, _, err := OverlaySeed(seed)
	if err != nil {
		t.Fatalf("OverlaySeed #1: %v", err)
	}

	// Simulate a runtime mutation (PATCH) to the seed-derived file.
	runtimeCopy := filepath.Join(runtimeDir, "stubs", "a.json")
	if err := os.WriteFile(runtimeCopy, []byte(`{"id":"a","name":"mutated"}`), 0644); err != nil {
		t.Fatalf("mutate runtime: %v", err)
	}

	// Seed file is edited (simulates git pull / manual edit).
	if err := os.WriteFile(filepath.Join(seed, seedStubRel), []byte(`{"id":"a","name":"updated"}`), 0644); err != nil {
		t.Fatalf("edit seed: %v", err)
	}

	// Second run: seed should win over the runtime mutation.
	if _, _, err := OverlaySeed(seed); err != nil {
		t.Fatalf("OverlaySeed #2: %v", err)
	}

	got, err := os.ReadFile(runtimeCopy)
	if err != nil {
		t.Fatalf("read runtime copy: %v", err)
	}
	if string(got) != `{"id":"a","name":"updated"}` {
		t.Errorf("runtime copy = %q, want seed content", string(got))
	}
}

func TestMirror_WipesRuntimeIncludingRuntimeOnlyFiles(t *testing.T) {
	seed := t.TempDir()
	writeSeed(t, seed, filepath.Join("stubs", "a.json"), `{"id":"a"}`)

	runtimeDir, _, err := OverlaySeed(seed)
	if err != nil {
		t.Fatalf("OverlaySeed: %v", err)
	}

	// A runtime-only file that Mirror (reset) must remove.
	runtimeOnly := filepath.Join(runtimeDir, "stubs", "only.json")
	if err := os.WriteFile(runtimeOnly, []byte(`{"only":true}`), 0644); err != nil {
		t.Fatalf("write runtime-only: %v", err)
	}

	if _, err := Mirror(seed); err != nil {
		t.Fatalf("Mirror (reset): %v", err)
	}

	assertNotExists(t, runtimeOnly)
	assertExists(t, filepath.Join(runtimeDir, "stubs", "a.json"))
	assertExists(t, filepath.Join(runtimeDir, sentinelFilename))
}

func TestOverlaySeed_SeedStubsAreNotModified(t *testing.T) {
	seed := t.TempDir()
	seedStub := filepath.Join(seed, "stubs", "a.json")
	writeSeed(t, seed, filepath.Join("stubs", "a.json"), `{"id":"a"}`)

	originalInfo, err := os.Stat(seedStub)
	if err != nil {
		t.Fatalf("stat seed: %v", err)
	}

	runtimeDir, _, err := OverlaySeed(seed)
	if err != nil {
		t.Fatalf("OverlaySeed: %v", err)
	}

	// Mutate the runtime copy. The seed file must remain byte-identical.
	runtimeStub := filepath.Join(runtimeDir, "stubs", "a.json")
	if err := os.WriteFile(runtimeStub, []byte(`{"id":"mutated"}`), 0644); err != nil {
		t.Fatalf("mutate runtime: %v", err)
	}

	data, err := os.ReadFile(seedStub)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	if string(data) != `{"id":"a"}` {
		t.Errorf("seed stub was modified: %q", string(data))
	}

	// Size check for good measure.
	newInfo, err := os.Stat(seedStub)
	if err != nil {
		t.Fatalf("restat seed: %v", err)
	}
	if newInfo.Size() != originalInfo.Size() {
		t.Errorf("seed size changed: %d → %d", originalInfo.Size(), newInfo.Size())
	}
}

func TestEphemeral_CreatesTempDirAndCleanupRemovesIt(t *testing.T) {
	dir, err := Ephemeral()
	if err != nil {
		t.Fatalf("Ephemeral: %v", err)
	}
	assertExists(t, dir)

	if err := Cleanup(dir); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	assertNotExists(t, dir)
}

func TestIsRuntimePath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{filepath.Join("foo", ".apitwin", "state"), true},
		{filepath.Join("/abs", "project", ".apitwin", "state"), true},
		{filepath.Join("foo", ".apitwin", "state") + string(filepath.Separator), true},
		{filepath.Join("foo", "stubs"), false},
		{"/tmp", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsRuntimePath(tc.path); got != tc.want {
			t.Errorf("IsRuntimePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// writeSeed creates a file under dir with the given relative path and content,
// creating any necessary parent directories.
func writeSeed(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected %q to exist: %v", path, err)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected %q not to exist (err=%v)", path, err)
	}
}
