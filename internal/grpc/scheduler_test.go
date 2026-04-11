package grpc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
)

func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func readTestJSON(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshalling %s: %v", path, err)
	}
	return m
}

func TestGRPCScheduler_AppliesTransitions(t *testing.T) {
	dir := t.TempDir()

	// Write a defaults file for the transition.
	defaultsPath := filepath.Join(dir, "defaults-ready.json")
	writeTestFile(t, defaultsPath, []byte(`{"status": "Ready"}`))

	// Create the initial stub file.
	stubFile := filepath.Join(dir, "city.json")
	writeTestFile(t, stubFile, []byte(`{"name": "rabat", "status": "Queued"}`))

	route := &config.GRPCRoute{
		Match:    "/continent.v1.CityService/CreateCity",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "provisioning", Duration: 1},
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"ready":   {Persist: true, Merge: "update", Defaults: "defaults-ready.json"},
		},
	}

	sched := newGRPCTransitionScheduler(context.Background())
	defer sched.Stop()

	sched.Schedule(route, stubFile, dir)

	// Wait for the transition to fire (1s duration + buffer).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(stubFile)
		if err == nil {
			var m map[string]interface{}
			if json.Unmarshal(data, &m) == nil && m["status"] == "Ready" {
				return // success
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("transition did not apply within deadline")
}

func TestGRPCScheduler_StopCancelsPending(t *testing.T) {
	dir := t.TempDir()

	defaultsPath := filepath.Join(dir, "defaults.json")
	writeTestFile(t, defaultsPath, []byte(`{"status": "Ready"}`))

	stubFile := filepath.Join(dir, "city.json")
	writeTestFile(t, stubFile, []byte(`{"name": "rabat", "status": "Queued"}`))

	route := &config.GRPCRoute{
		Match:    "/continent.v1.CityService/CreateCity",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "provisioning", Duration: 10}, // long delay
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"ready":   {Persist: true, Merge: "update", Defaults: "defaults.json"},
		},
	}

	sched := newGRPCTransitionScheduler(context.Background())
	sched.Schedule(route, stubFile, dir)

	// Stop immediately — transition should be cancelled.
	sched.Stop()

	// Verify file was NOT updated.
	m := readTestJSON(t, stubFile)
	if m["status"] != "Queued" {
		t.Fatalf("expected status=Queued after Stop, got %v", m["status"])
	}
}

// TestGRPCScheduler_RescheduleSameFileCancelsStale reproduces a bug where a
// delete+recreate sequence left a stale goroutine from the original create
// that then stomped the recreated file with "ready" defaults before its own
// transition window had elapsed.
//
// Timeline:
//   t=0.0s   Schedule(stubFile, delay=2s)             -> G1 pending
//   t=0.5s   file deleted (simulated)
//   t=0.55s  stubFile re-created, Schedule called again -> G2 pending
//   t=2.0s   G1 fires on the RE-CREATED file (bug)    -> status=Ready too early
//
// After the fix, Schedule with the same filePath must cancel any in-flight
// goroutine for that path so only G2 fires (at t=2.55s).
func TestGRPCScheduler_RescheduleSameFileCancelsStale(t *testing.T) {
	dir := t.TempDir()

	defaultsPath := filepath.Join(dir, "defaults-ready.json")
	writeTestFile(t, defaultsPath, []byte(`{"status": "Ready"}`))

	stubFile := filepath.Join(dir, "fr.json")
	writeTestFile(t, stubFile, []byte(`{"code": "FR", "status": "Queued"}`))

	route := &config.GRPCRoute{
		Match:    "/geography.GeographyService/CreateCountry",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "provisioning", Duration: 1}, // short for fast test
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"ready":   {Persist: true, Merge: "update", Defaults: "defaults-ready.json"},
		},
	}

	sched := newGRPCTransitionScheduler(context.Background())
	defer sched.Stop()

	// t=0: first Schedule — G1 will try to fire at t=1s.
	sched.Schedule(route, stubFile, dir)

	// t=0.3s: delete and recreate the file (simulating a delete+recreate
	// that happens entirely BEFORE G1's delay elapses).
	time.Sleep(300 * time.Millisecond)
	if err := os.Remove(stubFile); err != nil {
		t.Fatalf("remove stubFile: %v", err)
	}
	writeTestFile(t, stubFile, []byte(`{"code": "FR", "status": "Queued"}`))

	// t=0.35s: second Schedule for the SAME file — G2 will try to fire at t=1.35s.
	// The fix requires this call to cancel G1.
	sched.Schedule(route, stubFile, dir)

	// t=1.1s: G1 (at t=1s) should have been cancelled. The file should still
	// be "Queued" because G2 hasn't fired yet (it fires at t=1.35s).
	time.Sleep(750 * time.Millisecond) // total elapsed ~1.05s

	m := readTestJSON(t, stubFile)
	if m["status"] == "Ready" {
		t.Fatalf("stale goroutine stomped recreated file too early: status=Ready at t~1.05s; expected status=Queued (G1 should have been cancelled by second Schedule)")
	}
	if m["status"] != "Queued" {
		t.Fatalf("unexpected status at t~1.05s: %v (want Queued)", m["status"])
	}

	// t=1.6s: G2 has fired (scheduled at t=0.35s + 1s delay = t=1.35s).
	time.Sleep(550 * time.Millisecond) // total elapsed ~1.6s
	m = readTestJSON(t, stubFile)
	if m["status"] != "Ready" {
		t.Fatalf("G2 did not fire: status=%v at t~1.6s (want Ready)", m["status"])
	}
}

// TestGRPCScheduler_CancelFile verifies that CancelFile cancels an in-flight
// goroutine targeting the given file path without touching others.
func TestGRPCScheduler_CancelFile(t *testing.T) {
	dir := t.TempDir()

	defaultsPath := filepath.Join(dir, "defaults.json")
	writeTestFile(t, defaultsPath, []byte(`{"status": "Ready"}`))

	stubA := filepath.Join(dir, "a.json")
	stubB := filepath.Join(dir, "b.json")
	writeTestFile(t, stubA, []byte(`{"name": "a", "status": "Queued"}`))
	writeTestFile(t, stubB, []byte(`{"name": "b", "status": "Queued"}`))

	route := &config.GRPCRoute{
		Match:    "/geography.GeographyService/CreateCountry",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "provisioning", Duration: 1},
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"ready":   {Persist: true, Merge: "update", Defaults: "defaults.json"},
		},
	}

	sched := newGRPCTransitionScheduler(context.Background())
	defer sched.Stop()

	sched.Schedule(route, stubA, dir)
	sched.Schedule(route, stubB, dir)

	// Cancel A before its delay elapses.
	time.Sleep(200 * time.Millisecond)
	sched.CancelFile(stubA)

	// Wait past the delay.
	time.Sleep(1200 * time.Millisecond)

	mA := readTestJSON(t, stubA)
	if mA["status"] == "Ready" {
		t.Fatalf("CancelFile(stubA) failed: a.json was still updated (status=Ready)")
	}

	mB := readTestJSON(t, stubB)
	if mB["status"] != "Ready" {
		t.Fatalf("CancelFile(stubA) wrongly affected B: b.json status=%v (want Ready)", mB["status"])
	}
}

func TestGRPCScheduler_ResetCancelsOldGeneration(t *testing.T) {
	dir := t.TempDir()

	defaultsPath := filepath.Join(dir, "defaults.json")
	writeTestFile(t, defaultsPath, []byte(`{"status": "Ready"}`))

	stubFile := filepath.Join(dir, "city.json")
	writeTestFile(t, stubFile, []byte(`{"name": "rabat", "status": "Queued"}`))

	route := &config.GRPCRoute{
		Match:    "/continent.v1.CityService/CreateCity",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "provisioning", Duration: 10},
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"ready":   {Persist: true, Merge: "update", Defaults: "defaults.json"},
		},
	}

	sched := newGRPCTransitionScheduler(context.Background())
	defer sched.Stop()

	sched.Schedule(route, stubFile, dir)

	// Reset — old generation should be cancelled.
	sched.Reset(context.Background())

	time.Sleep(200 * time.Millisecond)

	m := readTestJSON(t, stubFile)
	if m["status"] != "Queued" {
		t.Fatalf("expected status=Queued after Reset, got %v", m["status"])
	}
}
