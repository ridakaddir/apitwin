package grpc

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/logger"
	"github.com/ridakaddir/apitwin/internal/persist"
)

// grpcTransitionScheduler manages background goroutines that apply deferred
// file mutations after a resource is created via persist append on a gRPC route.
//
// When a gRPC route has transitions defined, each transition case with
// persist=true, merge="update", and a non-empty defaults path schedules a
// goroutine that sleeps for the cumulative duration, then applies the case's
// defaults to the created file.
//
// In-flight goroutines are tracked by target file path so that a subsequent
// Schedule() for the same file — or an explicit CancelFile() — cancels any
// stale pending mutations. Without this, a delete+recreate sequence leaves a
// goroutine from the original create which then stomps the recreated file
// with the wrong defaults before its own transition window elapses.
type grpcTransitionScheduler struct {
	mu      sync.Mutex
	gen     *grpcSchedulerGeneration
	pending map[string][]*pendingMutation // keyed on target filePath
}

type grpcSchedulerGeneration struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// pendingMutation is a single in-flight scheduled update for a given filePath.
// Each entry owns its own context derived from the generation context so it
// can be cancelled individually without affecting other files.
type pendingMutation struct {
	cancel context.CancelFunc
}

func newGRPCTransitionScheduler(parent context.Context) *grpcTransitionScheduler {
	ctx, cancel := context.WithCancel(parent)
	return &grpcTransitionScheduler{
		gen:     &grpcSchedulerGeneration{ctx: ctx, cancel: cancel},
		pending: make(map[string][]*pendingMutation),
	}
}

// Schedule inspects a gRPC route's transitions and spawns background goroutines
// for each transition case that needs a deferred file mutation.
//
// filePath is the absolute path to the file created by the append operation.
// configDir is needed to resolve relative defaults file paths.
//
// Any in-flight goroutines targeting the same filePath are cancelled before
// the new ones are spawned. This prevents stale mutations from a previous
// create (e.g. after a delete+recreate of the same resource) from stomping
// the newly-created file with the wrong defaults.
func (s *grpcTransitionScheduler) Schedule(route *config.GRPCRoute, filePath, configDir string) {
	if len(route.Transitions) < 2 {
		return
	}

	// Cancel any stale in-flight goroutines for this file path so a
	// recreated resource starts with a clean timeline.
	s.CancelFile(filePath)

	var cumulative int64
	for i := 0; i < len(route.Transitions); i++ {
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

		if c.Persist && c.Defaults != "" {
			if !strings.EqualFold(c.Merge, "update") {
				logger.Warn("grpc deferred transition: case has persist+defaults but merge is not \"update\"; skipping",
					"case", t.Case, "merge", c.Merge)
			} else if cumulative > 0 {
				delay := time.Duration(cumulative) * time.Second
				s.schedule(delay, filePath, c, configDir)
			}
		}

		if t.Duration > 0 {
			cumulative += int64(t.Duration)
		}
	}
}

// schedule spawns a single goroutine that waits for delay, then applies
// the case's defaults to the file via persist.Update.
//
// Each goroutine owns a per-file context derived from the generation context
// so CancelFile can cancel it individually without touching other files.
//
// NOTE: unlike the HTTP proxy scheduler, no request context (refCtx) is
// passed here. gRPC defaults use renderGRPCTemplate which handles {{uuid}},
// {{now}}, etc. If future defaults files need request-scoped placeholders,
// a gRPC equivalent of RefContext will be needed.
func (s *grpcTransitionScheduler) schedule(delay time.Duration, filePath string, c config.Case, configDir string) {
	s.mu.Lock()
	gen := s.gen
	// If the current generation has already been cancelled (Stop or Reset
	// is in flight), do not spawn — calling gen.wg.Add(1) while another
	// goroutine is in gen.wg.Wait() with a zero counter panics.
	if gen.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	fileCtx, fileCancel := context.WithCancel(gen.ctx)
	pm := &pendingMutation{cancel: fileCancel}
	s.pending[filePath] = append(s.pending[filePath], pm)
	gen.wg.Add(1)
	s.mu.Unlock()

	go func() {
		defer gen.wg.Done()
		defer func() {
			// Remove this entry from the pending map once the goroutine is
			// done (either fired or was cancelled). Safe if already removed
			// by CancelFile.
			s.mu.Lock()
			list := s.pending[filePath]
			for i, entry := range list {
				if entry == pm {
					s.pending[filePath] = append(list[:i], list[i+1:]...)
					break
				}
			}
			if len(s.pending[filePath]) == 0 {
				delete(s.pending, filePath)
			}
			s.mu.Unlock()
			fileCancel() // idempotent
		}()

		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-timer.C:
			incoming := loadGRPCDefaults(c.Defaults, nil, nil, configDir)
			if len(incoming) == 0 {
				logger.Warn("grpc deferred transition: no defaults to apply",
					"file", filePath, "defaults", c.Defaults)
				return
			}

			if _, err := persist.Update(filePath, incoming); err != nil {
				if persist.IsNotFound(err) {
					logger.Warn("grpc deferred transition: file was deleted before transition",
						"file", filePath)
				} else {
					logger.Error("grpc deferred transition: update failed",
						"file", filePath, "err", err)
				}
				return
			}

			logger.Info("grpc deferred transition applied",
				"file", filePath, "defaults", c.Defaults, "delay", delay.String())

		case <-fileCtx.Done():
			return
		}
	}()
}

// HasPending reports whether there is at least one in-flight deferred
// mutation targeting the given filePath.
func (s *grpcTransitionScheduler) HasPending(filePath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending[filePath]) > 0
}

// ScheduleIfIdle arms the route's transition timeline only when there is
// no pending mutation already targeting filePath. Used by the handler on
// the merge="update" persist path so that a client write against an
// existing entity stub does not cancel an in-flight transition timer that
// was armed by an earlier create.
//
// Note on concurrency: the pending check and the subsequent Schedule call
// are not held under a single mutex lease, so two concurrent callers
// against the same idle file path can both observe "no pending" and both
// arm the timeline. The second arm immediately cancels the first via
// Schedule's CancelFile, leaving exactly one timeline alive — consistent
// state, just a wasted rearm. The race window is microseconds and
// inconsequential for a mocking tool with second-scale transitions.
func (s *grpcTransitionScheduler) ScheduleIfIdle(route *config.GRPCRoute, filePath, configDir string) {
	if s.HasPending(filePath) {
		return
	}
	s.Schedule(route, filePath, configDir)
}

// CancelFile cancels any in-flight deferred mutations targeting the given
// filePath. Called when a file is deleted (via handler's delete merge path)
// or rescheduled (via Schedule) so stale goroutines cannot stomp a
// subsequently-recreated file.
func (s *grpcTransitionScheduler) CancelFile(filePath string) {
	s.mu.Lock()
	entries := s.pending[filePath]
	delete(s.pending, filePath)
	s.mu.Unlock()

	for _, pm := range entries {
		pm.cancel()
	}
}

// Reset cancels all pending mutations and creates a fresh generation.
// The pending map is cleared atomically with the generation swap so that
// a concurrent Schedule() on the new generation does not see stale entries
// from goroutines that are still draining their deferred cleanup.
func (s *grpcTransitionScheduler) Reset(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	newGen := &grpcSchedulerGeneration{ctx: ctx, cancel: cancel}

	s.mu.Lock()
	oldGen := s.gen
	oldGen.cancel()
	s.gen = newGen
	s.pending = make(map[string][]*pendingMutation)
	s.mu.Unlock()

	oldGen.wg.Wait()
}

// Stop cancels all pending mutations and waits for goroutines to finish.
func (s *grpcTransitionScheduler) Stop() {
	s.mu.Lock()
	s.gen.cancel()
	gen := s.gen
	s.mu.Unlock()
	gen.wg.Wait()
}
