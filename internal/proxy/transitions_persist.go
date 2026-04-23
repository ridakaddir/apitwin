package proxy

import (
	"sync"
	"time"

	"github.com/ridakaddir/apitwin/internal/logger"
	runtimepersist "github.com/ridakaddir/apitwin/internal/runtime/persist"
)

// transitionsDiskPersister adapts the transitions.Persister interface to a
// JSON file on disk. Both the REST and gRPC state machines write through
// this adapter under their respective scope keys ("rest" / "grpc"), so a
// single transitions.json holds both sides' FirstHit timestamps.
//
// Safe for concurrent use: the internal mutex serialises read-modify-write
// across scopes.
type transitionsDiskPersister struct {
	mu   sync.Mutex
	path string
}

func newTransitionsDiskPersister(path string) *transitionsDiskPersister {
	return &transitionsDiskPersister{path: path}
}

// PersistFirstHit merges the given scope's FirstHit map into the on-disk
// file. Errors are logged and swallowed — the in-memory state is the
// source of truth for the running server; the file is a best-effort
// record for cross-restart recovery.
func (p *transitionsDiskPersister) PersistFirstHit(scope string, firstHit map[string]time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	file, err := runtimepersist.LoadTransitions(p.path)
	if err != nil {
		logger.Warn("transitions persist: load failed", "path", p.path, "err", err)
		return
	}
	if len(firstHit) == 0 {
		// Empty snapshot = scope was reset. Drop the whole scope entry.
		delete(file.Entries, scope)
	} else {
		copy := make(map[string]time.Time, len(firstHit))
		for k, v := range firstHit {
			copy[k] = v
		}
		file.Entries[scope] = runtimepersist.TransitionsScope{FirstHit: copy}
	}
	if err := runtimepersist.SaveTransitions(p.path, file); err != nil {
		logger.Warn("transitions persist: save failed", "path", p.path, "err", err)
	}
}
