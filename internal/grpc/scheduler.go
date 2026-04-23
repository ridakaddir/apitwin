package grpc

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
//
// Persistence: defaults are resolved eagerly at Schedule time and the
// resolved payload + fireAt is recorded via the shared ScheduledPersister
// (with Scope="grpc"). Fired items are pruned; cancelled items (CancelFile
// or hot-reload) are also pruned; shutdown leaves items on disk for next
// boot to re-arm.
type grpcTransitionScheduler struct {
	mu        sync.Mutex
	gen       *transitions.Generation
	pending   map[string][]*pendingMutation // keyed on target filePath
	persister *runtimepersist.ScheduledPersister
}

// pendingMutation is a single in-flight scheduled update for a given filePath.
// Each entry owns its own context derived from the generation context so it
// can be cancelled individually without affecting other files. The id lets
// cleanup Remove() the matching persisted entry on fire or cancel.
type pendingMutation struct {
	id     string
	cancel context.CancelFunc
}

var grpcIdGen uint64

func nextGRPCScheduledID() string {
	t := time.Now().UnixNano()
	grpcIdGen++
	return "grpc-" + time.Unix(0, t).Format("20060102T150405.000000000") + "-" + grpcItoa(grpcIdGen)
}

func grpcItoa(u uint64) string {
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

func newGRPCTransitionScheduler(parent context.Context) *grpcTransitionScheduler {
	return &grpcTransitionScheduler{
		gen:     transitions.NewGeneration(parent),
		pending: make(map[string][]*pendingMutation),
	}
}

// SetPersister wires a ScheduledPersister so new Schedule calls persist
// pending items and fired/cancelled items are pruned. Passing nil detaches
// the persister. Must be called before any Schedule call to avoid
// persisting partial state.
func (s *grpcTransitionScheduler) SetPersister(p *runtimepersist.ScheduledPersister) {
	s.mu.Lock()
	s.persister = p
	s.mu.Unlock()
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
				incoming := loadGRPCDefaults(c.Defaults, nil, nil, configDir)
				if len(incoming) == 0 {
					logger.Warn("grpc deferred transition: eager defaults resolved to empty, skipping",
						"case", t.Case, "defaults", c.Defaults)
					if t.Duration > 0 {
						cumulative += int64(t.Duration)
					}
					continue
				}
				id := nextGRPCScheduledID()
				fireAt := time.Now().Add(delay)
				s.persistItem(id, route.Match, filePath, fireAt, incoming, c.Defaults)
				s.scheduleResolved(id, delay, filePath, incoming, c.Defaults)
			}
		}

		if t.Duration > 0 {
			cumulative += int64(t.Duration)
		}
	}
}

// persistItem records a new pending item. Best-effort; errors are logged
// but do not block scheduling.
func (s *grpcTransitionScheduler) persistItem(id, routeKey, filePath string, fireAt time.Time, payload map[string]interface{}, source string) {
	s.mu.Lock()
	p := s.persister
	s.mu.Unlock()
	if p == nil {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		logger.Warn("grpc deferred transition: marshal payload failed", "err", err)
		return
	}
	if err := p.Add(runtimepersist.ScheduledItem{
		ID:              id,
		Scope:           "grpc",
		RouteKey:        routeKey,
		FilePath:        filePath,
		FireAt:          fireAt,
		ResolvedPayload: raw,
		Source:          source,
	}); err != nil {
		logger.Warn("grpc deferred transition: persist Add failed", "err", err)
	}
}

// scheduleResolved spawns a goroutine that waits delay, then applies the
// eager-resolved payload via persist.Update. Used by Schedule (fresh
// arming) and Rearm (post-restart). id is the persister item id so the
// fire-time Remove() matches the original Add.
func (s *grpcTransitionScheduler) scheduleResolved(id string, delay time.Duration, filePath string, payload map[string]interface{}, source string) {
	s.mu.Lock()
	gen := s.gen
	if gen.Ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	fileCtx, fileCancel := context.WithCancel(gen.Ctx)
	pm := &pendingMutation{id: id, cancel: fileCancel}
	s.pending[filePath] = append(s.pending[filePath], pm)
	gen.WG.Add(1)
	p := s.persister
	s.mu.Unlock()

	go func() {
		defer gen.WG.Done()
		defer func() {
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
			fileCancel()
		}()

		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-timer.C:
			if _, err := persist.Update(filePath, payload); err != nil {
				if persist.IsNotFound(err) {
					logger.Warn("grpc deferred transition: file was deleted before transition",
						"file", filePath)
				} else {
					logger.Error("grpc deferred transition: update failed",
						"file", filePath, "err", err)
				}
				if p != nil {
					_ = p.Remove(id)
				}
				return
			}
			if p != nil {
				_ = p.Remove(id)
			}
			logger.Info("grpc deferred transition applied",
				"file", filePath, "defaults", source, "delay", delay.String())

		case <-fileCtx.Done():
			// Cancelled via CancelFile (delete / reschedule) or gen ctx
			// (Stop / Reset). Fire-time removal is handled by the
			// caller (CancelFile for explicit cancel; Reset for
			// hot-reload; Stop intentionally leaves items on disk).
			return
		}
	}()
}

// Rearm re-arms pending items loaded from the persister on startup. Items
// whose fireAt is already in the past are applied synchronously; future
// items are scheduled with the remaining delay using their original id.
func (s *grpcTransitionScheduler) Rearm(items []runtimepersist.ScheduledItem) {
	now := time.Now()
	s.mu.Lock()
	p := s.persister
	s.mu.Unlock()

	for _, it := range items {
		if it.Scope != "grpc" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(it.ResolvedPayload, &payload); err != nil {
			logger.Warn("grpc rearm: invalid resolvedPayload, dropping",
				"id", it.ID, "err", err)
			if p != nil {
				_ = p.Remove(it.ID)
			}
			continue
		}
		if !it.FireAt.After(now) {
			if _, err := persist.Update(it.FilePath, payload); err != nil {
				if persist.IsNotFound(err) {
					logger.Warn("grpc rearm: past-due target missing",
						"file", it.FilePath)
				} else {
					logger.Error("grpc rearm: past-due update failed",
						"file", it.FilePath, "err", err)
				}
			} else {
				logger.Info("grpc rearm: past-due transition applied",
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
// filePath and prunes the matching persisted items (so a deleted resource
// does not come back on the next boot). Called when a file is deleted or
// rescheduled.
func (s *grpcTransitionScheduler) CancelFile(filePath string) {
	s.mu.Lock()
	entries := s.pending[filePath]
	delete(s.pending, filePath)
	p := s.persister
	s.mu.Unlock()

	for _, pm := range entries {
		pm.cancel()
		if p != nil {
			_ = p.Remove(pm.id)
		}
	}
}

// Reset cancels all pending mutations and creates a fresh generation.
// Hot-reload semantics: wipe the gRPC-scoped persisted schedule too,
// since the new config may carry different transitions.
func (s *grpcTransitionScheduler) Reset(parent context.Context) {
	newGen := transitions.NewGeneration(parent)

	s.mu.Lock()
	oldGen := s.gen
	s.gen = newGen
	s.pending = make(map[string][]*pendingMutation)
	p := s.persister
	s.mu.Unlock()

	oldGen.Stop()

	if p != nil {
		if err := p.RemoveByScope("grpc"); err != nil {
			logger.Warn("grpc scheduler reset: clearing persisted schedule failed", "err", err)
		}
	}
}

// Stop cancels all pending mutations and waits for goroutines to finish.
// Intentionally does NOT clear the persister — pending items must survive
// shutdown so the next start can re-arm them.
func (s *grpcTransitionScheduler) Stop() {
	s.mu.Lock()
	gen := s.gen
	s.mu.Unlock()
	gen.Stop()
}
