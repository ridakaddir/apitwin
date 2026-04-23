package transitions

import (
	"sync"
	"testing"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
)

type fakeRoute struct {
	key         string
	transitions []config.Transition
}

func (r *fakeRoute) GetTransitions() []config.Transition { return r.transitions }

type capturingPersister struct {
	mu    sync.Mutex
	calls []persisterCall
}

type persisterCall struct {
	scope string
	snap  map[string]time.Time
}

func (p *capturingPersister) PersistFirstHit(scope string, firstHit map[string]time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make(map[string]time.Time, len(firstHit))
	for k, v := range firstHit {
		cp[k] = v
	}
	p.calls = append(p.calls, persisterCall{scope: scope, snap: cp})
}

func (p *capturingPersister) last() persisterCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[len(p.calls)-1]
}

func (p *capturingPersister) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

func TestPersistentState_SeedFirstHitIsUsed(t *testing.T) {
	seeded := time.Now().Add(-10 * time.Second).UTC()
	seed := map[string]time.Time{"GET /x": seeded}

	p := &capturingPersister{}
	st := NewPersistentState[*fakeRoute](
		func(r *fakeRoute) string { return r.key },
		"rest", p, seed,
	)

	route := &fakeRoute{
		key: "GET /x",
		transitions: []config.Transition{
			{Case: "initial", Duration: 5},
			{Case: "ready"},
		},
	}

	// Seeded FirstHit is 10s old, transitions = [initial 5s, ready].
	// elapsed > 5 → terminal "ready".
	if got := st.Resolve(route); got != "ready" {
		t.Errorf("Resolve = %q, want ready (seeded FirstHit should NOT be overwritten)", got)
	}
	// Resolve on an already-seen key should not emit a persister call.
	if p.count() != 0 {
		t.Errorf("persister called %d times on seen key, want 0", p.count())
	}
}

func TestPersistentState_ResolveEmitsPersist(t *testing.T) {
	p := &capturingPersister{}
	st := NewPersistentState[*fakeRoute](
		func(r *fakeRoute) string { return r.key },
		"rest", p, nil,
	)

	route := &fakeRoute{
		key: "GET /y",
		transitions: []config.Transition{
			{Case: "initial", Duration: 5},
			{Case: "ready"},
		},
	}
	_ = st.Resolve(route)
	if p.count() != 1 {
		t.Fatalf("expected 1 persister call after first Resolve, got %d", p.count())
	}
	if got := p.last().scope; got != "rest" {
		t.Errorf("scope = %q, want rest", got)
	}
	if _, ok := p.last().snap["GET /y"]; !ok {
		t.Errorf("snap missing GET /y: %v", p.last().snap)
	}

	// A second Resolve should NOT re-persist (same key, same t0).
	_ = st.Resolve(route)
	if p.count() != 1 {
		t.Errorf("second Resolve persisted again: count=%d", p.count())
	}
}

func TestPersistentState_ResetPersistsEmpty(t *testing.T) {
	p := &capturingPersister{}
	st := NewPersistentState[*fakeRoute](
		func(r *fakeRoute) string { return r.key },
		"rest", p,
		map[string]time.Time{"GET /x": time.Now()},
	)
	st.Reset()
	if p.count() != 1 {
		t.Fatalf("expected 1 persister call after Reset, got %d", p.count())
	}
	if got := len(p.last().snap); got != 0 {
		t.Errorf("snap after Reset = %d entries, want 0", got)
	}
}

func TestPersistentState_ResetMatchPersistsOnlyWhenChanged(t *testing.T) {
	p := &capturingPersister{}
	st := NewPersistentState[*fakeRoute](
		func(r *fakeRoute) string { return r.key },
		"grpc", p,
		map[string]time.Time{"svc.Foo/Bar": time.Now(), "svc.Baz/Qux": time.Now()},
	)

	// No match → no persister call.
	st.ResetMatch("nothing-matches-this")
	if p.count() != 0 {
		t.Errorf("ResetMatch with no match should not persist, got %d calls", p.count())
	}

	st.ResetMatch("svc.Foo/Bar")
	if p.count() != 1 {
		t.Fatalf("expected 1 persister call after ResetMatch, got %d", p.count())
	}
	if _, ok := p.last().snap["svc.Foo/Bar"]; ok {
		t.Error("matched key should be gone from snap")
	}
}
