package proxy

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/logger"
	apitwinruntime "github.com/ridakaddir/apitwin/internal/runtime"
)

// routeLoader is the subset of config.Loader used by recorder.
type routeLoader interface {
	AddRoute(route config.Route)
}

// recorder returns a responseRecorder that saves the proxied response body to a
// stub file, appends an enabled route stub to the config file, and immediately
// injects the route into the in-memory config so the next request is served
// from the stub without a restart.
//
// configPath is the seed config dir (recorded.toml and the canonical stub
// files live here — the dev commits them). stubRoot is the directory the
// running server actually reads stubs from: either the runtime mirror dir,
// or configPath when --no-runtime-dir is set. When they differ, each new
// stub is dual-written to both so the seed copy is committable and the
// runtime copy is what the in-memory route reads from during the current
// session.
func recorder(configPath, stubRoot string, loader routeLoader) responseRecorder {
	// seen tracks method+path pairs already recorded this session to avoid
	// appending duplicate routes on repeated requests to the same endpoint.
	seen := make(map[string]bool)

	return func(r *http.Request, status int, header http.Header, body []byte, latency time.Duration) {
		if len(body) == 0 {
			return
		}

		key := strings.ToUpper(r.Method) + " " + r.URL.Path
		if seen[key] {
			// Already recorded this session — stub file is up to date, skip.
			return
		}

		// Convert latency to integer seconds, capped at 5.
		// Sub-second responses become 0 (no artificial delay on fast APIs).
		delaySecs := int(latency.Seconds())
		if delaySecs > 5 {
			delaySecs = 5
		}

		// configPath is always a directory (loader.ConfigDir()).
		// stubs/ lives inside that directory.
		stubsDir := filepath.Join(configPath, "stubs")
		if err := os.MkdirAll(stubsDir, 0755); err != nil {
			logger.Error("record: creating stubs dir", "err", err)
			return
		}

		method := strings.ToLower(r.Method)
		slug := slugify(r.URL.Path)
		filename := fmt.Sprintf("%s_%s.json", method, slug)
		stubPath := filepath.Join(stubsDir, filename)

		// Decompress gzip-encoded responses before saving.
		if strings.EqualFold(header.Get("Content-Encoding"), "gzip") {
			if decompressed, err := gunzip(body); err == nil {
				body = decompressed
			}
		}

		// Pretty-print the body if it looks like JSON.
		ct := header.Get("Content-Type")
		if strings.Contains(ct, "application/json") {
			body = indentJSON(body)
		}

		if err := os.WriteFile(stubPath, body, 0644); err != nil {
			logger.Error("record: writing stub", "file", stubPath, "err", err)
			return
		}

		relStubPath := filepath.Join("stubs", filename)
		if err := appendRouteStub(configPath, r.Method, r.URL.Path, status, relStubPath, delaySecs); err != nil {
			logger.Error("record: appending route stub", "err", err)
			return
		}

		// Dual-write into the runtime mirror so the injected in-memory route
		// (which points at the runtime copy) serves the stub immediately.
		// When stubRoot == configPath (legacy --no-runtime-dir mode) this is
		// a no-op.
		runtimeStubPath := stubPath
		if stubRoot != configPath {
			runtimeStubPath = filepath.Join(stubRoot, "stubs", filename)
			if err := os.MkdirAll(filepath.Dir(runtimeStubPath), 0755); err != nil {
				logger.Error("record: creating runtime stubs dir", "err", err)
				// Fall back to seed path so the route is still injectable.
				runtimeStubPath = stubPath
			} else if err := os.WriteFile(runtimeStubPath, body, 0644); err != nil {
				logger.Error("record: writing runtime stub", "file", runtimeStubPath, "err", err)
				runtimeStubPath = stubPath
			}
		}

		// Mark as seen so we don't record it again this session.
		seen[key] = true

		// Use the runtime copy's absolute path for the in-memory route so
		// file reads work regardless of the process working directory and
		// so any future persist mutation on this route stays in the runtime
		// dir (never touching the committed seed).
		absStubPath := runtimeStubPath

		// Immediately inject the route into the in-memory config so the very
		// next request to this path is served from the stub (via=stub).
		enabled := true
		loader.AddRoute(config.Route{
			Method:   strings.ToUpper(r.Method),
			Match:    r.URL.Path,
			Enabled:  &enabled,
			Fallback: "recorded",
			Cases: map[string]config.Case{
				"recorded": {
					Status: status,
					File:   absStubPath,
					Delay:  delaySecs,
				},
			},
		})

		logger.Info("recorded", "method", r.Method, "path", r.URL.Path, "stub", relStubPath, "delay", fmt.Sprintf("%ds", delaySecs))
	}
}

// appendRouteStub appends an enabled route stub to the config file.
// When configPath is a directory, routes are written to <configPath>/recorded.toml.
func appendRouteStub(configPath, method, path string, status int, stubFile string, delaySecs int) error {
	target := configPath

	info, err := os.Stat(configPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		// Always use TOML for auto-generated recorded routes.
		target = filepath.Join(configPath, "recorded.toml")
	}

	ext := strings.ToLower(filepath.Ext(target))
	caseName := "recorded"
	stub := buildRouteStub(ext, method, path, caseName, status, stubFile, delaySecs)

	f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = f.WriteString("\n" + stub)
	return err
}

func buildRouteStub(ext, method, path, caseName string, status int, stubFile string, delaySecs int) string {
	switch ext {
	case ".yaml", ".yml":
		return fmt.Sprintf(`
  - method: %s
    match: %s
    enabled: true
    fallback: %s
    cases:
      %s:
        status: %d
        file: %s
        delay: %d
`, method, path, caseName, caseName, status, stubFile, delaySecs)

	case ".json":
		return fmt.Sprintf(`
// NOTE: add this to your "routes" array:
// {
//   "method": %q,
//   "match": %q,
//   "enabled": true,
//   "fallback": %q,
//   "cases": { %q: { "status": %d, "file": %q, "delay": %d } }
// }
`, method, path, caseName, caseName, status, stubFile, delaySecs)

	default: // .toml
		return fmt.Sprintf(`
[[routes]]
method   = %q
match    = %q
enabled  = true
fallback = %q

  [routes.cases.%s]
  status = %d
  file   = %q
  delay  = %d
`, method, path, caseName, caseName, status, stubFile, delaySecs)
	}
}

// initTemplate returns the content of a starter apitwin.toml file.
func initTemplate() string {
	return `# apitwin configuration
# Run: apitwin --target https://api.example.com

[[routes]]
method   = "GET"
match    = "/api/users"
enabled  = true
fallback = "success"

  [routes.cases.success]
  status = 200
  file   = "stubs/users.json"

  [routes.cases.empty]
  status = 200
  json   = '{"users": []}'

  [routes.cases.error]
  status = 500
  json   = '{"message": "Internal Server Error"}'
  delay  = 0

[[routes]]
method   = "POST"
match    = "/api/users"
enabled  = true
fallback = "created"

  [[routes.conditions]]
  source = "body"
  field  = "role"
  op     = "eq"
  value  = "admin"
  case   = "admin_created"

  [routes.cases.admin_created]
  status    = 201
  json      = '{"id": "{{uuid}}", "role": "admin", "created_at": "{{now}}"}'

  [routes.cases.created]
  status    = 201
  file      = "stubs/users.json"
  persist   = true
  merge     = "append"
  array_key = "users"

[[routes]]
method   = "PUT"
match    = "/api/users/*"
enabled  = true
fallback = "updated"

  [routes.cases.updated]
  status    = 200
  file      = "stubs/users.json"
  persist   = true
  merge     = "replace"
  key       = "id"
  array_key = "users"

[[routes]]
method   = "DELETE"
match    = "/api/users/*"
enabled  = true
fallback = "deleted"

  [routes.cases.deleted]
  status    = 204
  file      = "stubs/users.json"
  persist   = true
  merge     = "delete"
  key       = "id"
  array_key = "users"
`
}

// initStubTemplate returns a starter stubs/users.json file.
func initStubTemplate() string {
	return `{
  "users": [
    { "id": "1", "name": "Alice", "role": "user" },
    { "id": "2", "name": "Bob",   "role": "admin" }
  ]
}
`
}

// Init writes apitwin.toml and stubs/users.json to the given directory and
// adds the runtime state dir to the project's .gitignore so mutation traffic
// during a session doesn't leave modified stub files in git status.
func Init(dir string) error {
	if err := writeInitFiles(dir); err != nil {
		return err
	}
	if err := apitwinruntime.EnsureGitignore(dir); err != nil {
		// Non-fatal: scaffolding still succeeded.
		logger.Warn("init: could not update .gitignore", "err", err)
	}
	return nil
}

// writeInitFiles writes apitwin.toml and stubs/users.json to the given directory.
func writeInitFiles(dir string) error {
	configPath := filepath.Join(dir, "apitwin.toml")
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("apitwin.toml already exists — remove it first or edit it directly")
	}

	if err := os.WriteFile(configPath, []byte(initTemplate()), 0644); err != nil {
		return fmt.Errorf("writing apitwin.toml: %w", err)
	}

	stubsDir := filepath.Join(dir, "stubs")
	if err := os.MkdirAll(stubsDir, 0755); err != nil {
		return fmt.Errorf("creating stubs dir: %w", err)
	}

	stubPath := filepath.Join(stubsDir, "users.json")
	if _, err := os.Stat(stubPath); os.IsNotExist(err) {
		if err := os.WriteFile(stubPath, []byte(initStubTemplate()), 0644); err != nil {
			return fmt.Errorf("writing stubs/users.json: %w", err)
		}
	}

	return nil
}

// gunzip decompresses a gzip-encoded byte slice.
func gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(r)
}
