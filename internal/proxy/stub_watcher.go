package proxy

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ridakaddir/apitwin/internal/logger"
	"github.com/ridakaddir/apitwin/internal/persist"
)

// stubRefPattern matches {{ref:path...}} tokens in stub file content (outside
// surrounding JSON quotes so we can scan raw file bytes).
var stubRefPattern = regexp.MustCompile(`\{\{ref:([^}]*(?:\{[^}]+\}[^}]*)*)\}\}`)

// stubDynamicPattern matches dynamic placeholders inside ref tokens:
// {.field}, {.nested.field}, {path.param}, {query.param}, {header.Name}.
// These are replaced with "*" when building directory watch patterns.
var stubDynamicPattern = regexp.MustCompile(`\{[^}]+\}`)

// StubWatcher monitors persist change events and automatically re-evaluates
// stub files whose cross-references point at directories that have changed.
//
// Architecture:
//   - On startup it scans all stub directories for files containing {{ref:...}}
//     tokens and builds a dependency map: "watched directory pattern" → list of
//     files that reference it.
//   - persist.OnChange delivers events whenever a stub file is created, updated
//     or deleted. The watcher checks whether the changed file's parent directory
//     matches any watched directory and, if so, queues a batch re-evaluation of
//     all dependent files.
//   - Re-evaluation is a no-op for the file contents themselves (refs are always
//     resolved at read-time); but the watcher's value is logging the update for
//     observability and, crucially, triggering config-reload callbacks so that
//     any downstream caches are invalidated.
//
// Concurrency: all methods are safe for concurrent use.
type StubWatcher struct {
	mu        sync.RWMutex
	configDir string

	// deps maps a normalised directory path (relative to configDir, e.g.
	// "stubs/deployments") to the set of stub files that contain a ref
	// pointing into that directory.
	// The directory patterns have dynamic placeholders replaced with a
	// regex-friendly wildcard so that "stubs/deployments/123/" and
	// "stubs/deployments/456/" both match "stubs/deployments/*/".
	deps map[string]*dependency

	// onUpdate is called after a batch of dependent files have been
	// identified as affected by a persist change.  The server wires this
	// to the config loader's reload callbacks.
	onUpdate func()

	// batchWindow groups rapid-fire persist events into a single batch.
	batchWindow time.Duration

	// pending tracks directories with pending batch updates.
	pending   map[string]bool
	pendingMu sync.Mutex
	timer     *time.Timer

	// stopped is set to 1 when Stop() is called; handleChanges checks
	// this flag to short-circuit after shutdown.
	stopped atomic.Int32

	// unsubscribe removes the persist.OnChange listener.
	unsubscribe func()
}

// dependency tracks a single ref-based dependency.
type dependency struct {
	// dirPattern is the directory pattern relative to configDir,
	// with dynamic segments replaced by "*" (e.g. "stubs/deployments/*/").
	dirPattern string

	// compiledRe is the compiled regex form of dirPattern for matching
	// against absolute directory paths.
	compiledRe *regexp.Regexp

	// files lists absolute paths of stub files that contain a ref
	// pointing at this directory pattern.
	files map[string]bool
}

// StubWatcherOptions configures the StubWatcher.
type StubWatcherOptions struct {
	ConfigDir   string
	OnUpdate    func()
	BatchWindow time.Duration // defaults to 100ms if zero
}

// NewStubWatcher creates a StubWatcher and scans all stub files to discover
// cross-reference dependencies. It registers itself as a persist.OnChange
// listener to receive file mutation events.
func NewStubWatcher(opts StubWatcherOptions) *StubWatcher {
	bw := opts.BatchWindow
	if bw == 0 {
		bw = 100 * time.Millisecond
	}

	sw := &StubWatcher{
		configDir:   opts.ConfigDir,
		deps:        make(map[string]*dependency),
		onUpdate:    opts.OnUpdate,
		batchWindow: bw,
		pending:     make(map[string]bool),
	}

	// Scan stub directories for cross-references.
	sw.scan()

	// Register as a persist change listener and store the unsubscribe function.
	sw.unsubscribe = persist.OnChange(sw.handleChanges)

	return sw
}

// Stop cancels any pending batch timer and deregisters the persist change
// listener. Safe to call multiple times.
func (sw *StubWatcher) Stop() {
	sw.stopped.Store(1)

	sw.pendingMu.Lock()
	if sw.timer != nil {
		sw.timer.Stop()
		sw.timer = nil
	}
	sw.pendingMu.Unlock()

	if sw.unsubscribe != nil {
		sw.unsubscribe()
	}
}

// scan walks all .json files under configDir looking for {{ref:...}} tokens
// and registers dependencies. Uses filepath.WalkDir for efficiency.
func (sw *StubWatcher) scan() {
	if sw.configDir == "" {
		return
	}

	err := filepath.WalkDir(sw.configDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		sw.registerRefsInFile(path, data)
		return nil
	})
	if err != nil {
		logger.Warn("stub watcher: scan error", "err", err)
	}

	sw.mu.RLock()
	depCount := len(sw.deps)
	fileCount := 0
	for _, d := range sw.deps {
		fileCount += len(d.files)
	}
	sw.mu.RUnlock()

	if depCount > 0 {
		logger.Info("stub watcher: discovered dependencies",
			"patterns", depCount, "dependent_files", fileCount)
	}
}

// registerRefsInFile extracts all {{ref:...}} tokens from file content and
// registers dependencies for any that reference directories.
func (sw *StubWatcher) registerRefsInFile(filePath string, content []byte) {
	matches := stubRefPattern.FindAllSubmatch(content, -1)
	if len(matches) == 0 {
		return
	}

	for _, match := range matches {
		token := string(match[1]) // the path?params part

		// Extract just the path (before any ?query params).
		refPath := token
		if idx := strings.Index(refPath, "?"); idx != -1 {
			refPath = refPath[:idx]
		}

		// Only care about directory references (ending with /).
		if !strings.HasSuffix(refPath, "/") {
			continue
		}

		// Security: reject paths with directory traversal.
		if strings.Contains(refPath, "..") {
			logger.Warn("stub watcher: skipping ref with path traversal", "ref", refPath)
			continue
		}

		// Replace dynamic placeholders with * for pattern matching.
		// e.g. "stubs/deployments/{.endpointId}/" → "stubs/deployments/*/"
		pattern := stubDynamicPattern.ReplaceAllString(refPath, "*")

		sw.addDependency(pattern, filePath)
	}
}

// addDependency registers that filePath depends on the given directory pattern.
func (sw *StubWatcher) addDependency(dirPattern, filePath string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	dep, exists := sw.deps[dirPattern]
	if !exists {
		// Build a regex from the pattern.
		// Convert "stubs/deployments/*/" to match any concrete directory
		// like "<configDir>/stubs/deployments/12345".
		absPattern := dirPattern
		if sw.configDir != "" {
			absPattern = filepath.Join(sw.configDir, dirPattern)
		}
		// Escape regex special chars, then replace the literal "*" with "[^/]+"
		escaped := regexp.QuoteMeta(strings.TrimSuffix(absPattern, "/"))
		reStr := "^" + strings.ReplaceAll(escaped, `\*`, `[^/]+`) + "$"
		re, err := regexp.Compile(reStr)
		if err != nil {
			logger.Warn("stub watcher: invalid dependency pattern",
				"pattern", dirPattern, "err", err)
			return
		}

		dep = &dependency{
			dirPattern: dirPattern,
			compiledRe: re,
			files:      make(map[string]bool),
		}
		sw.deps[dirPattern] = dep
	}

	dep.files[filePath] = true
}

// handleChanges is the persist.ChangeListener callback.
// It collects matched directories under mu.RLock, then adds them to the
// pending set in a single pendingMu.Lock block to avoid interleaved locking.
func (sw *StubWatcher) handleChanges(events []persist.ChangeEvent) {
	if sw.stopped.Load() != 0 {
		return
	}

	sw.mu.RLock()
	if len(sw.deps) == 0 {
		sw.mu.RUnlock()
		return
	}

	// Collect all matched directories first (under mu.RLock only).
	var matchedDirs []string
	for _, event := range events {
		changedDir := event.Dir
		for _, dep := range sw.deps {
			if dep.compiledRe.MatchString(changedDir) {
				matchedDirs = append(matchedDirs, changedDir)
			}
		}
	}
	sw.mu.RUnlock()

	if len(matchedDirs) == 0 {
		return
	}

	// Add all matched directories to pending in a single lock acquisition.
	sw.pendingMu.Lock()
	for _, d := range matchedDirs {
		sw.pending[d] = true
	}
	// Debounce: reset the timer on each event within the batch window.
	if sw.timer != nil {
		sw.timer.Stop()
	}
	sw.timer = time.AfterFunc(sw.batchWindow, sw.processBatch)
	sw.pendingMu.Unlock()
}

// processBatch runs after the debounce window expires and notifies dependents.
func (sw *StubWatcher) processBatch() {
	if sw.stopped.Load() != 0 {
		return
	}

	sw.pendingMu.Lock()
	dirs := make([]string, 0, len(sw.pending))
	for d := range sw.pending {
		dirs = append(dirs, d)
	}
	sw.pending = make(map[string]bool)
	sw.pendingMu.Unlock()

	if len(dirs) == 0 {
		return
	}

	// Collect all affected dependent files.
	affected := make(map[string]bool)
	sw.mu.RLock()
	for _, dir := range dirs {
		for _, dep := range sw.deps {
			if dep.compiledRe.MatchString(dir) {
				for f := range dep.files {
					affected[f] = true
				}
			}
		}
	}
	sw.mu.RUnlock()

	if len(affected) == 0 {
		return
	}

	logger.Info("stub watcher: dependencies changed",
		"directories", len(dirs), "affected_files", len(affected))

	// Notify callback (triggers config-reload-like invalidation).
	if sw.onUpdate != nil {
		sw.onUpdate()
	}
}

// AddFile registers a newly created stub file for dependency tracking.
// Called when a persist append creates a file whose content may contain refs.
// This handles the case where a new endpoint is created with refs to deployment
// directories — the watcher needs to know about it immediately.
func (sw *StubWatcher) AddFile(filePath string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	sw.registerRefsInFile(filePath, data)
}

// RemoveFile removes a stub file from all dependency tracking.
// Called when a file is deleted via persist.
func (sw *StubWatcher) RemoveFile(filePath string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	for _, dep := range sw.deps {
		delete(dep.files, filePath)
	}
}

// DependencyCount returns the number of tracked directory patterns.
// Used for testing and observability.
func (sw *StubWatcher) DependencyCount() int {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return len(sw.deps)
}

// AffectedFiles returns the set of files that depend on the given directory.
// Used for testing.
func (sw *StubWatcher) AffectedFiles(dir string) []string {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	var result []string
	for _, dep := range sw.deps {
		if dep.compiledRe.MatchString(dir) {
			for f := range dep.files {
				result = append(result, f)
			}
		}
	}
	return result
}

// DumpDeps returns a debug-friendly summary of all dependencies.
func (sw *StubWatcher) DumpDeps() map[string][]string {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	result := make(map[string][]string)
	for pattern, dep := range sw.deps {
		files := make([]string, 0, len(dep.files))
		for f := range dep.files {
			rel, err := filepath.Rel(sw.configDir, f)
			if err != nil {
				rel = f
			}
			files = append(files, rel)
		}
		result[pattern] = files
	}
	return result
}
