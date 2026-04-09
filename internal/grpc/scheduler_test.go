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
