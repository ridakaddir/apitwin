package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadTransitions_MissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transitions.json")

	f, err := LoadTransitions(path)
	if err != nil {
		t.Fatalf("LoadTransitions: %v", err)
	}
	if f == nil {
		t.Fatal("expected non-nil file")
	}
	if len(f.Entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(f.Entries))
	}
}

func TestTransitions_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transitions.json")

	now := time.Now().UTC().Round(time.Millisecond)
	want := &TransitionsFile{
		Version: 1,
		Entries: map[string]TransitionsScope{
			"rest": {FirstHit: map[string]time.Time{"GET /continents": now}},
			"grpc": {FirstHit: map[string]time.Time{"continents.Continents/List": now.Add(-time.Second)}},
		},
	}
	if err := SaveTransitions(path, want); err != nil {
		t.Fatalf("SaveTransitions: %v", err)
	}

	got, err := LoadTransitions(path)
	if err != nil {
		t.Fatalf("LoadTransitions: %v", err)
	}
	if g := got.Scope("rest")["GET /continents"]; !g.Equal(now) {
		t.Errorf("rest scope round-trip mismatch: %v vs %v", g, now)
	}
	if g := got.Scope("grpc")["continents.Continents/List"]; !g.Equal(now.Add(-time.Second)) {
		t.Errorf("grpc scope round-trip mismatch: %v", g)
	}
}

func TestDeleteTransitions_MissingIsNoError(t *testing.T) {
	dir := t.TempDir()
	if err := DeleteTransitions(filepath.Join(dir, "missing.json")); err != nil {
		t.Errorf("DeleteTransitions of missing file should succeed: %v", err)
	}
}

func TestScheduledPersister_AddRemoveClear(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := t.TempDir()
	path := filepath.Join(dir, "scheduled.json")
	p := NewScheduledPersister(path, runtimeDir)

	payload, _ := json.Marshal(map[string]string{"status": "ready"})
	absFile := filepath.Join(runtimeDir, "stubs", "continents", "EU.json")
	item := ScheduledItem{
		ID:              "abc-123",
		Scope:           "rest",
		RouteKey:        "POST /continents",
		FilePath:        absFile,
		FireAt:          time.Now().Add(30 * time.Second).UTC().Round(time.Millisecond),
		ResolvedPayload: payload,
		Source:          "defaults/ready.json",
	}
	if err := p.Add(item); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Round-trip: file path should come back as absolute after Load.
	f, err := p.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(f.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(f.Items))
	}
	if f.Items[0].FilePath != absFile {
		t.Errorf("FilePath = %q, want %q", f.Items[0].FilePath, absFile)
	}

	// On-disk form should be relative.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading scheduled.json: %v", err)
	}
	var onDisk ScheduledFile
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if filepath.IsAbs(onDisk.Items[0].FilePath) {
		t.Errorf("on-disk FilePath should be relative, got %q", onDisk.Items[0].FilePath)
	}

	// Remove.
	if err := p.Remove("abc-123"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	f, _ = p.Load()
	if len(f.Items) != 0 {
		t.Errorf("expected empty after Remove, got %d items", len(f.Items))
	}

	// Clear on empty should be fine.
	if err := p.Clear(); err != nil {
		t.Errorf("Clear on empty: %v", err)
	}
}

func TestScheduledPersister_ItemsForScope(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := t.TempDir()
	p := NewScheduledPersister(filepath.Join(dir, "scheduled.json"), runtimeDir)

	mustAdd := func(id, scope string) {
		if err := p.Add(ScheduledItem{
			ID:       id,
			Scope:    scope,
			FilePath: filepath.Join(runtimeDir, "stubs", id+".json"),
			FireAt:   time.Now().Add(time.Minute),
		}); err != nil {
			t.Fatalf("Add %s: %v", id, err)
		}
	}
	mustAdd("a", "rest")
	mustAdd("b", "grpc")
	mustAdd("c", "rest")

	f, err := p.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(f.ItemsForScope("rest")); got != 2 {
		t.Errorf("rest items = %d, want 2", got)
	}
	if got := len(f.ItemsForScope("grpc")); got != 1 {
		t.Errorf("grpc items = %d, want 1", got)
	}

	if err := p.RemoveByScope("grpc"); err != nil {
		t.Fatalf("RemoveByScope: %v", err)
	}
	f, _ = p.Load()
	if got := len(f.ItemsForScope("grpc")); got != 0 {
		t.Errorf("grpc items after RemoveByScope = %d, want 0", got)
	}
	if got := len(f.ItemsForScope("rest")); got != 2 {
		t.Errorf("rest items should be untouched, got %d", got)
	}
}

func TestAtomicWrite_NoPartialFileOnError(t *testing.T) {
	// Writing to a path whose parent is a regular file should fail without
	// leaving a stray .tmp file in the target location.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	err := atomicWrite(filepath.Join(blocker, "inner.json"), []byte("hi"))
	if err == nil {
		t.Fatal("expected error writing under a regular file")
	}
	// There should be no half-written tmp file under dir.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "blocker" {
			t.Errorf("unexpected leftover: %s", e.Name())
		}
	}
}
