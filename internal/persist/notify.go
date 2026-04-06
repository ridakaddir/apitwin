// Package persist — file mutation notification system.
//
// When a stub file is created, updated, or deleted, the package can notify
// registered listeners so that cross-references (e.g. endpoint → deployments
// directory) can be re-evaluated automatically.
package persist

import (
	"path/filepath"
	"sync"

	"github.com/ridakaddir/apitwin/internal/logger"
)

// ChangeType describes what happened to a stub file.
type ChangeType int

const (
	FileCreated ChangeType = iota
	FileUpdated
	FileDeleted
)

// ChangeEvent describes a single stub file mutation.
type ChangeEvent struct {
	FilePath string     // Absolute path to the file that changed.
	Dir      string     // Parent directory of the changed file.
	Type     ChangeType // What happened.
}

// ChangeListener is called when one or more stub files have been mutated.
// Events are delivered in a batch to allow efficient bulk processing.
type ChangeListener func(events []ChangeEvent)

var (
	listenersMu sync.Mutex
	listeners   []ChangeListener
)

// OnChange registers a callback invoked whenever stub files are mutated via
// AppendToDir, Update, or DeleteFile. Returns an unsubscribe function that
// removes the listener. Safe for concurrent use.
func OnChange(fn ChangeListener) func() {
	listenersMu.Lock()
	listeners = append(listeners, fn)
	idx := len(listeners) - 1
	listenersMu.Unlock()

	return func() {
		listenersMu.Lock()
		defer listenersMu.Unlock()
		// Remove the entry from the slice to avoid leaking references.
		if idx < len(listeners) && listeners[idx] != nil {
			listeners = append(listeners[:idx], listeners[idx+1:]...)
			// Invalidate this index so a double-call is a no-op.
			idx = -1
		}
	}
}

// ResetListeners removes all registered change listeners.
// Intended for tests that need a clean state.
func ResetListeners() {
	listenersMu.Lock()
	defer listenersMu.Unlock()
	listeners = nil
}

// notify delivers a single change event to all registered listeners.
// Active listeners are snapshot-copied under the lock and then called
// outside the lock so that listener code cannot deadlock the system.
// Each listener is called inside a recover block to prevent a panicking
// listener from crashing the server.
func notify(filePath string, changeType ChangeType) {
	listenersMu.Lock()
	fns := make([]ChangeListener, len(listeners))
	copy(fns, listeners)
	listenersMu.Unlock()

	if len(fns) == 0 {
		return
	}

	event := ChangeEvent{
		FilePath: filePath,
		Dir:      filepath.Dir(filePath),
		Type:     changeType,
	}
	batch := []ChangeEvent{event}

	for _, fn := range fns {
		if fn == nil {
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("persist change listener panicked", "err", r)
				}
			}()
			fn(batch)
		}()
	}
}
