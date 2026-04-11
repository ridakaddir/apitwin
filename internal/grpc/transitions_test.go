package grpc

import (
	"testing"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
)

// helper: build a gRPC route with the given transitions and pre-seed the
// grpcTransitionState so that firstHit is `elapsed` seconds in the past.
func grpcResolveAt(transitions []config.Transition, elapsed int) string {
	route := &config.GRPCRoute{
		Match:       "/test.Service/Method",
		Transitions: transitions,
	}

	ts := newGRPCTransitionState()
	ts.FirstHit[route.Match] = time.Now().Add(-time.Duration(elapsed) * time.Second)

	return ts.Resolve(route)
}

func TestGRPCResolve_EmptyTransitions(t *testing.T) {
	route := &config.GRPCRoute{
		Match: "/test.Service/Method",
	}
	ts := newGRPCTransitionState()
	got := ts.Resolve(route)
	if got != "" {
		t.Errorf("resolve with no transitions = %q, want empty string", got)
	}
}

func TestGRPCResolve_SingleTerminalState(t *testing.T) {
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
		got := grpcResolveAt(transitions, tt.elapsed)
		if got != tt.want {
			t.Errorf("elapsed=%ds: got %q, want %q", tt.elapsed, got, tt.want)
		}
	}
}

func TestGRPCResolve_TwoStates(t *testing.T) {
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
		got := grpcResolveAt(transitions, tt.elapsed)
		if got != tt.want {
			t.Errorf("elapsed=%ds: got %q, want %q", tt.elapsed, got, tt.want)
		}
	}
}

func TestGRPCResolve_ThreeStates(t *testing.T) {
	// "processing" 10s → "shipped" 50s → "delivered" terminal
	// Cumulative thresholds: 10s, 60s
	transitions := []config.Transition{
		{Case: "processing", Duration: 10},
		{Case: "shipped", Duration: 50},
		{Case: "delivered"},
	}

	tests := []struct {
		elapsed int
		want    string
	}{
		{0, "processing"},
		{9, "processing"},
		{10, "shipped"},
		{59, "shipped"},
		{60, "delivered"},
		{1000, "delivered"},
	}

	for _, tt := range tests {
		got := grpcResolveAt(transitions, tt.elapsed)
		if got != tt.want {
			t.Errorf("elapsed=%ds: got %q, want %q", tt.elapsed, got, tt.want)
		}
	}
}

func TestGRPCResolve_ZeroDurationNonTerminalSkipped(t *testing.T) {
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
		got := grpcResolveAt(transitions, tt.elapsed)
		if got != tt.want {
			t.Errorf("elapsed=%ds: got %q, want %q", tt.elapsed, got, tt.want)
		}
	}
}

func TestGRPCReset_ClearsFirstHit(t *testing.T) {
	route := &config.GRPCRoute{
		Match: "/test.Service/Method",
		Transitions: []config.Transition{
			{Case: "a", Duration: 10},
			{Case: "b"},
		},
	}

	ts := newGRPCTransitionState()
	ts.Resolve(route)

	_, exists := ts.FirstHit[route.Match]
	if !exists {
		t.Fatal("expected firstHit entry before reset")
	}

	ts.Reset()

	_, exists = ts.FirstHit[route.Match]
	if exists {
		t.Error("firstHit entry should be cleared after reset")
	}
}
