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
// Timeline (transition duration = 2s so each margin is ≥500ms for CI stability):
//   t=0.0s   Schedule(stubFile, delay=2s)             -> G1 fires at t=2.0s
//   t=0.5s   delete+recreate stubFile, Schedule again -> G2 fires at t=2.5s
//   t=2.25s  assert status still "Queued" (G1 must be cancelled, G2 not yet fired)
//   t=3.0s   assert status "Ready" (G2 has fired)
//
// After the fix, Schedule with the same filePath must cancel any in-flight
// goroutine for that path so only G2 fires.
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
			{Case: "provisioning", Duration: 2},
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"ready":   {Persist: true, Merge: "update", Defaults: "defaults-ready.json"},
		},
	}

	sched := newGRPCTransitionScheduler(context.Background())
	defer sched.Stop()

	start := time.Now()

	// t=0: first Schedule — G1 will try to fire at t=2.0s.
	sched.Schedule(route, stubFile, dir)

	// t=0.5s: delete and recreate the file (simulating a delete+recreate
	// that happens entirely BEFORE G1's delay elapses).
	sleepUntil(start, 500*time.Millisecond)
	if err := os.Remove(stubFile); err != nil {
		t.Fatalf("remove stubFile: %v", err)
	}
	writeTestFile(t, stubFile, []byte(`{"code": "FR", "status": "Queued"}`))

	// Second Schedule for the SAME file — G2 will try to fire at t~2.5s.
	// The fix requires this call to cancel G1.
	sched.Schedule(route, stubFile, dir)

	// t=2.25s: G1 (at t=2.0s) should have been cancelled. The file should
	// still be "Queued" because G2 hasn't fired yet (it fires at t~2.5s).
	// Margin: 250ms either side of the G1 firing window and 250ms before G2.
	sleepUntil(start, 2250*time.Millisecond)

	m := readTestJSON(t, stubFile)
	if m["status"] == "Ready" {
		t.Fatalf("stale goroutine stomped recreated file too early: status=Ready at t~2.25s; expected status=Queued (G1 should have been cancelled by second Schedule)")
	}
	if m["status"] != "Queued" {
		t.Fatalf("unexpected status at t~2.25s: %v (want Queued)", m["status"])
	}

	// t=3.0s: G2 has fired (scheduled at t=0.5s + 2s delay = t=2.5s). 500ms margin.
	sleepUntil(start, 3000*time.Millisecond)
	m = readTestJSON(t, stubFile)
	if m["status"] != "Ready" {
		t.Fatalf("G2 did not fire: status=%v at t~3.0s (want Ready)", m["status"])
	}
}

// sleepUntil sleeps the remainder of target since start. A no-op if already past.
func sleepUntil(start time.Time, target time.Duration) {
	remain := target - time.Since(start)
	if remain > 0 {
		time.Sleep(remain)
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

// TestGRPCScheduler_ScheduleIfIdle verifies that ScheduleIfIdle arms the
// timeline only when no mutation is already pending for the file. This is
// the helper the handler uses on the merge="update" path so a client write
// against an existing entity stub does not reset an in-flight ready timer
// (preserves scenarios 13/15).
func TestGRPCScheduler_ScheduleIfIdle(t *testing.T) {
	dir := t.TempDir()

	defaultsPath := filepath.Join(dir, "defaults-ready.json")
	writeTestFile(t, defaultsPath, []byte(`{"status": "Ready"}`))

	stubFile := filepath.Join(dir, "city.json")
	writeTestFile(t, stubFile, []byte(`{"name": "rabat", "status": "Queued"}`))

	route := &config.GRPCRoute{
		Match:    "/continent.v1.CityService/CreateCity",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "provisioning", Duration: 2}, // long enough for the assertions below
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"ready":   {Persist: true, Merge: "update", Defaults: "defaults-ready.json"},
		},
	}

	sched := newGRPCTransitionScheduler(context.Background())
	defer sched.Stop()

	start := time.Now()

	// First call: file is idle → arms the timeline.
	sched.ScheduleIfIdle(route, stubFile, dir)
	if !sched.HasPending(stubFile) {
		t.Fatal("expected HasPending=true after first ScheduleIfIdle")
	}

	// Second call mid-flight: file already has pending → must be a no-op.
	// To prove it's a no-op, we record the goroutine count by checking
	// HasPending and then verifying the timer fires at its ORIGINAL t=2s
	// (not reset to t=now+2s).
	sleepUntil(start, 500*time.Millisecond)
	sched.ScheduleIfIdle(route, stubFile, dir)
	if !sched.HasPending(stubFile) {
		t.Fatal("expected HasPending=true after second ScheduleIfIdle")
	}

	// Wait until t~2.3s — original timer fires at t=2s, reset would fire
	// at t=2.5s. If ScheduleIfIdle had reset the timer, status would still
	// be "Queued" here.
	sleepUntil(start, 2300*time.Millisecond)

	m := readTestJSON(t, stubFile)
	if m["status"] != "Ready" {
		t.Fatalf("expected original timer to fire at t=2s (status=Ready at t~2.3s), got %v — ScheduleIfIdle must have reset the timer", m["status"])
	}
}

// TestGRPCScheduler_HasPending verifies the HasPending helper used by the
// handler to skip rescheduling on merge="update" when a transition timeline
// is already armed (preserves scenario 13/15 semantics: a client update
// during the provisioning window must NOT cancel an in-flight ready timer).
func TestGRPCScheduler_HasPending(t *testing.T) {
	dir := t.TempDir()

	defaultsPath := filepath.Join(dir, "defaults.json")
	writeTestFile(t, defaultsPath, []byte(`{"status": "Ready"}`))

	stubFile := filepath.Join(dir, "city.json")
	writeTestFile(t, stubFile, []byte(`{"name": "rabat", "status": "Queued"}`))

	route := &config.GRPCRoute{
		Match:    "/continent.v1.CityService/CreateCity",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "provisioning", Duration: 5}, // long enough to inspect mid-flight
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"ready":   {Persist: true, Merge: "update", Defaults: "defaults.json"},
		},
	}

	sched := newGRPCTransitionScheduler(context.Background())
	defer sched.Stop()

	// Nothing scheduled yet.
	if sched.HasPending(stubFile) {
		t.Error("HasPending should be false before Schedule")
	}

	sched.Schedule(route, stubFile, dir)

	if !sched.HasPending(stubFile) {
		t.Error("HasPending should be true after Schedule")
	}

	// Different file path — must be independent.
	otherFile := filepath.Join(dir, "other.json")
	if sched.HasPending(otherFile) {
		t.Error("HasPending should be false for unrelated file")
	}

	// CancelFile drains the entry.
	sched.CancelFile(stubFile)
	if sched.HasPending(stubFile) {
		t.Error("HasPending should be false after CancelFile")
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
