package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/logger"
	uifs "github.com/ridakaddir/apitwin/ui"
)

// ServerOptions holds all runtime configuration for the proxy server.
type ServerOptions struct {
	Target     string
	Port       int
	ConfigPath string
	ApiPrefix  string // stripped from request path before route matching and upstream forwarding
	RecordMode bool
}

// Server wraps the HTTP server and the config loader.
type Server struct {
	opts        ServerOptions
	loader      *config.Loader
	srv         *http.Server
	scheduler   *transitionScheduler
	stubWatcher *StubWatcher
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

	// transitions and scheduler are created here so we can reference them
	// in the onChange callback before the handler is fully constructed.
	ts := newTransitionState()
	sched := newTransitionScheduler(context.Background())

	loader, err := config.NewLoader(opts.ConfigPath, func(cfg *config.Config) {
		logger.Info("config reloaded", "routes", len(cfg.Routes))
		ts.Reset()
		sched.Reset(context.Background())
	})
	if err != nil {
		sched.Stop()
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// Start the stub watcher to automatically re-evaluate cross-references
	// when stub files change (e.g. deployment created → endpoint's
	// deployedModels updates immediately).
	stubWatcher := NewStubWatcher(StubWatcherOptions{
		ConfigDir: loader.ConfigDir(),
		OnUpdate: func() {
			logger.Info("stub watcher: cross-reference dependencies updated")
		},
	})

	handler := NewHandlerWithTransitions(loader, rp, opts.RecordMode, opts.ApiPrefix, ts, sched, stubWatcher)

	mux := http.NewServeMux()
	mux.HandleFunc("/__api/routes", apiRoutesHandler(loader))
	mux.Handle("/__ui/", uiHandler())
	mux.Handle("/", handler)

	chain := corsMiddleware(logger.Middleware(mux))

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", opts.Port),
		Handler:      chain,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{opts: opts, loader: loader, srv: srv, scheduler: sched, stubWatcher: stubWatcher}, nil
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
		return nil
	case err := <-errCh:
		s.stubWatcher.Stop()
		s.scheduler.Stop()
		s.loader.Close()
		return err
	}
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
		// Try to serve the requested file; fall back to index.html for SPA routes.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(distFS, path); err != nil {
			r.URL.Path = "/index.html"
		}
		fileServer.ServeHTTP(w, r)
	}))
}

// corsMiddleware injects CORS headers on every response and handles preflight.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
