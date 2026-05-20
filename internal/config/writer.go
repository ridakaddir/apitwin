package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// RouteKind identifies which top-level slice a route lives in.
type RouteKind string

const (
	KindHTTP RouteKind = "http"
	KindGRPC RouteKind = "grpc"
)

// FormatForPath maps a config file's extension to its serialization format
// ("toml" | "yaml" | "json"). Returns "" for unsupported extensions.
func FormatForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		return "toml"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	}
	return ""
}

// Marshal serializes a config to bytes in the requested format.
// format is one of "toml", "yaml", "json".
func Marshal(cfg *Config, format string) ([]byte, error) {
	switch strings.ToLower(format) {
	case "toml":
		var buf bytes.Buffer
		enc := toml.NewEncoder(&buf)
		enc.Indent = "  "
		if err := enc.Encode(cfg); err != nil {
			return nil, fmt.Errorf("marshal toml: %w", err)
		}
		return stripZeroIntsTOML(buf.Bytes()), nil
	case "yaml", "yml":
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(cfg); err != nil {
			return nil, fmt.Errorf("marshal yaml: %w", err)
		}
		_ = enc.Close()
		return buf.Bytes(), nil
	case "json":
		out, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal json: %w", err)
		}
		return append(out, '\n'), nil
	}
	return nil, fmt.Errorf("unsupported format %q (use toml, yaml, or json)", format)
}

// MarshalRoute serializes a single route wrapped in a minimal Config — used
// by the dev-tool preview pane so the user sees the exact bytes that would
// be written to disk for just this route.
func MarshalRoute(kind RouteKind, route any, format string) ([]byte, error) {
	cfg := &Config{}
	switch kind {
	case KindHTTP:
		r, ok := route.(Route)
		if !ok {
			if rp, isPtr := route.(*Route); isPtr && rp != nil {
				r = *rp
			} else {
				return nil, fmt.Errorf("kind=http but route is %T", route)
			}
		}
		cfg.Routes = []Route{r}
	case KindGRPC:
		r, ok := route.(GRPCRoute)
		if !ok {
			if rp, isPtr := route.(*GRPCRoute); isPtr && rp != nil {
				r = *rp
			} else {
				return nil, fmt.Errorf("kind=grpc but route is %T", route)
			}
		}
		cfg.GRPCRoutes = []GRPCRoute{r}
	default:
		return nil, fmt.Errorf("unknown route kind %q", kind)
	}
	return Marshal(cfg, format)
}

// WriteFileAtomic writes data to path via a sibling temp file and os.Rename
// so partial writes are never observable. The temp filename is prefixed with
// a dot so the loader's isConfigFile filter skips it.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// Selector identifies an existing route inside a config file for replacement
// or deletion. Method is ignored for gRPC routes.
type Selector struct {
	Kind   RouteKind
	Match  string
	Method string
}

func (s Selector) matchesHTTP(r Route) bool {
	return r.Match == s.Match && strings.EqualFold(r.Method, s.Method)
}

func (s Selector) matchesGRPC(r GRPCRoute) bool {
	return r.Match == s.Match
}

// ErrRouteNotFound is returned when a Selector identifies no existing route.
var ErrRouteNotFound = errors.New("route not found")

// ErrRouteConflict is returned when inserting a route whose (kind, match,
// method) already exists in the file. Use Selector-based replace instead.
var ErrRouteConflict = errors.New("route already exists in file")

// UpsertRouteInFile loads path, replaces the route identified by replace
// (if non-nil) or inserts when replace is nil, then atomically writes the
// file back in its native format. Creates an empty file when path doesn't
// exist yet, so callers can use this for both insert-into-new-file and
// edit-existing flows.
func UpsertRouteInFile(path string, kind RouteKind, route any, replace *Selector) error {
	format := FormatForPath(path)
	if format == "" {
		return fmt.Errorf("unsupported config file extension: %s", path)
	}

	cfg := &Config{}
	if _, err := os.Stat(path); err == nil {
		loaded, err := Load(path)
		if err != nil {
			return fmt.Errorf("loading existing config: %w", err)
		}
		cfg = loaded
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	switch kind {
	case KindHTTP:
		r, ok := asRoute(route)
		if !ok {
			return fmt.Errorf("kind=http but route is %T", route)
		}
		if replace != nil {
			found := false
			for i := range cfg.Routes {
				if replace.matchesHTTP(cfg.Routes[i]) {
					cfg.Routes[i] = r
					found = true
					break
				}
			}
			if !found {
				return ErrRouteNotFound
			}
		} else {
			conflict := Selector{Kind: KindHTTP, Match: r.Match, Method: r.Method}
			if slices.ContainsFunc(cfg.Routes, conflict.matchesHTTP) {
				return ErrRouteConflict
			}
			cfg.Routes = append(cfg.Routes, r)
		}
	case KindGRPC:
		r, ok := asGRPCRoute(route)
		if !ok {
			return fmt.Errorf("kind=grpc but route is %T", route)
		}
		if replace != nil {
			found := false
			for i := range cfg.GRPCRoutes {
				if replace.matchesGRPC(cfg.GRPCRoutes[i]) {
					cfg.GRPCRoutes[i] = r
					found = true
					break
				}
			}
			if !found {
				return ErrRouteNotFound
			}
		} else {
			conflict := Selector{Kind: KindGRPC, Match: r.Match}
			if slices.ContainsFunc(cfg.GRPCRoutes, conflict.matchesGRPC) {
				return ErrRouteConflict
			}
			cfg.GRPCRoutes = append(cfg.GRPCRoutes, r)
		}
	default:
		return fmt.Errorf("unknown route kind %q", kind)
	}

	data, err := Marshal(cfg, format)
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, data)
}

// DeleteRouteFromFile removes the route identified by sel and rewrites the
// file. Returns ErrRouteNotFound when the route isn't present.
func DeleteRouteFromFile(path string, sel Selector) error {
	format := FormatForPath(path)
	if format == "" {
		return fmt.Errorf("unsupported config file extension: %s", path)
	}
	cfg, err := Load(path)
	if err != nil {
		return err
	}

	switch sel.Kind {
	case KindHTTP:
		idx := -1
		for i := range cfg.Routes {
			if sel.matchesHTTP(cfg.Routes[i]) {
				idx = i
				break
			}
		}
		if idx == -1 {
			return ErrRouteNotFound
		}
		cfg.Routes = append(cfg.Routes[:idx], cfg.Routes[idx+1:]...)
	case KindGRPC:
		idx := -1
		for i := range cfg.GRPCRoutes {
			if sel.matchesGRPC(cfg.GRPCRoutes[i]) {
				idx = i
				break
			}
		}
		if idx == -1 {
			return ErrRouteNotFound
		}
		cfg.GRPCRoutes = append(cfg.GRPCRoutes[:idx], cfg.GRPCRoutes[idx+1:]...)
	default:
		return fmt.Errorf("unknown route kind %q", sel.Kind)
	}

	data, err := Marshal(cfg, format)
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, data)
}

// FindFileContainingRoute returns the path of the first config file under
// dir that contains a route matching sel. Returns "" with no error when no
// file contains it (caller should treat as "new route — pick a file").
func FindFileContainingRoute(dir string, sel Selector) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("reading dir %s: %w", dir, err)
	}
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

	for _, f := range files {
		cfg, err := Load(f)
		if err != nil {
			continue
		}
		switch sel.Kind {
		case KindHTTP:
			if slices.ContainsFunc(cfg.Routes, sel.matchesHTTP) {
				return f, nil
			}
		case KindGRPC:
			if slices.ContainsFunc(cfg.GRPCRoutes, sel.matchesGRPC) {
				return f, nil
			}
		}
	}
	return "", nil
}

// ListConfigFiles returns the config files in dir in deterministic order.
// When path is a single file, returns just that file.
func ListConfigFiles(path string, isDir bool) ([]string, error) {
	if !isDir {
		return []string{path}, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("reading dir %s: %w", path, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if isConfigFile(e.Name()) {
			files = append(files, filepath.Join(path, e.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

// stripZeroIntsTOML removes lines that emit zero-valued ints for fields where
// zero is the default and the loader treats omit-or-zero identically. The
// BurntSushi/toml encoder ignores `omitempty` for numeric types, so we
// post-process to keep the preview and the on-disk file clean.
func stripZeroIntsTOML(data []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(data))
	for _, line := range bytes.SplitAfter(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if bytes.Equal(trimmed, []byte("status = 0")) ||
			bytes.Equal(trimmed, []byte("delay = 0")) ||
			bytes.Equal(trimmed, []byte("duration = 0")) {
			continue
		}
		out.Write(line)
	}
	return out.Bytes()
}

func asRoute(v any) (Route, bool) {
	switch r := v.(type) {
	case Route:
		return r, true
	case *Route:
		if r == nil {
			return Route{}, false
		}
		return *r, true
	}
	return Route{}, false
}

func asGRPCRoute(v any) (GRPCRoute, bool) {
	switch r := v.(type) {
	case GRPCRoute:
		return r, true
	case *GRPCRoute:
		if r == nil {
			return GRPCRoute{}, false
		}
		return *r, true
	}
	return GRPCRoute{}, false
}
