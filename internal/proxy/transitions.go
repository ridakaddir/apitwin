package proxy

import (
	"strings"
	"sync"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
)

// transitionState tracks the first-request timestamp for each route that has
// transitions defined. The key is "<METHOD> <match>" e.g. "GET /orders/*".
type transitionState struct {
	mu       sync.Mutex
	firstHit map[string]time.Time
}

func newTransitionState() *transitionState {
	return &transitionState{
		firstHit: make(map[string]time.Time),
	}
}

// Reset clears all recorded first-hit times, restarting every transition
// sequence. Called on config hot-reload.
func (ts *transitionState) Reset() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.firstHit = make(map[string]time.Time)
}

// ResetMatch clears recorded first-hit times for all routes whose match
// pattern equals the given pattern, regardless of HTTP method.
//
// This is used when a DELETE operation removes a resource: any POST route
// with transitions on the same match pattern must restart its transition
// sequence so that subsequent POSTs use the initial "created" case instead
// of the terminal "ready/active" case (which would try to update a file
// that no longer exists).
func (ts *transitionState) ResetMatch(matchPattern string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for key := range ts.firstHit {
		// key format is "METHOD match" (e.g. "POST /api/v1/*/model/{id}/config")
		// Extract the match portion after the first space.
		if idx := strings.IndexByte(key, ' '); idx != -1 && key[idx+1:] == matchPattern {
			delete(ts.firstHit, key)
		}
	}
}

// routeKey returns the unique key for a route.
func routeKey(route *config.Route) string {
	return route.Method + " " + route.Match
}

// resolve returns the case name that should be served for a route with
// transitions, recording the first-hit time if this is the first request.
//
// Resolution logic:
//  1. Record time.Now() as t0 on the very first request.
//  2. elapsed = time.Since(t0)
//  3. Walk transitions in order, accumulating Duration values to compute
//     absolute thresholds. Return the case for the first entry whose
//     cumulative threshold is still in the future (elapsedSecs < cumulative).
//     Once all thresholds are crossed, serve the terminal (last) entry.
//
// Example with duration=[30, 60, terminal]:
//
//	elapsed < 30s        → transitions[0].Case  (shipped)
//	30s ≤ elapsed < 90s  → transitions[1].Case  (out_for_delivery)  — cumulative 30+60
//	elapsed ≥ 90s        → transitions[2].Case  (delivered — terminal)
func (ts *transitionState) resolve(route *config.Route) string {
	if len(route.Transitions) == 0 {
		return ""
	}

	key := routeKey(route)

	ts.mu.Lock()
	t0, seen := ts.firstHit[key]
	if !seen {
		t0 = time.Now()
		ts.firstHit[key] = t0
	}
	ts.mu.Unlock()

	elapsed := time.Since(t0)
	elapsedSecs := int64(elapsed.Seconds())

	// Default to the terminal (last) entry — used when all thresholds are crossed.
	current := route.Transitions[len(route.Transitions)-1].Case

	// Walk non-terminal entries: accumulate durations to compute absolute
	// thresholds. Return the first entry whose cumulative threshold is still
	// in the future. Entries with Duration == 0 in a non-terminal position
	// are skipped, since they never become selectable under the
	// elapsedSecs < cumulative rule (they are effectively no-op stages).
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
