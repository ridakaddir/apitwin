package proxy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
	runtimepersist "github.com/ridakaddir/apitwin/internal/runtime/persist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests the persistence wiring on the REST scheduler: Schedule writes an
// item, fire prunes it, Rearm past-due applies immediately, Rearm future
// schedules with remaining delay, Reset clears, Stop does not.

func TestSchedulerPersist_ScheduleWritesItemAndFirePrunesIt(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := dir
	metaPath := filepath.Join(dir, "scheduled.json")
	p := runtimepersist.NewScheduledPersister(metaPath, runtimeDir)

	stubFile := filepath.Join(dir, "continent.json")
	writeJSONFile(t, stubFile, map[string]interface{}{"status": "Queued"})

	defaultsFile := filepath.Join(dir, "ready.json")
	require.NoError(t, os.WriteFile(defaultsFile, []byte(`{"status":"Ready"}`), 0644))

	route := &config.Route{
		Method:   "POST",
		Match:    "/continents",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "provisioning", Duration: 1},
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"ready":   {Persist: true, Merge: "update", Defaults: "ready.json"},
		},
	}

	sched := newTransitionScheduler(context.Background())
	sched.SetPersister(p)
	defer sched.Stop()

	sched.Schedule(route, stubFile, dir, nil)

	// Immediately after Schedule, persister should have one "rest" item.
	f, err := p.Load()
	require.NoError(t, err)
	require.Len(t, f.Items, 1)
	assert.Equal(t, "rest", f.Items[0].Scope)
	assert.Equal(t, stubFile, f.Items[0].FilePath)
	// Payload was resolved eagerly.
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(f.Items[0].ResolvedPayload, &payload))
	assert.Equal(t, "Ready", payload["status"])

	// Wait for the timer to fire.
	waitForStatus(t, stubFile, "status", "Ready")

	// After fire, the persisted list should be empty.
	require.Eventually(t, func() bool {
		f, err := p.Load()
		return err == nil && len(f.Items) == 0
	}, 2*time.Second, 25*time.Millisecond, "persisted item was not pruned after fire")
}

func TestSchedulerPersist_RearmPastDueAppliesImmediately(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "scheduled.json")
	p := runtimepersist.NewScheduledPersister(metaPath, dir)

	stubFile := filepath.Join(dir, "continent.json")
	writeJSONFile(t, stubFile, map[string]interface{}{"status": "Pending"})

	payload, _ := json.Marshal(map[string]interface{}{"status": "Ready"})
	require.NoError(t, p.Add(runtimepersist.ScheduledItem{
		ID:              "past-1",
		Scope:           "rest",
		RouteKey:        "POST /continents",
		FilePath:        stubFile,
		FireAt:          time.Now().Add(-30 * time.Second),
		ResolvedPayload: payload,
	}))

	sched := newTransitionScheduler(context.Background())
	sched.SetPersister(p)
	defer sched.Stop()

	f, _ := p.Load()
	sched.Rearm(f.ItemsForScope("rest"))

	// Past-due: should have been applied synchronously.
	data := readJSONFile(t, stubFile)
	assert.Equal(t, "Ready", data["status"], "past-due rearm should apply immediately")

	// And removed from the persisted list.
	f, _ = p.Load()
	assert.Empty(t, f.Items, "past-due items should be pruned after rearm")
}

func TestSchedulerPersist_RearmFutureSchedulesRemainingDelay(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "scheduled.json")
	p := runtimepersist.NewScheduledPersister(metaPath, dir)

	stubFile := filepath.Join(dir, "continent.json")
	writeJSONFile(t, stubFile, map[string]interface{}{"status": "Pending"})

	payload, _ := json.Marshal(map[string]interface{}{"status": "Ready"})
	fireAt := time.Now().Add(500 * time.Millisecond)
	require.NoError(t, p.Add(runtimepersist.ScheduledItem{
		ID:              "future-1",
		Scope:           "rest",
		RouteKey:        "POST /continents",
		FilePath:        stubFile,
		FireAt:          fireAt,
		ResolvedPayload: payload,
	}))

	sched := newTransitionScheduler(context.Background())
	sched.SetPersister(p)
	defer sched.Stop()

	f, _ := p.Load()
	sched.Rearm(f.ItemsForScope("rest"))

	// Shortly after, the file has not been updated yet.
	data := readJSONFile(t, stubFile)
	assert.Equal(t, "Pending", data["status"])

	// But within the remaining delay it should update.
	waitForStatus(t, stubFile, "status", "Ready")

	require.Eventually(t, func() bool {
		f, err := p.Load()
		return err == nil && len(f.Items) == 0
	}, 2*time.Second, 25*time.Millisecond, "item was not pruned after fire")
}

func TestSchedulerPersist_ResetClearsPersistedSchedule(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "scheduled.json")
	p := runtimepersist.NewScheduledPersister(metaPath, dir)

	stubFile := filepath.Join(dir, "continent.json")
	writeJSONFile(t, stubFile, map[string]interface{}{"status": "Pending"})

	defaultsFile := filepath.Join(dir, "ready.json")
	require.NoError(t, os.WriteFile(defaultsFile, []byte(`{"status":"Ready"}`), 0644))

	route := &config.Route{
		Method:   "POST",
		Match:    "/continents",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "provisioning", Duration: 60},
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"ready":   {Persist: true, Merge: "update", Defaults: "ready.json"},
		},
	}

	sched := newTransitionScheduler(context.Background())
	sched.SetPersister(p)

	sched.Schedule(route, stubFile, dir, nil)

	f, _ := p.Load()
	require.Len(t, f.Items, 1, "item should be persisted before reset")

	// Simulate hot-reload.
	sched.Reset(context.Background())

	f, _ = p.Load()
	assert.Empty(t, f.Items, "Reset should clear persisted schedule")

	sched.Stop()
}

func TestSchedulerPersist_StopDoesNotClearSchedule(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "scheduled.json")
	p := runtimepersist.NewScheduledPersister(metaPath, dir)

	stubFile := filepath.Join(dir, "continent.json")
	writeJSONFile(t, stubFile, map[string]interface{}{"status": "Pending"})

	defaultsFile := filepath.Join(dir, "ready.json")
	require.NoError(t, os.WriteFile(defaultsFile, []byte(`{"status":"Ready"}`), 0644))

	route := &config.Route{
		Method:   "POST",
		Match:    "/continents",
		Fallback: "created",
		Transitions: []config.Transition{
			{Case: "provisioning", Duration: 60},
			{Case: "ready"},
		},
		Cases: map[string]config.Case{
			"created": {Persist: true, Merge: "append"},
			"ready":   {Persist: true, Merge: "update", Defaults: "ready.json"},
		},
	}

	sched := newTransitionScheduler(context.Background())
	sched.SetPersister(p)

	sched.Schedule(route, stubFile, dir, nil)

	// Stop the scheduler as if the server is shutting down — the pending
	// item must remain on disk so the next boot can re-arm it.
	sched.Stop()

	f, _ := p.Load()
	assert.Len(t, f.Items, 1, "Stop should NOT clear the persisted schedule")
}
