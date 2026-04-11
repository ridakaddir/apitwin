// Package transitions implements the time-based state machine that drives
// config.Route / config.GRPCRoute transitions. Both REST and gRPC handlers
// parameterize the same generic State over their own route type, sharing a
// single implementation of the resolve algorithm.
package transitions

import (
	"strings"
	"sync"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
)

// RouteLike is satisfied by any route type that carries a transition sequence.
// Both *config.Route and *config.GRPCRoute implement it via GetTransitions().
type RouteLike interface {
	GetTransitions() []config.Transition
}

// State tracks the first-request timestamp for each route that has transitions
// defined. The key format is caller-defined via keyFn (e.g. REST uses
// "<METHOD> <match>", gRPC uses just <match>).
//
// FirstHit is exported so white-box tests can prime timestamps directly.
type State[R RouteLike] struct {
	mu       sync.Mutex
	FirstHit map[string]time.Time
	keyFn    func(R) string
}

// NewState constructs an empty state machine. keyFn extracts the unique key
// for a given route — callers pass this to bridge the REST/gRPC difference
// in key shape.
func NewState[R RouteLike](keyFn func(R) string) *State[R] {
	return &State[R]{
		FirstHit: make(map[string]time.Time),
		keyFn:    keyFn,
	}
}

// Reset clears all recorded first-hit times, restarting every transition
// sequence. Called on config hot-reload.
func (s *State[R]) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FirstHit = make(map[string]time.Time)
}

// ResetMatch clears recorded first-hit times for any key that targets the
// given match pattern. Handles both key shapes:
//   - exact match (gRPC: key == pattern)
//   - "<METHOD> <match>" suffix (REST: key has a space-prefixed method)
//
// This is used when a DELETE removes a resource: any route with transitions
// on the same match pattern must restart its transition sequence so that a
// subsequent recreate uses the initial case, not the terminal case (which
// would try to update a file that no longer exists).
func (s *State[R]) ResetMatch(matchPattern string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	suffix := " " + matchPattern
	for key := range s.FirstHit {
		if key == matchPattern || strings.HasSuffix(key, suffix) {
			delete(s.FirstHit, key)
		}
	}
}

// Resolve returns the case name that should be served for a route with
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
// Entries with Duration == 0 in a non-terminal position are skipped, since
// they never become selectable under the elapsedSecs < cumulative rule.
func (s *State[R]) Resolve(route R) string {
	ts := route.GetTransitions()
	if len(ts) == 0 {
		return ""
	}

	key := s.keyFn(route)

	s.mu.Lock()
	t0, seen := s.FirstHit[key]
	if !seen {
		t0 = time.Now()
		s.FirstHit[key] = t0
	}
	s.mu.Unlock()

	elapsed := time.Since(t0)
	elapsedSecs := int64(elapsed.Seconds())

	// Default to the terminal (last) entry — used when all thresholds are crossed.
	current := ts[len(ts)-1].Case

	var cumulative int64
	for i := 0; i < len(ts)-1; i++ {
		t := ts[i]
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
