// Package persist stores runtime-generated state (transition first-hit
// timestamps, pending deferred mutations) under the runtime dir so it
// survives server restarts.
//
// All writes are atomic via tmp-file + os.Rename so a crash mid-write
// cannot leave a partially serialised JSON file on disk.
package persist

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// transitionsVersion is the on-disk format version for transitions.json.
// Bump if the shape changes incompatibly.
const transitionsVersion = 1

// TransitionsFile is the on-disk shape for transition FirstHit state.
// It is keyed by scope ("rest" / "grpc") so HTTP and gRPC state live in
// one file without risk of collision.
type TransitionsFile struct {
	Version int                         `json:"version"`
	Entries map[string]TransitionsScope `json:"entries"`
}

// TransitionsScope holds the FirstHit map for one scope (HTTP or gRPC).
type TransitionsScope struct {
	FirstHit map[string]time.Time `json:"firstHit"`
}

// EmptyTransitions returns an initialised TransitionsFile with no entries.
func EmptyTransitions() *TransitionsFile {
	return &TransitionsFile{Version: transitionsVersion, Entries: map[string]TransitionsScope{}}
}

// Scope returns the FirstHit map for the given scope, or nil if not present.
// Callers can pass the result directly as the seed for NewPersistentState.
func (f *TransitionsFile) Scope(scope string) map[string]time.Time {
	if f == nil {
		return nil
	}
	s, ok := f.Entries[scope]
	if !ok {
		return nil
	}
	return s.FirstHit
}

// LoadTransitions reads a TransitionsFile from path. A missing file returns
// an empty file (not an error) so callers can treat first-run and subsequent
// runs uniformly.
func LoadTransitions(path string) (*TransitionsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return EmptyTransitions(), nil
		}
		return nil, fmt.Errorf("reading %q: %w", path, err)
	}
	var f TransitionsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("decoding %q: %w", path, err)
	}
	if f.Entries == nil {
		f.Entries = map[string]TransitionsScope{}
	}
	return &f, nil
}

// SaveTransitions writes f to path atomically. Callers can invoke this
// from concurrent goroutines safely — the internal mutex serialises writes
// against each other.
func SaveTransitions(path string, f *TransitionsFile) error {
	transitionsMu.Lock()
	defer transitionsMu.Unlock()
	if f == nil {
		f = EmptyTransitions()
	}
	if f.Version == 0 {
		f.Version = transitionsVersion
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding transitions: %w", err)
	}
	return atomicWrite(path, data)
}

// DeleteTransitions removes the transitions file. Missing file is not an
// error — used by hot-reload and --reset-runtime paths where the caller
// doesn't care whether anything was actually written.
func DeleteTransitions(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// transitionsMu serialises concurrent SaveTransitions calls. Callers may
// write FirstHit updates from request goroutines.
var transitionsMu sync.Mutex

// atomicWrite writes data to path via a sibling tmp file + rename so a
// crash mid-write cannot leave a partial file on disk. Uses the same dir
// as the target so rename stays atomic on POSIX.
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating dir for %q: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating tmp for %q: %w", path, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("writing tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("closing tmp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename tmp → %q: %w", path, err)
	}
	return nil
}
