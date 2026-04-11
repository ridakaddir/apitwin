package proxy

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/logger"
	"github.com/ridakaddir/apitwin/internal/persist"
	"github.com/ridakaddir/apitwin/internal/transitions"
)

// transitionScheduler manages background goroutines that apply deferred file
// mutations after a resource is created via persist append.
//
// When a POST route has transitions defined, each non-terminal transition case
// with persist=true and merge="update" schedules a goroutine that sleeps for
// the cumulative duration, then applies the case's defaults to the created file.
//
// This allows resources to "transition" on disk (e.g. Deploying → Ready)
// without requiring a subsequent GET request to trigger the state change.
//
// Concurrency: each call to Reset or Stop swaps in a new transitions.Generation
// so that in-flight Schedule/schedule calls from concurrent HTTP requests
// never call WG.Add on a WaitGroup that is being waited on.
type transitionScheduler struct {
	mu  sync.Mutex
	gen *transitions.Generation
}

// newTransitionScheduler creates a scheduler with a cancellable context.
func newTransitionScheduler(parent context.Context) *transitionScheduler {
	return &transitionScheduler{
		gen: transitions.NewGeneration(parent),
	}
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
				s.schedule(delay, filePath, c, configDir, refCtx)
			}
		}

		// Accumulate this entry's duration for subsequent entries.
		// Skip Duration <= 0 to align with request-time resolve semantics.
		if t.Duration > 0 {
			cumulative += int64(t.Duration)
		}
	}
}

// schedule spawns a single goroutine that waits for delay, then applies
// the case's defaults to the file via persist.Update.
//
// Uses time.NewTimer instead of time.After so the timer can be stopped
// and garbage-collected promptly when the context is cancelled.
func (s *transitionScheduler) schedule(delay time.Duration, filePath string, c config.Case, configDir string, refCtx *RefContext) {
	s.mu.Lock()
	gen := s.gen
	if gen.Ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	gen.WG.Add(1)
	s.mu.Unlock()

	go func() {
		defer gen.WG.Done()

		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-timer.C:
			// Load defaults and apply the mutation.
			incoming, err := loadDefaultsStatic(c.Defaults, configDir, make(map[string]bool), refCtx)
			if err != nil {
				logger.Error("deferred transition: loading defaults failed", "err", err)
				return
			}
			if len(incoming) == 0 {
				logger.Warn("deferred transition: no defaults to apply",
					"file", filePath, "defaults", c.Defaults)
				return
			}

			if _, err := persist.Update(filePath, incoming); err != nil {
				if persist.IsNotFound(err) {
					logger.Warn("deferred transition: file was deleted before transition",
						"file", filePath)
				} else {
					logger.Error("deferred transition: update failed",
						"file", filePath, "err", err)
				}
				return
			}

			logger.Info("deferred transition applied",
				"file", filePath, "defaults", c.Defaults, "delay", delay.String())

		case <-gen.Ctx.Done():
			// Cancelled (hot-reload or shutdown).
			return
		}
	}()
}

// Reset cancels all pending mutations and creates a fresh generation.
// Called on config hot-reload to restart transition schedules.
//
// The old generation's context is cancelled and its WaitGroup is drained.
// New Schedule/schedule calls that arrive concurrently will attach to the
// new generation, avoiding the sync.WaitGroup Add/Wait race.
func (s *transitionScheduler) Reset(parent context.Context) {
	newGen := transitions.NewGeneration(parent)

	s.mu.Lock()
	oldGen := s.gen
	s.gen = newGen
	s.mu.Unlock()

	// Cancel + drain the old generation outside the lock. New requests that
	// call Schedule concurrently will use newGen, so there is no Add/Wait
	// race on oldGen's WaitGroup.
	oldGen.Stop()
}

// Stop cancels all pending mutations and waits for goroutines to finish.
// Called on server shutdown.
func (s *transitionScheduler) Stop() {
	s.mu.Lock()
	gen := s.gen
	s.mu.Unlock()
	gen.Stop()
}
