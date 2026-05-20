package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// Loader holds the current merged config and watches for changes.
type Loader struct {
	mu          sync.RWMutex
	cfg         *Config
	path        string // file or directory
	isDir       bool
	stubRoot    string // when non-empty, stub reads/writes resolve here instead of ConfigDir()
	watcher     *fsnotify.Watcher
	onChangeFns []func(*Config)
}

// NewLoader loads config from path (file or directory) and starts a file watcher.
// onChange is called with the merged config whenever any config file changes.
func NewLoader(path string, onChange func(*Config)) (*Loader, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("config path %q not found: %w", path, err)
	}

	isDir := info.IsDir()

	cfg, err := loadPath(path, isDir)
	if err != nil {
		return nil, err
	}

	l := &Loader{
		cfg:   cfg,
		path:  path,
		isDir: isDir,
	}
	if onChange != nil {
		l.onChangeFns = append(l.onChangeFns, onChange)
	}

	if err := l.watch(); err != nil {
		return nil, err
	}

	return l, nil
}

// Get returns the current merged config (safe for concurrent reads).
func (l *Loader) Get() *Config {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.cfg
}

// AddRoute appends a route to the in-memory config immediately, without waiting
// for the file watcher. Used by record mode so the next request is served from
// the stub without a restart.
func (l *Loader) AddRoute(route Route) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cfgCopy := *l.cfg
	cfgCopy.Routes = append(cfgCopy.Routes, route)
	l.cfg = &cfgCopy
}

// Close stops the file watcher.
func (l *Loader) Close() {
	if l.watcher != nil {
		_ = l.watcher.Close()
	}
}

// AddOnChange registers an additional callback invoked whenever any config
// file changes. Callbacks are called in the order they are registered.
// Safe to call from any goroutine.
func (l *Loader) AddOnChange(fn func(*Config)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onChangeFns = append(l.onChangeFns, fn)
}

// Path returns the original --config path (file or directory) the loader was
// constructed with. The dev-tool config-builder uses this to answer
// /__api/config/files.
func (l *Loader) Path() string {
	return l.path
}

// IsDir reports whether the loader was given a directory of config files.
func (l *Loader) IsDir() bool {
	return l.isDir
}

// ConfigDir returns the directory that contains the config file(s).
// Use this when you need the seed directory (e.g. for writing recorded routes
// back to the config file). For stub file I/O, use StubRoot() instead so
// runtime mutations land in the gitignored runtime state directory.
func (l *Loader) ConfigDir() string {
	if l.isDir {
		return l.path
	}
	return filepath.Dir(l.path)
}

// StubRoot returns the directory used as the base for resolving relative stub
// paths at request time. When the server has set a runtime state directory
// (via SetStubRoot), that is returned; otherwise it falls back to ConfigDir()
// so legacy (--no-runtime-dir) mode and pre-bootstrap callers keep working.
func (l *Loader) StubRoot() string {
	l.mu.RLock()
	root := l.stubRoot
	l.mu.RUnlock()
	if root != "" {
		return root
	}
	return l.ConfigDir()
}

// SetStubRoot configures the runtime stub root. The proxy server calls this
// during NewServer bootstrap after mirroring the seed tree into the runtime
// directory. Passing an empty string resets to "seed is the stub root"
// behavior.
func (l *Loader) SetStubRoot(path string) {
	l.mu.Lock()
	l.stubRoot = path
	l.mu.Unlock()
}

// watch starts fsnotify on the config directory (whether path is a file or dir).
func (l *Loader) watch() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}
	l.watcher = w

	// Always watch the directory — catches atomic saves (rename) and new files.
	watchDir := l.path
	if !l.isDir {
		watchDir = filepath.Dir(l.path)
	}
	if err := w.Add(watchDir); err != nil {
		return fmt.Errorf("watching %s: %w", watchDir, err)
	}

	go func() {
		for {
			select {
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if !isConfigFile(event.Name) {
					continue
				}
				// For file mode, only react to the specific config file.
				if !l.isDir && filepath.Clean(event.Name) != filepath.Clean(l.path) {
					continue
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
					cfg, err := loadPath(l.path, l.isDir)
					if err != nil {
						continue
					}
					l.mu.Lock()
					l.cfg = cfg
					fns := make([]func(*Config), len(l.onChangeFns))
					copy(fns, l.onChangeFns)
					l.mu.Unlock()
					for _, fn := range fns {
						fn(cfg)
					}
				}
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return nil
}

// loadPath loads and merges config from a file or directory.
func loadPath(path string, isDir bool) (*Config, error) {
	if isDir {
		return loadDir(path)
	}
	return Load(path)
}

// loadDir reads all config files in dir, merges their routes in filename order.
func loadDir(dir string) (*Config, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading config dir %s: %w", dir, err)
	}

	// Collect and sort config files so load order is deterministic.
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if isConfigFile(e.Name()) {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)

	merged := &Config{}
	for _, f := range files {
		cfg, err := Load(f)
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", f, err)
		}
		merged.Routes = append(merged.Routes, cfg.Routes...)
		merged.GRPCRoutes = append(merged.GRPCRoutes, cfg.GRPCRoutes...)
	}

	return merged, nil
}

// Load parses a single config file, auto-detecting format from extension.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))

	var cfg Config
	switch ext {
	case ".toml":
		if err := toml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing TOML %s: %w", path, err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing YAML %s: %w", path, err)
		}
	case ".json":
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing JSON %s: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("unsupported config format %q (use .toml, .yaml, .yml, or .json)", ext)
	}

	return &cfg, nil
}

// isConfigFile returns true if the filename has a supported config extension.
func isConfigFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".toml", ".yaml", ".yml", ".json":
		return true
	}
	return false
}
