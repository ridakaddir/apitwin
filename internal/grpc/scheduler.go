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
type grpcTransitionScheduler struct {
	mu  sync.Mutex
	gen *grpcSchedulerGeneration
}

type grpcSchedulerGeneration struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newGRPCTransitionScheduler(parent context.Context) *grpcTransitionScheduler {
	ctx, cancel := context.WithCancel(parent)
	return &grpcTransitionScheduler{
		gen: &grpcSchedulerGeneration{ctx: ctx, cancel: cancel},
	}
}

// Schedule inspects a gRPC route's transitions and spawns background goroutines
// for each transition case that needs a deferred file mutation.
//
// filePath is the absolute path to the file created by the append operation.
// configDir is needed to resolve relative defaults file paths.
func (s *grpcTransitionScheduler) Schedule(route *config.GRPCRoute, filePath, configDir string) {
	if len(route.Transitions) < 2 {
		return
	}

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

func (s *grpcTransitionScheduler) schedule(delay time.Duration, filePath string, c config.Case, configDir string) {
	s.mu.Lock()
	gen := s.gen
	gen.wg.Add(1)
	s.mu.Unlock()

	go func() {
		defer gen.wg.Done()

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

		case <-gen.ctx.Done():
			return
		}
	}()
}

// Reset cancels all pending mutations and creates a fresh generation.
func (s *grpcTransitionScheduler) Reset(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	newGen := &grpcSchedulerGeneration{ctx: ctx, cancel: cancel}

	s.mu.Lock()
	oldGen := s.gen
	oldGen.cancel()
	s.gen = newGen
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
