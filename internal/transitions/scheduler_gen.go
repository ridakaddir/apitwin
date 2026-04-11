package transitions

import (
	"context"
	"sync"
)

// Generation groups a cancellable context and its WaitGroup for one scheduler
// lifecycle between resets. Both the REST and gRPC transition schedulers use
// this primitive to spawn deferred-mutation goroutines without racing
// sync.WaitGroup's Add/Wait contract.
//
// Usage pattern:
//
//	// spawn (under caller's own mutex)
//	gen := sched.gen
//	if gen.Ctx.Err() != nil { return }  // already cancelled; refuse
//	gen.WG.Add(1)
//	go func() {
//	    defer gen.WG.Done()
//	    select {
//	    case <-time.After(delay): ...
//	    case <-gen.Ctx.Done(): return
//	    }
//	}()
//
//	// reset
//	old := sched.gen
//	sched.gen = NewGeneration(parent)
//	old.Stop()  // cancel + drain (caller must release its own mutex first)
//
// Note: the cancelled-check before Add must happen under the same lock that
// guards generation swaps, otherwise a reset can land between the check and
// the Add.
type Generation struct {
	Ctx    context.Context
	Cancel context.CancelFunc
	WG     sync.WaitGroup
}

// NewGeneration constructs a fresh generation with a context derived from parent.
func NewGeneration(parent context.Context) *Generation {
	ctx, cancel := context.WithCancel(parent)
	return &Generation{Ctx: ctx, Cancel: cancel}
}

// Stop cancels the generation's context and waits for any in-flight goroutines
// to observe the cancel and exit. Must be called with any owning mutex
// released, otherwise goroutines that need that mutex to clean up will deadlock.
func (g *Generation) Stop() {
	g.Cancel()
	g.WG.Wait()
}
