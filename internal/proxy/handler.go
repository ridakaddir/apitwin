package proxy

import (
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/ridakaddir/apitwin/internal/logger"
)

// Handler holds runtime state for the request dispatcher.
type Handler struct {
	loader      configLoader
	rp          *httputil.ReverseProxy // nil if no --target
	rec         responseRecorder       // non-nil in record mode, created once
	apiPrefix   string                 // stripped from request path before matching + forwarding
	transitions *transitionState       // time-based transition state
	scheduler   *transitionScheduler   // deferred file mutations after persist append
	stubWatcher *StubWatcher           // tracks cross-reference dependencies between stub files
}

// configLoader abstracts config.Loader so it can be mocked in tests.
//
// ConfigDir() returns the seed directory (where the dev's committed stubs
// live). StubRoot() returns the base path that stub I/O should resolve
// against — either the runtime mirror directory or, in legacy mode, the
// seed directory. Always use StubRoot() for request-time reads/writes and
// ConfigDir() only for operations that must target the seed (e.g. record
// mode writing routes into the user's config file).
type configLoader interface {
	Get() *config.Config
	AddRoute(route config.Route)
	ConfigDir() string
	StubRoot() string
}

// NewHandler builds a new Handler with a fresh transition state.
// NOTE: The handler created this way has no scheduler or stub watcher.
// Use NewHandlerWithTransitions when background transition mutations are needed.
//
// PII masking defaults on for record mode so test/scaffold callers don't
// have to think about the safer default; production callers go through
// NewHandlerWithOptions.
func NewHandler(loader configLoader, rp *httputil.ReverseProxy, recordMode bool, apiPrefix string) *Handler {
	return NewHandlerWithOptions(HandlerOptions{
		Loader:     loader,
		Proxy:      rp,
		RecordMode: recordMode,
		MaskPII:    true,
		ApiPrefix:  apiPrefix,
	})
}

// NewHandlerWithTransitions builds a Handler with the provided transition, scheduler, and stub watcher state.
// Used by NewServer to share the transition state with the config reload callback.
//
// PII masking defaults on. Use NewHandlerWithOptions to override.
func NewHandlerWithTransitions(loader configLoader, rp *httputil.ReverseProxy, recordMode bool, apiPrefix string, ts *transitionState, sched *transitionScheduler, sw *StubWatcher) *Handler {
	return NewHandlerWithOptions(HandlerOptions{
		Loader:      loader,
		Proxy:       rp,
		RecordMode:  recordMode,
		MaskPII:     true,
		ApiPrefix:   apiPrefix,
		Transitions: ts,
		Scheduler:   sched,
		StubWatcher: sw,
	})
}

// HandlerOptions bundles every knob NewHandlerWithOptions accepts. It
// exists so we can keep adding optional behaviour (PII masking, future
// per-route transforms, ...) without growing the positional argument
// list every time.
type HandlerOptions struct {
	Loader      configLoader
	Proxy       *httputil.ReverseProxy
	RecordMode  bool
	MaskPII     bool
	ApiPrefix   string
	Transitions *transitionState
	Scheduler   *transitionScheduler
	StubWatcher *StubWatcher
}

// NewHandlerWithOptions is the canonical constructor. Older entry points
// (NewHandler, NewHandlerWithTransitions) delegate here.
func NewHandlerWithOptions(opts HandlerOptions) *Handler {
	apiPrefix := opts.ApiPrefix
	// Normalise prefix: ensure it starts with / and has no trailing slash.
	if apiPrefix != "" && !strings.HasPrefix(apiPrefix, "/") {
		apiPrefix = "/" + apiPrefix
	}
	apiPrefix = strings.TrimRight(apiPrefix, "/")

	ts := opts.Transitions
	if ts == nil {
		ts = newTransitionState()
	}

	// Create the recorder once so its internal `seen` dedup map persists
	// across all requests for the lifetime of the server. Recorded stubs
	// are dual-written: the canonical copy lands in the seed dir (so the
	// dev can commit them) and an immediate working copy lands in the
	// runtime mirror (so the just-injected route reads from a path that
	// participates in StubRoot semantics).
	var rec responseRecorder
	if opts.RecordMode {
		rec = recorder(opts.Loader.ConfigDir(), opts.Loader.StubRoot(), opts.Loader, opts.MaskPII)
	}

	return &Handler{
		loader:      opts.Loader,
		rp:          opts.Proxy,
		rec:         rec,
		apiPrefix:   apiPrefix,
		transitions: ts,
		scheduler:   opts.Scheduler,
		stubWatcher: opts.StubWatcher,
	}
}

// ServeHTTP is the main dispatcher.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cfg := h.loader.Get()
	bodyBytes := readBody(r)

	// Strip the api prefix from the path used for matching and forwarding.
	// The original r.URL.Path is preserved; matchPath and upstream see the stripped path.
	matchPath_ := r.URL.Path
	if h.apiPrefix != "" {
		if strings.HasPrefix(r.URL.Path, h.apiPrefix) {
			matchPath_ = r.URL.Path[len(h.apiPrefix):]
			if matchPath_ == "" {
				matchPath_ = "/"
			}
		}
	}

	for i := range cfg.Routes {
		route := &cfg.Routes[i]

		if !route.IsEnabled() {
			continue
		}
		if !strings.EqualFold(route.Method, r.Method) {
			continue
		}
		// Check if route matches and extract any named parameters
		matched, pathParams := matchWithNamedParams(route.Match, matchPath_)
		if !matched {
			continue
		}

		// Route matched — resolve case.
		caseName := h.resolveCase(route, r, bodyBytes, route.Match, pathParams)
		if caseName == "" {
			// No condition matched and no fallback — fall through to proxy.
			break
		}

		c, ok := config.ResolveCase(route.Cases, caseName, route.Fallback)
		if !ok {
			logger.Warn("case not found in route", "case", caseName, "route", route.Match)
			break
		}

		// Clone request with stripped path for consistent path-based extractions
		requestForExtraction := r
		if h.apiPrefix != "" && strings.HasPrefix(r.URL.Path, h.apiPrefix) {
			requestForExtraction = r.Clone(r.Context())
			requestForExtraction.URL.Path = matchPath_
			if requestForExtraction.URL.RawPath != "" {
				requestForExtraction.URL.RawPath = matchPath_
			}
		}

		// Persist (mutating methods).
		if c.Persist {
			// When a transition resolved this case to an "update" merge on
			// a file that doesn't exist (e.g. a new resource hitting a
			// terminal transition state meant for a different resource),
			// buffer the response so we can detect the 404 and retry with
			// the fallback case instead of returning an error to the client.
			isTransitionCase := caseName != route.Fallback && len(route.Transitions) > 0
			shouldBuffer := isTransitionCase && strings.EqualFold(c.Merge, "update")

			var (
				target = http.ResponseWriter(w)
				buf    *responseBuffer
			)
			if shouldBuffer {
				buf = newIsolatedBuffer()
				target = buf
			}

			handled, persistedPath := applyPersist(target, requestForExtraction, c, bodyBytes, route.Match, h.loader.StubRoot(), pathParams)
			if handled {
				// If the buffered response is a 404 (file not found for
				// update), fall through to the fallback case. This handles
				// the scenario where the transition state is per-route-pattern
				// (shared across all resource IDs) and resolves to a terminal
				// "update" case for a resource whose file doesn't exist yet.
				if buf != nil && buf.status == http.StatusNotFound {
					fallbackCase, ok := config.ResolveCase(route.Cases, route.Fallback, route.Fallback)
					if ok && fallbackCase.Persist {
						handled2, persistedPath2 := applyPersist(w, requestForExtraction, fallbackCase, bodyBytes, route.Match, h.loader.StubRoot(), pathParams)
						if handled2 {
							if persistedPath2 != "" {
								if strings.EqualFold(fallbackCase.Merge, "append") && h.stubWatcher != nil {
									h.stubWatcher.AddFile(persistedPath2)
								}
								if len(route.Transitions) > 0 && h.scheduler != nil {
									refCtx := NewRefContext(requestForExtraction, bodyBytes, pathParams)
									h.scheduler.Schedule(route, persistedPath2, h.loader.StubRoot(), refCtx)
								}
							}
							if strings.EqualFold(fallbackCase.Merge, "delete") {
								h.transitions.ResetMatch(route.Match)
							}
							return
						}
					}
					// If fallback also fails or doesn't exist, flush the original 404.
					buf.flush(w)
					return
				}

				// Flush buffered response if it was not a 404.
				if buf != nil {
					buf.flush(w)
					// The isolated buffer couldn't propagate logger.SetSource
					// to the real writer, so mark it now after flushing.
					logger.SetSource(w, logger.SourceStub)
				}

				if persistedPath != "" {
					// Register newly created files (append only) with the stub
					// watcher so that any cross-references they contain are
					// tracked for automatic hot-reload.
					if strings.EqualFold(c.Merge, "append") && h.stubWatcher != nil {
						h.stubWatcher.AddFile(persistedPath)
					}

					// If this route has transitions, schedule deferred background
					// mutations so the persisted file transitions on disk over
					// time. This applies to both append (resource creation) and
					// update (resource mutation) operations.
					if len(route.Transitions) > 0 && h.scheduler != nil {
						refCtx := NewRefContext(requestForExtraction, bodyBytes, pathParams)
						h.scheduler.Schedule(route, persistedPath, h.loader.StubRoot(), refCtx)
					}
				}

				// When a DELETE removes a resource, reset transition state for
				// all routes sharing the same match pattern. Without this, a
				// subsequent POST would resolve to the terminal transition case
				// (e.g. "ready" with merge=update) instead of the fallback
				// "created" case (merge=append), causing a 404 because the
				// file no longer exists.
				if strings.EqualFold(c.Merge, "delete") {
					h.transitions.ResetMatch(route.Match)
				}

				return
			}
		}

		// Serve mock response into a buffer so we can inspect it first.
		rec := newResponseRecorder(w)
		serveMock(rec, requestForExtraction, c, bodyBytes, h.loader.StubRoot(), route.Match, pathParams)

		// If the dynamic file was missing, try next condition / fallback / proxy.
		if isFileMissing(rec) {
			continue
		}

		// Mark the response as coming from a local stub before flushing.
		logger.SetSource(w, logger.SourceStub)

		// Flush the captured response to the real writer.
		clearFileMissing(rec)
		rec.flush(w)
		return
	}

	// No route matched — proxy to upstream.
	if h.rp == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "no matching route and no --target configured",
		})
		return
	}

	// Rewrite the request path to strip the api prefix before forwarding.
	if h.apiPrefix != "" && strings.HasPrefix(r.URL.Path, h.apiPrefix) {
		stripped := r.URL.Path[len(h.apiPrefix):]
		if stripped == "" {
			stripped = "/"
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = stripped
		if r2.URL.RawPath != "" {
			r2.URL.RawPath = stripped
		}
		r = r2
	}

	forwardRequest(w, r, h.rp, bodyBytes, h.rec)
}

// resolveCase determines which case to serve for a matched route.
// Priority order:
//  1. Conditions — first matching condition wins
//  2. Transitions — time-based sequence (if defined and no condition matched)
//  3. Fallback
func (h *Handler) resolveCase(route *config.Route, r *http.Request, bodyBytes []byte, routePattern string, pathParams map[string]string) string {
	// 1. Conditions take priority.
	for _, cond := range route.Conditions {
		if evalCondition(cond, r, bodyBytes, routePattern, pathParams) {
			return cond.Case
		}
	}

	// 2. Transitions — resolve by elapsed time since first request.
	// Only use the transition result if the case actually exists in the route;
	// routes that define transitions purely for background scheduling (e.g. POST
	// routes with deferred mutations) may have transition case names that don't
	// correspond to request-time cases.
	if len(route.Transitions) > 0 {
		if caseName := h.transitions.Resolve(route); caseName != "" {
			if _, ok := route.Cases[caseName]; ok {
				return caseName
			}
		}
	}

	// 3. Fallback.
	return route.Fallback
}

// responseBuffer is a lightweight http.ResponseWriter that buffers the response
// so we can inspect it before flushing (e.g. to detect missing dynamic files).
type responseBuffer struct {
	header   http.Header
	status   int
	body     []byte
	isolated bool // true when the buffer owns its own header map (not shared with w)
}

func newResponseRecorder(w http.ResponseWriter) *responseBuffer {
	return &responseBuffer{
		header: w.Header(),
		status: http.StatusOK,
	}
}

// newIsolatedBuffer creates a responseBuffer with its own independent header
// map, suitable for speculative writes that may be discarded.
func newIsolatedBuffer() *responseBuffer {
	return &responseBuffer{
		header:   make(http.Header),
		status:   http.StatusOK,
		isolated: true,
	}
}

func (rb *responseBuffer) Header() http.Header  { return rb.header }
func (rb *responseBuffer) WriteHeader(code int) { rb.status = code }
func (rb *responseBuffer) Write(b []byte) (int, error) {
	rb.body = append(rb.body, b...)
	return len(b), nil
}

func (rb *responseBuffer) flush(w http.ResponseWriter) {
	// For isolated buffers, copy headers to the target writer.
	if rb.isolated {
		for k, vs := range rb.header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(rb.status)
	_, _ = w.Write(rb.body)
}
