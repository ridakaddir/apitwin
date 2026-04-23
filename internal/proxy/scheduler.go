package proxy

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/logger"
	"github.com/ridakaddir/apitwin/internal/persist"
	runtimepersist "github.com/ridakaddir/apitwin/internal/runtime/persist"
	"github.com/ridakaddir/apitwin/internal/transitions"
)

// transitionScheduler manages background goroutines that apply deferred file
// mutations after a resource is created via persist append.
//
// When a POST route has transitions defined, each non-terminal transition case
// with persist=true and merge="update" schedules a goroutine that sleeps for
// the cumulative duration, then applies the case's defaults to the created file.
//
// Since server restarts must preserve pending mutations (runtime state
// persistence), defaults are resolved eagerly at schedule time and the
// resolved payload is persisted alongside the fireAt timestamp. The
// observable delta from fire-time resolution: cross-endpoint refs and
// dynamic tokens like {{now}}/{{uuid}} freeze at creation time. This is
// almost always what users want — the "Ready" payload reflects the state
// of the world when the resource was created.
//
// Concurrency: each call to Reset or Stop swaps in a new transitions.Generation
// so that in-flight Schedule/schedule calls from concurrent HTTP requests
// never call WG.Add on a WaitGroup that is being waited on.
type transitionScheduler struct {
	mu        sync.Mutex
	gen       *transitions.Generation
	persister *runtimepersist.ScheduledPersister // nil for legacy/ephemeral/tests
}

// idGen produces monotonically increasing ids for pending items. Using a
// timestamp-based id keeps the persisted schedule human-inspectable and
// makes collisions impossible without pulling in a uuid dependency.
var idGen uint64

func nextScheduledID() string {
	t := time.Now().UnixNano()
	idGen++
	return "sch-" + time.Unix(0, t).Format("20060102T150405.000000000") + "-" + itoa(idGen)
}

func itoa(u uint64) string {
	if u == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for u > 0 {
		i--
		buf[i] = byte('0' + u%10)
		u /= 10
	}
	return string(buf[i:])
}

// newTransitionScheduler creates a scheduler with a cancellable context.
func newTransitionScheduler(parent context.Context) *transitionScheduler {
	return &transitionScheduler{
		gen: transitions.NewGeneration(parent),
	}
}

// SetPersister wires a ScheduledPersister so new Schedule calls persist
// pending items and fired items are pruned. Passing nil detaches the
// persister (used by tests). Must be called before any Schedule call to
// avoid persisting partial state.
func (s *transitionScheduler) SetPersister(p *runtimepersist.ScheduledPersister) {
	s.mu.Lock()
	s.persister = p
	s.mu.Unlock()
}

// Schedule inspects a route's transitions and spawns background goroutines
// for each transition case that needs a deferred file mutation.
//
// filePath is the absolute path to the file created by the append operation.
// configDir is needed to resolve relative defaults file paths.
// refCtx is the request context needed to resolve dynamic placeholders in defaults files.
//
// Only transition cases (beyond the initial/fallback case) that have
// persist=true, merge="update", and a non-empty defaults path are scheduled.
// Entries with Duration <= 0 in a non-terminal position are skipped (same
// semantics as request-time resolve).
func (s *transitionScheduler) Schedule(route *config.Route, filePath, configDir string, refCtx *RefContext) {
	if len(route.Transitions) < 2 {
		return // need at least an initial + one transition case
	}

	key := routeKey(route)

	// Accumulate durations of preceding entries to compute absolute delays
	// from now (creation time). The delay for entry N is the sum of durations
	// of all entries before it (entries 0..N-1).
	var cumulative int64
	for i := 0; i < len(route.Transitions); i++ {
		// Skip the first entry (it's the initial state, already applied by
		// the append operation).
		if i == 0 {
			if route.Transitions[i].Duration > 0 {
				cumulative += int64(route.Transitions[i].Duration)
			}
			continue
		}

		t := route.Transitions[i]
		c, ok := route.Cases[t.Case]
		if !ok {
			if t.Duration > 0 {
				cumulative += int64(t.Duration)
			}
			continue
		}

		// Only schedule if the case has persist + merge="update" + defaults.
		if c.Persist && c.Defaults != "" {
			if !strings.EqualFold(c.Merge, "update") {
				logger.Warn("deferred transition: case has persist+defaults but merge is not \"update\"; skipping",
					"case", t.Case, "merge", c.Merge)
			} else if cumulative > 0 {
				delay := time.Duration(cumulative) * time.Second
				// Resolve defaults eagerly so the persisted payload is
				// independent of the (non-serialisable) request context.
				incoming, err := loadDefaultsStatic(c.Defaults, configDir, make(map[string]bool), refCtx)
				if err != nil {
					logger.Error("deferred transition: eager defaults resolution failed",
						"case", t.Case, "defaults", c.Defaults, "err", err)
					if t.Duration > 0 {
						cumulative += int64(t.Duration)
					}
					continue
				}
				if len(incoming) == 0 {
					logger.Warn("deferred transition: eager defaults resolved to empty, skipping",
						"case", t.Case, "defaults", c.Defaults)
					if t.Duration > 0 {
						cumulative += int64(t.Duration)
					}
					continue
				}
				id := nextScheduledID()
				fireAt := time.Now().Add(delay)
				s.persistItem(id, key, filePath, fireAt, incoming, c.Defaults)
				s.scheduleResolved(id, delay, filePath, incoming, c.Defaults)
			}
		}

		// Accumulate this entry's duration for subsequent entries.
		// Skip Duration <= 0 to align with request-time resolve semantics.
		if t.Duration > 0 {
			cumulative += int64(t.Duration)
		}
	}
}

// persistItem records a new pending item in the scheduled file. Best-effort:
// errors are logged but do not block scheduling — the timer goroutine runs
// regardless, so the mutation still fires in-process.
func (s *transitionScheduler) persistItem(id, routeKey, filePath string, fireAt time.Time, payload map[string]interface{}, source string) {
	s.mu.Lock()
	p := s.persister
	s.mu.Unlock()
	if p == nil {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		logger.Warn("deferred transition: marshal payload for persistence failed", "err", err)
		return
	}
	if err := p.Add(runtimepersist.ScheduledItem{
		ID:              id,
		Scope:           "rest",
		RouteKey:        routeKey,
		FilePath:        filePath,
		FireAt:          fireAt,
		ResolvedPayload: raw,
		Source:          source,
	}); err != nil {
		logger.Warn("deferred transition: persist Add failed", "err", err)
	}
}

// scheduleResolved spawns a goroutine that waits delay then applies the
// eager-resolved payload via persist.Update. Used both by Schedule (fresh
// armings) and Rearm (restart recovery with remaining delay).
func (s *transitionScheduler) scheduleResolved(id string, delay time.Duration, filePath string, payload map[string]interface{}, source string) {
	s.mu.Lock()
	gen := s.gen
	if gen.Ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	gen.WG.Add(1)
	p := s.persister
	s.mu.Unlock()

	go func() {
		defer gen.WG.Done()

		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-timer.C:
			if _, err := persist.Update(filePath, payload); err != nil {
				if persist.IsNotFound(err) {
					logger.Warn("deferred transition: file was deleted before transition",
						"file", filePath)
				} else {
					logger.Error("deferred transition: update failed",
						"file", filePath, "err", err)
				}
				// Still remove from persister — the item fired (or was
				// doomed): keeping it would cause a redundant rearm on
				// next start.
				if p != nil {
					_ = p.Remove(id)
				}
				return
			}
			if p != nil {
				_ = p.Remove(id)
			}
			logger.Info("deferred transition applied",
				"file", filePath, "defaults", source, "delay", delay.String())

		case <-gen.Ctx.Done():
			// Cancelled (hot-reload or shutdown). Shutdown leaves the
			// item on disk for next-boot rearm; hot-reload clears via
			// persister.Clear in Reset.
			return
		}
	}()
}

// Rearm re-arms pending items loaded from the persister on startup. Items
// whose fireAt is already in the past are applied synchronously; future
// items are scheduled with the remaining delay using their original id so
// the on-fire Remove() matches.
func (s *transitionScheduler) Rearm(items []runtimepersist.ScheduledItem) {
	now := time.Now()
	s.mu.Lock()
	p := s.persister
	s.mu.Unlock()

	for _, it := range items {
		if it.Scope != "rest" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(it.ResolvedPayload, &payload); err != nil {
			logger.Warn("rearm: invalid resolvedPayload, dropping",
				"id", it.ID, "err", err)
			if p != nil {
				_ = p.Remove(it.ID)
			}
			continue
		}
		if !it.FireAt.After(now) {
			// Past-due: apply immediately and prune.
			if _, err := persist.Update(it.FilePath, payload); err != nil {
				if persist.IsNotFound(err) {
					logger.Warn("rearm: past-due target missing",
						"file", it.FilePath)
				} else {
					logger.Error("rearm: past-due update failed",
						"file", it.FilePath, "err", err)
				}
			} else {
				logger.Info("rearm: past-due transition applied",
					"file", it.FilePath, "source", it.Source)
			}
			if p != nil {
				_ = p.Remove(it.ID)
			}
			continue
		}
		remaining := time.Until(it.FireAt)
		s.scheduleResolved(it.ID, remaining, it.FilePath, payload, it.Source)
	}
}

// Reset cancels all pending mutations and creates a fresh generation.
// Called on config hot-reload to restart transition schedules.
//
// The old generation's context is cancelled and its WaitGroup is drained.
// New Schedule/schedule calls that arrive concurrently will attach to the
// new generation, avoiding the sync.WaitGroup Add/Wait race.
//
// Hot-reload wipes the persisted schedule — the new config may carry
// different transitions, and honouring a stale pending item against a
// route that no longer exists (or whose shape has changed) is worse than
// dropping it.
func (s *transitionScheduler) Reset(parent context.Context) {
	newGen := transitions.NewGeneration(parent)

	s.mu.Lock()
	oldGen := s.gen
	s.gen = newGen
	p := s.persister
	s.mu.Unlock()

	// Cancel + drain the old generation outside the lock. New requests that
	// call Schedule concurrently will use newGen, so there is no Add/Wait
	// race on oldGen's WaitGroup.
	oldGen.Stop()

	if p != nil {
		// Only clear our scope so gRPC's pending items are untouched.
		if err := p.RemoveByScope("rest"); err != nil {
			logger.Warn("scheduler reset: clearing persisted schedule failed", "err", err)
		}
	}
}

// Stop cancels all pending mutations and waits for goroutines to finish.
// Called on server shutdown.
//
// Intentionally does NOT clear the persister — pending items must survive
// shutdown so the next start can re-arm them.
func (s *transitionScheduler) Stop() {
	s.mu.Lock()
	gen := s.gen
	s.mu.Unlock()
	gen.Stop()
}
