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

func TestGRPCScheduler_AppliesTransitions(t *testing.T) {
	dir := t.TempDir()

	// Write a defaults file for the transition.
	defaultsPath := filepath.Join(dir, "defaults-ready.json")
	os.WriteFile(defaultsPath, []byte(`{"status": "Ready"}`), 0644)

	// Create the initial stub file.
	stubFile := filepath.Join(dir, "city.json")
	os.WriteFile(stubFile, []byte(`{"name": "rabat", "status": "Queued"}`), 0644)

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
	os.WriteFile(defaultsPath, []byte(`{"status": "Ready"}`), 0644)

	stubFile := filepath.Join(dir, "city.json")
	os.WriteFile(stubFile, []byte(`{"name": "rabat", "status": "Queued"}`), 0644)

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
	data, _ := os.ReadFile(stubFile)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	if m["status"] != "Queued" {
		t.Fatalf("expected status=Queued after Stop, got %v", m["status"])
	}
}

func TestGRPCScheduler_ResetCancelsOldGeneration(t *testing.T) {
	dir := t.TempDir()

	defaultsPath := filepath.Join(dir, "defaults.json")
	os.WriteFile(defaultsPath, []byte(`{"status": "Ready"}`), 0644)

	stubFile := filepath.Join(dir, "city.json")
	os.WriteFile(stubFile, []byte(`{"name": "rabat", "status": "Queued"}`), 0644)

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

	data, _ := os.ReadFile(stubFile)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	if m["status"] != "Queued" {
		t.Fatalf("expected status=Queued after Reset, got %v", m["status"])
	}
}
