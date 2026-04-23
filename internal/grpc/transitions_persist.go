package grpc

import (
	"sync"
	"time"

	"github.com/ridakaddir/apitwin/internal/logger"
	runtimepersist "github.com/ridakaddir/apitwin/internal/runtime/persist"
)

// transitionsDiskPersister adapts the transitions.Persister interface to a
// JSON file on disk. Shared with the REST server — both sides write through
// their own copy under their respective scope keys ("grpc" here), so one
// transitions.json holds HTTP and gRPC state side-by-side.
type transitionsDiskPersister struct {
	mu   sync.Mutex
	path string
}

func newTransitionsDiskPersister(path string) *transitionsDiskPersister {
	return &transitionsDiskPersister{path: path}
}

// PersistFirstHit merges the given scope's FirstHit map into the on-disk
// file. Errors are logged and swallowed.
func (p *transitionsDiskPersister) PersistFirstHit(scope string, firstHit map[string]time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	file, err := runtimepersist.LoadTransitions(p.path)
	if err != nil {
		logger.Warn("grpc transitions persist: load failed", "path", p.path, "err", err)
		return
	}
	if len(firstHit) == 0 {
		delete(file.Entries, scope)
	} else {
		copy := make(map[string]time.Time, len(firstHit))
		for k, v := range firstHit {
			copy[k] = v
		}
		file.Entries[scope] = runtimepersist.TransitionsScope{FirstHit: copy}
	}
	if err := runtimepersist.SaveTransitions(p.path, file); err != nil {
		logger.Warn("grpc transitions persist: save failed", "path", p.path, "err", err)
	}
}
