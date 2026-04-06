package grpc

import (
	"sync"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
)

// grpcTransitionState tracks the first-request timestamp for each gRPC route
// that has transitions defined. Mirrors proxy.transitionState for GRPCRoute.
type grpcTransitionState struct {
	mu       sync.Mutex
	firstHit map[string]time.Time
}

func newGRPCTransitionState() *grpcTransitionState {
	return &grpcTransitionState{
		firstHit: make(map[string]time.Time),
	}
}

// Reset clears all recorded first-hit times (called on config hot-reload).
func (ts *grpcTransitionState) Reset() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.firstHit = make(map[string]time.Time)
}

// resolve returns the active case name for a gRPC route with transitions.
// Durations are relative: each entry's Duration specifies how long that state
// lasts. They are accumulated into absolute thresholds at resolution time.
func (ts *grpcTransitionState) resolve(route *config.GRPCRoute) string {
	if len(route.Transitions) == 0 {
		return ""
	}

	key := route.Match
	ts.mu.Lock()
	t0, seen := ts.firstHit[key]
	if !seen {
		t0 = time.Now()
		ts.firstHit[key] = t0
	}
	ts.mu.Unlock()

	elapsed := time.Since(t0)
	elapsedSecs := int64(elapsed.Seconds())

	// Default to terminal (last) entry.
	current := route.Transitions[len(route.Transitions)-1].Case

	var cumulative int64
	for i := 0; i < len(route.Transitions)-1; i++ {
		t := route.Transitions[i]
		if t.Duration > 0 {
			cumulative += int64(t.Duration)
			if elapsedSecs < cumulative {
				current = t.Case
				break
			}
		}
	}

	return current
}
