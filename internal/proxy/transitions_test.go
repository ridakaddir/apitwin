package proxy

import (
	"testing"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
)

// helper: build a route with the given transitions and pre-seed the
// transitionState so that firstHit is `elapsed` seconds in the past.
func resolveAt(transitions []config.Transition, elapsed int) string {
	route := &config.Route{
		Method:      "GET",
		Match:       "/test",
		Transitions: transitions,
	}

	ts := newTransitionState()
	ts.firstHit[routeKey(route)] = time.Now().Add(-time.Duration(elapsed) * time.Second)

	return ts.resolve(route)
}

func TestResolve_EmptyTransitions(t *testing.T) {
	route := &config.Route{
		Method: "GET",
		Match:  "/test",
	}
	ts := newTransitionState()
	got := ts.resolve(route)
	if got != "" {
		t.Errorf("resolve with no transitions = %q, want empty string", got)
	}
}

func TestResolve_SingleTerminalState(t *testing.T) {
	// A single transition with no duration is always terminal.
	transitions := []config.Transition{
		{Case: "done"},
	}

	tests := []struct {
		elapsed int
		want    string
	}{
		{0, "done"},
		{100, "done"},
	}

	for _, tt := range tests {
		got := resolveAt(transitions, tt.elapsed)
		if got != tt.want {
			t.Errorf("elapsed=%ds: got %q, want %q", tt.elapsed, got, tt.want)
		}
	}
}

func TestResolve_TwoStates(t *testing.T) {
	// "loading" lasts 10s, then "ready" is terminal.
	transitions := []config.Transition{
		{Case: "loading", Duration: 10},
		{Case: "ready"},
	}

	tests := []struct {
		elapsed int
		want    string
	}{
		{0, "loading"},
		{5, "loading"},
		{9, "loading"},
		{10, "ready"},
		{100, "ready"},
	}

	for _, tt := range tests {
		got := resolveAt(transitions, tt.elapsed)
		if got != tt.want {
			t.Errorf("elapsed=%ds: got %q, want %q", tt.elapsed, got, tt.want)
		}
	}
}

func TestResolve_ThreeStates(t *testing.T) {
	// "shipped" 30s → "out_for_delivery" 60s → "delivered" terminal
	// Cumulative thresholds: 30s, 90s
	transitions := []config.Transition{
		{Case: "shipped", Duration: 30},
		{Case: "out_for_delivery", Duration: 60},
		{Case: "delivered"},
	}

	tests := []struct {
		elapsed int
		want    string
	}{
		{0, "shipped"},
		{15, "shipped"},
		{29, "shipped"},
		{30, "out_for_delivery"},
		{60, "out_for_delivery"},
		{89, "out_for_delivery"},
		{90, "delivered"},
		{1000, "delivered"},
	}

	for _, tt := range tests {
		got := resolveAt(transitions, tt.elapsed)
		if got != tt.want {
			t.Errorf("elapsed=%ds: got %q, want %q", tt.elapsed, got, tt.want)
		}
	}
}

func TestResolve_FourStates(t *testing.T) {
	// "a" 5s → "b" 10s → "c" 15s → "d" terminal
	// Cumulative thresholds: 5s, 15s, 30s
	transitions := []config.Transition{
		{Case: "a", Duration: 5},
		{Case: "b", Duration: 10},
		{Case: "c", Duration: 15},
		{Case: "d"},
	}

	tests := []struct {
		elapsed int
		want    string
	}{
		{0, "a"},
		{4, "a"},
		{5, "b"},
		{14, "b"},
		{15, "c"},
		{29, "c"},
		{30, "d"},
		{999, "d"},
	}

	for _, tt := range tests {
		got := resolveAt(transitions, tt.elapsed)
		if got != tt.want {
			t.Errorf("elapsed=%ds: got %q, want %q", tt.elapsed, got, tt.want)
		}
	}
}

func TestResolve_ZeroDurationNonTerminalSkipped(t *testing.T) {
	// A non-terminal entry with Duration == 0 is skipped.
	// "skipped" (0s) → "active" (10s) → "done" terminal
	// Only "active" at cumulative 10s matters; "skipped" is never served.
	transitions := []config.Transition{
		{Case: "skipped", Duration: 0},
		{Case: "active", Duration: 10},
		{Case: "done"},
	}

	tests := []struct {
		elapsed int
		want    string
	}{
		{0, "active"},
		{9, "active"},
		{10, "done"},
	}

	for _, tt := range tests {
		got := resolveAt(transitions, tt.elapsed)
		if got != tt.want {
			t.Errorf("elapsed=%ds: got %q, want %q", tt.elapsed, got, tt.want)
		}
	}
}

func TestResolve_FirstHitRecordedOnFirstCall(t *testing.T) {
	route := &config.Route{
		Method: "GET",
		Match:  "/test",
		Transitions: []config.Transition{
			{Case: "first", Duration: 100},
			{Case: "second"},
		},
	}

	ts := newTransitionState()

	// First call should record t0 and return the first case.
	got := ts.resolve(route)
	if got != "first" {
		t.Errorf("first call: got %q, want %q", got, "first")
	}

	// Verify firstHit was recorded.
	key := routeKey(route)
	ts.mu.Lock()
	_, recorded := ts.firstHit[key]
	ts.mu.Unlock()
	if !recorded {
		t.Error("firstHit was not recorded after first resolve call")
	}
}

func TestReset_ClearsFirstHit(t *testing.T) {
	route := &config.Route{
		Method: "GET",
		Match:  "/test",
		Transitions: []config.Transition{
			{Case: "a", Duration: 10},
			{Case: "b"},
		},
	}

	ts := newTransitionState()
	ts.resolve(route)

	key := routeKey(route)

	ts.mu.Lock()
	_, exists := ts.firstHit[key]
	ts.mu.Unlock()
	if !exists {
		t.Fatal("expected firstHit entry before reset")
	}

	ts.Reset()

	ts.mu.Lock()
	_, exists = ts.firstHit[key]
	ts.mu.Unlock()
	if exists {
		t.Error("firstHit entry should be cleared after reset")
	}
}

func TestRouteKey(t *testing.T) {
	route := &config.Route{
		Method: "GET",
		Match:  "/orders/*",
	}
	want := "GET /orders/*"
	got := routeKey(route)
	if got != want {
		t.Errorf("routeKey = %q, want %q", got, want)
	}
}
