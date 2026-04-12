package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMirror_CopiesStubTreeAndExcludesConfigFiles(t *testing.T) {
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

	runtimeDir, err := Mirror(seed)
	if err != nil {
		t.Fatalf("Mirror: %v", err)
	}

	expectedRuntime := filepath.Join(seed, ".apitwin", "state")
	if runtimeDir != expectedRuntime {
		t.Errorf("runtimeDir = %q, want %q", runtimeDir, expectedRuntime)
	}

	// Mirrored files should exist.
	assertExists(t, filepath.Join(runtimeDir, "stubs", "a.json"))
	assertExists(t, filepath.Join(runtimeDir, "fixtures", "nested", "b.json"))

	// Excluded files must NOT exist in the runtime dir.
	assertNotExists(t, filepath.Join(runtimeDir, "apitwin.toml"))
	assertNotExists(t, filepath.Join(runtimeDir, ".apitwin", "stale.txt"))
}

func TestMirror_WipesPreviousRuntimeState(t *testing.T) {
	seed := t.TempDir()
	writeSeed(t, seed, filepath.Join("stubs", "a.json"), `{"id":"a"}`)

	// Populate a stale runtime dir with a file that should disappear.
	runtimeDir := DefaultPath(seed)
	if err := os.MkdirAll(filepath.Join(runtimeDir, "stubs"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stale := filepath.Join(runtimeDir, "stubs", "old.json")
	if err := os.WriteFile(stale, []byte(`{"stale":true}`), 0644); err != nil {
		t.Fatalf("writing stale: %v", err)
	}

	if _, err := Mirror(seed); err != nil {
		t.Fatalf("Mirror: %v", err)
	}

	assertNotExists(t, stale)
	assertExists(t, filepath.Join(runtimeDir, "stubs", "a.json"))
}

func TestMirror_SeedStubsAreNotModified(t *testing.T) {
	seed := t.TempDir()
	seedStub := filepath.Join(seed, "stubs", "a.json")
	writeSeed(t, seed, filepath.Join("stubs", "a.json"), `{"id":"a"}`)

	originalInfo, err := os.Stat(seedStub)
	if err != nil {
		t.Fatalf("stat seed: %v", err)
	}

	runtimeDir, err := Mirror(seed)
	if err != nil {
		t.Fatalf("Mirror: %v", err)
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
