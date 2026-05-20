package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/logger"
	apitwinruntime "github.com/ridakaddir/apitwin/internal/runtime"
	runtimepersist "github.com/ridakaddir/apitwin/internal/runtime/persist"
	"github.com/ridakaddir/apitwin/internal/transitions"
	"github.com/ridakaddir/apitwin/internal/validation"
	uifs "github.com/ridakaddir/apitwin/ui"
)

// maxGRPCInvokeBodyBytes caps the /__api/grpc/invoke request body so a
// malicious or misbehaving caller cannot exhaust memory by posting a huge
// JSON blob that would be buffered via json.RawMessage. Protobuf request
// messages in practice are well under this; 4 MiB matches the default
// gRPC max-recv size.
const maxGRPCInvokeBodyBytes = 4 << 20

// ServerOptions holds all runtime configuration for the proxy server.
type ServerOptions struct {
	Target     string
	Port       int
	ConfigPath string
	ApiPrefix  string // stripped from request path before route matching and upstream forwarding
	RecordMode bool

	// MaskPII gates the PII-masking pass that runs over recorded
	// responses before they are written to disk. Defaults on at the CLI
	// layer; opt out with --no-mask-pii. No effect when RecordMode is
	// false. The live response sent to the client is never masked.
	MaskPII bool

	// NoRuntimeDir disables the runtime state mirror and writes mutations
	// back to the seed stub files (legacy behavior). Dirties git on every
	// mutation — only set when reproducing the pre-runtime-dir experience.
	NoRuntimeDir bool

	// Ephemeral puts the runtime state directory in a tempdir that is
	// wiped on shutdown. Useful for CI and one-shot demos. Mutually
	// exclusive with NoRuntimeDir (ephemeral wins if both are set).
	Ephemeral bool

	// ResetRuntime wipes the runtime state directory before startup and
	// re-mirrors from seed. Runtime-only stubs (created via POST) and the
	// persisted transition / schedule metadata are cleared. No effect in
	// --no-runtime-dir or --ephemeral modes.
	ResetRuntime bool
}

// GRPCInvoker is the narrow surface the devtool HTTP endpoints need from the
// gRPC server. Implemented by *grpc.Invoker. The methods return `any` so
// internal/proxy stays decoupled from internal/grpc — the handlers simply
// pass the values through encoding/json.
type GRPCInvoker interface {
	Schema() any
	Invoke(ctx context.Context, fullMethod string, metadata map[string]string, reqJSON []byte) (any, error)
}

// Server wraps the HTTP server and the config loader.
type Server struct {
	opts        ServerOptions
	loader      *config.Loader
	srv         *http.Server
	scheduler   *transitionScheduler
	stubWatcher *StubWatcher

	// runtimeDir is the mirror directory stub I/O is redirected to. Empty
	// in legacy (NoRuntimeDir) mode.
	runtimeDir string
	// ephemeralDir is set when Ephemeral is true; removed on shutdown.
	ephemeralDir string

	// grpcInvoker is set lazily after NewServer when the gRPC server is
	// started (see SetGRPCInvoker). When unset, /__api/grpc/* return 503.
	// atomic.Pointer guarantees safe publication across goroutines: the
	// HTTP handlers read this from request goroutines while cmd/root.go
	// writes it from the startup goroutine.
	grpcInvoker atomic.Pointer[GRPCInvoker]
}

// NewServer initialises the proxy server.
func NewServer(opts ServerOptions) (*Server, error) {
	var rp *httputil.ReverseProxy
	if opts.Target != "" {
		var rpErr error
		rp, rpErr = newReverseProxy(opts.Target)
		if rpErr != nil {
			return nil, fmt.Errorf("creating reverse proxy: %w", rpErr)
		}
	}

	// Load config first so we know ConfigDir() for the runtime-dir
	// bootstrap. The onChange callback is attached after ts/sched are
	// constructed below — that way the callback can close over them.
	loader, err := config.NewLoader(opts.ConfigPath, nil)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// Fail-fast structural validation of REST routes. Catches config
	// errors that would otherwise silently corrupt persisted stubs at
	// runtime (e.g. a transition case with persist+merge=update but no
	// file). Wrap/source enforcement on REST is handled by the runtime
	// safeguard in applyPersist — there's no schema here to know what
	// "valid wrap" means.
	if cfg := loader.Get(); cfg != nil {
		if err := config.AsError(config.ValidateRESTRoutes(cfg.Routes)); err != nil {
			loader.Close()
			return nil, err
		}
		if err := validation.ValidateConfig(cfg); err != nil {
			loader.Close()
			return nil, fmt.Errorf("validation rules: %w", err)
		}
	}

	// Runtime state directory bootstrap.
	//
	// Unless --no-runtime-dir is passed, mirror the seed tree into either
	// .apitwin/state/ (default) or a tempdir (--ephemeral) and redirect all
	// stub I/O there. The default behavior is seed-overlay: seed files are
	// copied on top of any runtime copies (seed wins), but runtime-only
	// files created via POST are preserved across restarts. Use
	// ResetRuntime for a full wipe.
	var (
		runtimeDir   string
		ephemeralDir string
	)
	switch {
	case opts.NoRuntimeDir:
		logger.Warn("runtime state dir disabled (--no-runtime-dir): mutations will write back to seed stubs and dirty git")
	case opts.Ephemeral:
		edir, eErr := apitwinruntime.Ephemeral()
		if eErr != nil {
			loader.Close()
			return nil, fmt.Errorf("creating ephemeral runtime dir: %w", eErr)
		}
		if mErr := apitwinruntime.MirrorInto(loader.ConfigDir(), edir); mErr != nil {
			_ = apitwinruntime.Cleanup(edir)
			loader.Close()
			return nil, fmt.Errorf("mirroring into ephemeral runtime dir: %w", mErr)
		}
		runtimeDir = edir
		ephemeralDir = edir
		logger.Info("runtime state dir (ephemeral)", "dir", edir)
	default:
		if opts.ResetRuntime {
			if err := apitwinruntime.Cleanup(apitwinruntime.DefaultPath(loader.ConfigDir())); err != nil {
				loader.Close()
				return nil, fmt.Errorf("reset-runtime cleanup: %w", err)
			}
			logger.Info("runtime state reset (--reset-runtime): wiped runtime dir and persisted metadata")
		}
		rdir, initialised, mErr := apitwinruntime.OverlaySeed(loader.ConfigDir())
		if mErr != nil {
			loader.Close()
			return nil, fmt.Errorf("overlaying seed into runtime state dir: %w", mErr)
		}
		runtimeDir = rdir
		if initialised {
			logger.Info("runtime state dir initialised from seed", "dir", rdir)
		} else {
			logger.Info("runtime state dir refreshed from seed (runtime-only stubs preserved)", "dir", rdir)
		}
		if eiErr := apitwinruntime.EnsureGitignore(loader.ConfigDir()); eiErr != nil {
			logger.Warn("could not update .gitignore", "err", eiErr)
		}
		if !apitwinruntime.CheckGitignore(loader.ConfigDir()) {
			logger.Warn("runtime state dir is not gitignored — add '.apitwin/state/' to your .gitignore")
		}
	}
	loader.SetStubRoot(runtimeDir)

	// Persistence wiring: only when we have a non-empty runtime dir and
	// it's not ephemeral. Legacy mode (NoRuntimeDir) and Ephemeral mode
	// get plain in-memory ts/sched.
	var (
		ts              *transitionState
		sched           *transitionScheduler
		schedPersister  *runtimepersist.ScheduledPersister
		pendingForRest  []runtimepersist.ScheduledItem
	)
	if runtimeDir != "" && !opts.Ephemeral {
		metaDir := apitwinruntime.MetaDir(runtimeDir)
		if err := os.MkdirAll(metaDir, 0755); err != nil {
			loader.Close()
			return nil, fmt.Errorf("creating runtime meta dir %q: %w", metaDir, err)
		}
		transitionsPath := filepath.Join(metaDir, "transitions.json")
		scheduledPath := filepath.Join(metaDir, "scheduled.json")

		tfile, tErr := runtimepersist.LoadTransitions(transitionsPath)
		if tErr != nil {
			logger.Warn("transitions load failed; continuing with empty state", "err", tErr)
			tfile = runtimepersist.EmptyTransitions()
		}
		tPersister := newTransitionsDiskPersister(transitionsPath)
		ts = transitions.NewPersistentState[*config.Route](
			routeKey, "rest", tPersister, tfile.Scope("rest"),
		)

		schedPersister = runtimepersist.NewScheduledPersister(scheduledPath, runtimeDir)
		sfile, sErr := schedPersister.Load()
		if sErr != nil {
			logger.Warn("scheduled load failed; continuing with empty state", "err", sErr)
			sfile = runtimepersist.EmptyScheduled()
		}
		pendingForRest = sfile.ItemsForScope("rest")

		sched = newTransitionScheduler(context.Background())
		sched.SetPersister(schedPersister)
	} else {
		ts = newTransitionState()
		sched = newTransitionScheduler(context.Background())
	}

	// Attach the hot-reload callback now that ts/sched exist.
	loader.AddOnChange(func(cfg *config.Config) {
		logger.Info("config reloaded", "routes", len(cfg.Routes))
		ts.Reset()
		sched.Reset(context.Background())
	})

	// Start the stub watcher to automatically re-evaluate cross-references
	// when stub files change (e.g. deployment created → endpoint's
	// deployedModels updates immediately). Watches the runtime dir so it
	// sees the live, mutating copies — not the frozen seed files.
	stubWatcher := NewStubWatcher(StubWatcherOptions{
		ConfigDir: loader.StubRoot(),
		OnUpdate: func() {
			logger.Info("stub watcher: cross-reference dependencies updated")
		},
	})

	handler := NewHandlerWithOptions(HandlerOptions{
		Loader:      loader,
		Proxy:       rp,
		RecordMode:  opts.RecordMode,
		MaskPII:     opts.MaskPII,
		ApiPrefix:   opts.ApiPrefix,
		Transitions: ts,
		Scheduler:   sched,
		StubWatcher: stubWatcher,
	})

	s := &Server{
		opts:         opts,
		loader:       loader,
		scheduler:    sched,
		stubWatcher:  stubWatcher,
		runtimeDir:   runtimeDir,
		ephemeralDir: ephemeralDir,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/__api/routes", apiRoutesHandler(loader))
	mux.HandleFunc("/__api/grpc/methods", s.apiGRPCMethodsHandler)
	mux.HandleFunc("/__api/grpc/invoke", s.apiGRPCInvokeHandler)
	mux.Handle("/__ui/", uiHandler())
	mux.Handle("/", handler)

	chain := corsMiddleware(logger.Middleware(mux))

	s.srv = &http.Server{
		Addr:         fmt.Sprintf(":%d", opts.Port),
		Handler:      chain,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Re-arm any pending deferred mutations persisted by a previous
	// session. Past-due items fire synchronously; future items re-schedule
	// with the remaining delay.
	if len(pendingForRest) > 0 {
		logger.Info("rearming persisted transitions", "count", len(pendingForRest))
		sched.Rearm(pendingForRest)
	}

	return s, nil
}

// SetGRPCInvoker wires a gRPC invoker so the devtool endpoints can dispatch
// JSON-shaped requests into the gRPC server. Safe to call at any time; the
// atomic pointer guarantees handlers either see the old nil state (503) or
// the new invoker, never a partially-initialised one.
func (s *Server) SetGRPCInvoker(inv GRPCInvoker) {
	s.grpcInvoker.Store(&inv)
}

// loadGRPCInvoker returns the current invoker or nil if none is wired.
func (s *Server) loadGRPCInvoker() GRPCInvoker {
	p := s.grpcInvoker.Load()
	if p == nil {
		return nil
	}
	return *p
}

// Loader returns the config loader, allowing other servers (e.g. gRPC) to
// share the same loaded config and config directory.
func (s *Server) Loader() *config.Loader {
	return s.loader
}

// Start listens and serves. It blocks until the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		prefixMsg := s.opts.ApiPrefix
		if prefixMsg == "" {
			prefixMsg = "(none)"
		}
		logger.Info("apitwin running",
			"port", s.opts.Port,
			"config", s.opts.ConfigPath,
			"routes", len(s.loader.Get().Routes),
			"target", s.opts.Target,
			"prefix", prefixMsg,
			"record", s.opts.RecordMode,
			"ui", fmt.Sprintf("http://localhost:%d/__ui/", s.opts.Port),
		)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutCtx)
		s.stubWatcher.Stop()
		s.scheduler.Stop()
		s.loader.Close()
		s.cleanupEphemeral()
		return nil
	case err := <-errCh:
		s.stubWatcher.Stop()
		s.scheduler.Stop()
		s.loader.Close()
		s.cleanupEphemeral()
		return err
	}
}

// cleanupEphemeral removes the tempdir backing the ephemeral runtime state,
// if one was created. No-op for non-ephemeral runs.
func (s *Server) cleanupEphemeral() {
	if s.ephemeralDir == "" {
		return
	}
	if err := apitwinruntime.Cleanup(s.ephemeralDir); err != nil {
		logger.Warn("cleaning up ephemeral runtime dir", "dir", s.ephemeralDir, "err", err)
	}
}

// RuntimeDir exposes the runtime state directory (if one is in use) so other
// subsystems (e.g. the gRPC server, tests) can observe it.
func (s *Server) RuntimeDir() string {
	return s.runtimeDir
}

// MetaDir returns the persisted-metadata directory inside the runtime dir
// (empty in legacy / ephemeral mode where persistence is disabled). Used
// by cmd/root.go to hand the same metadata location to the gRPC server so
// REST and gRPC share one transitions.json / scheduled.json.
func (s *Server) MetaDir() string {
	if s.runtimeDir == "" || s.ephemeralDir != "" {
		return ""
	}
	return apitwinruntime.MetaDir(s.runtimeDir)
}

// apiGRPCMethodsHandler returns the loaded gRPC method schemas for the UI.
// Returns 503 when apitwin is running without --grpc-proto so the UI can
// hide or disable the gRPC tester.
func (s *Server) apiGRPCMethodsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	inv := s.loadGRPCInvoker()
	if inv == nil {
		http.Error(w, "grpc not enabled (start apitwin with --grpc-proto)", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(inv.Schema())
}

// apiGRPCInvokeHandler dispatches a JSON-shaped unary call into the gRPC
// server on behalf of the browser UI. Request/response bodies are
// transcoded via the shared proto registry.
func (s *Server) apiGRPCInvokeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	inv := s.loadGRPCInvoker()
	if inv == nil {
		http.Error(w, "grpc not enabled (start apitwin with --grpc-proto)", http.StatusServiceUnavailable)
		return
	}

	// Cap the request body so a pathological caller can't buffer a huge
	// JSON blob into memory via the RawMessage copy below.
	r.Body = http.MaxBytesReader(w, r.Body, maxGRPCInvokeBodyBytes)

	var req struct {
		Method   string            `json:"method"`
		Message  json.RawMessage   `json:"message"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Method == "" {
		http.Error(w, "missing `method`", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	result, err := inv.Invoke(ctx, req.Method, req.Metadata, []byte(req.Message))
	if err != nil {
		// Transcoding / configuration errors — not a gRPC status. Surface
		// as 400 so the UI can show a meaningful message rather than a
		// mysterious 500.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// apiRoutesHandler returns the current config as JSON (GET only).
func apiRoutesHandler(loader *config.Loader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cfg := loader.Get()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfg)
	}
}

// uiHandler serves the embedded React SPA with index.html fallback for client-side routing.
func uiHandler() http.Handler {
	distFS, err := fs.Sub(uifs.DistFS, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "UI assets not available", http.StatusInternalServerError)
		})
	}
	fileServer := http.FileServer(http.FS(distFS))

	return http.StripPrefix("/__ui", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			serveUIIndex(w, distFS)
			return
		}
		if _, err := fs.Stat(distFS, p); err != nil {
			// A missing asset (something with an extension) must return 404, not
			// fall back to index.html. Mutating r.URL.Path to "/index.html" would
			// trigger http.FileServer's built-in redirect of ".../index.html" →
			// "./", which breaks the page with a spurious 301 and can loop when
			// browsers have cached an older HTML referencing stale asset hashes.
			if strings.Contains(path.Base(p), ".") {
				http.NotFound(w, r)
				return
			}
			serveUIIndex(w, distFS)
			return
		}
		fileServer.ServeHTTP(w, r)
	}))
}

func serveUIIndex(w http.ResponseWriter, distFS fs.FS) {
	data, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

// corsMiddleware injects CORS headers on mock responses and handles
// preflight. The devtool endpoints under /__api/ are deliberately excluded:
// they dispatch into the local mock and gRPC servers and must only be
// reachable from the same-origin devtool UI, not from arbitrary pages the
// user happens to visit (which would otherwise be able to drive the local
// gRPC invoker via a cross-origin POST).
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/__api/") || strings.HasPrefix(r.URL.Path, "/__ui/") {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
