package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ridakaddir/apitwin/internal/config"
)

// maxConfigBodyBytes caps incoming /__api/config/* request bodies.
const maxConfigBodyBytes = 4 << 20

// configFileInfo is the wire shape returned by GET /__api/config/files.
type configFileInfo struct {
	Path            string `json:"path"`
	Name            string `json:"name"`
	Format          string `json:"format"`
	RouteCount      int    `json:"routeCount"`
	GRPCRouteCount  int    `json:"grpcRouteCount"`
	ModUnix         int64  `json:"mtime"`
	Writable        bool   `json:"writable"`
	ContainsRoute   bool   `json:"containsRoute,omitempty"`
}

// apiConfigFilesHandler returns the set of config files the loader is watching.
// In file mode, returns a single entry. In directory mode, returns all
// candidate files in sorted order.
//
// Optional query params filter to files that already contain a given route:
//
//	?kind=http&match=/foo&method=GET
//	?kind=grpc&match=/pkg.Svc/Method
//
// When filter params are supplied, the response sets ContainsRoute=true on
// matching files. This lets the UI pre-select the file that currently owns
// the route being edited.
func (s *Server) apiConfigFilesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := s.loader.Path()
	files, err := config.ListConfigFiles(path, s.loader.IsDir())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var filterSel *config.Selector
	if kind := r.URL.Query().Get("kind"); kind != "" {
		filterSel = &config.Selector{
			Kind:   config.RouteKind(kind),
			Match:  r.URL.Query().Get("match"),
			Method: r.URL.Query().Get("method"),
		}
	}

	out := make([]configFileInfo, 0, len(files))
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		cfg, err := config.Load(f)
		if err != nil {
			out = append(out, configFileInfo{
				Path:   f,
				Name:   filepath.Base(f),
				Format: config.FormatForPath(f),
			})
			continue
		}
		entry := configFileInfo{
			Path:           f,
			Name:           filepath.Base(f),
			Format:         config.FormatForPath(f),
			RouteCount:     len(cfg.Routes),
			GRPCRouteCount: len(cfg.GRPCRoutes),
			ModUnix:        info.ModTime().Unix(),
			Writable:       isWritable(f),
		}
		if filterSel != nil {
			entry.ContainsRoute = containsRoute(cfg, *filterSel)
		}
		out = append(out, entry)
	}

	writeJSON(w, http.StatusOK, out)
}

// previewRequest is the body of POST /__api/config/preview.
type previewRequest struct {
	Kind   string          `json:"kind"`   // "http" | "grpc"
	Route  json.RawMessage `json:"route"`  // single route to wrap
	Format string          `json:"format"` // "toml" | "yaml" | "json"
}

// apiConfigPreviewHandler renders a single route in the requested format so
// the UI can show a live preview pane.
func (s *Server) apiConfigPreviewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxConfigBodyBytes)

	var req previewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Format == "" {
		req.Format = "toml"
	}

	data, err := marshalRouteFromRaw(config.RouteKind(req.Kind), req.Route, req.Format)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": string(data)})
}

// validateRequest is the body of POST /__api/config/validate.
type validateRequest struct {
	Kind  string          `json:"kind"`
	Route json.RawMessage `json:"route"`
}

type validationErrorOut struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// apiConfigValidateHandler runs the existing config validators against the
// posted route and returns errors keyed by a dotted form-field path so the
// UI can attach them to inputs.
func (s *Server) apiConfigValidateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxConfigBodyBytes)

	var req validateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	var validationErrors []config.ValidationError
	var grpcSchemaAvailable bool

	switch config.RouteKind(req.Kind) {
	case config.KindHTTP:
		var route config.Route
		if err := json.Unmarshal(req.Route, &route); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid route json: "+err.Error())
			return
		}
		validationErrors = config.ValidateRESTRoutes([]config.Route{route})
	case config.KindGRPC:
		var route config.GRPCRoute
		if err := json.Unmarshal(req.Route, &route); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid route json: "+err.Error())
			return
		}
		schema := s.loadGRPCSchemaForValidate()
		grpcSchemaAvailable = schema != nil
		validationErrors = config.ValidateGRPCRoutes([]config.GRPCRoute{route}, schema)
	default:
		writeJSONError(w, http.StatusBadRequest, "unknown kind "+req.Kind)
		return
	}

	out := make([]validationErrorOut, 0, len(validationErrors))
	for _, e := range validationErrors {
		out = append(out, validationErrorOut{
			Path:   fieldPathFor(e),
			Reason: e.Reason,
		})
	}

	resp := map[string]any{"errors": out}
	if config.RouteKind(req.Kind) == config.KindGRPC {
		resp["grpcSchemaAvailable"] = grpcSchemaAvailable
	}
	writeJSON(w, http.StatusOK, resp)
}

// saveRequest is the body of POST /__api/config/routes.
type saveRequest struct {
	File    string           `json:"file"`
	Kind    string           `json:"kind"`
	Route   json.RawMessage  `json:"route"`
	Replace *replaceSelector `json:"replace,omitempty"`
	IfMatch int64            `json:"ifMatch,omitempty"`
}

type replaceSelector struct {
	Match  string `json:"match"`
	Method string `json:"method,omitempty"`
}

// deleteRequest is the body of DELETE /__api/config/routes.
type deleteRequest struct {
	File   string `json:"file"`
	Kind   string `json:"kind"`
	Match  string `json:"match"`
	Method string `json:"method,omitempty"`
}

// apiConfigRoutesHandler handles save (POST) and delete (DELETE) of routes
// inside config files.
func (s *Server) apiConfigRoutesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleSaveRoute(w, r)
	case http.MethodDelete:
		s.handleDeleteRoute(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSaveRoute(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxConfigBodyBytes)
	var req saveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.File == "" {
		writeJSONError(w, http.StatusBadRequest, "missing `file`")
		return
	}
	if err := s.ensureFileAllowed(req.File); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.configWriteMu.Lock()
	defer s.configWriteMu.Unlock()

	if req.IfMatch != 0 {
		if info, err := os.Stat(req.File); err == nil {
			if info.ModTime().Unix() != req.IfMatch {
				writeJSONError(w, http.StatusConflict, "file changed on disk since edit started")
				return
			}
		}
	}

	kind := config.RouteKind(req.Kind)
	var route any
	switch kind {
	case config.KindHTTP:
		var r0 config.Route
		if err := json.Unmarshal(req.Route, &r0); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid route json: "+err.Error())
			return
		}
		route = r0
	case config.KindGRPC:
		var r0 config.GRPCRoute
		if err := json.Unmarshal(req.Route, &r0); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid route json: "+err.Error())
			return
		}
		route = r0
	default:
		writeJSONError(w, http.StatusBadRequest, "unknown kind "+req.Kind)
		return
	}

	var replace *config.Selector
	if req.Replace != nil {
		replace = &config.Selector{
			Kind:   kind,
			Match:  req.Replace.Match,
			Method: req.Replace.Method,
		}
	}

	err := config.UpsertRouteInFile(req.File, kind, route, replace)
	if errors.Is(err, config.ErrRouteNotFound) {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, config.ErrRouteConflict) {
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var mtime int64
	if info, err := os.Stat(req.File); err == nil {
		mtime = info.ModTime().Unix()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"file":  req.File,
		"mtime": mtime,
	})
}

func (s *Server) handleDeleteRoute(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxConfigBodyBytes)
	var req deleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.File == "" {
		writeJSONError(w, http.StatusBadRequest, "missing `file`")
		return
	}
	if err := s.ensureFileAllowed(req.File); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.configWriteMu.Lock()
	defer s.configWriteMu.Unlock()

	sel := config.Selector{
		Kind:   config.RouteKind(req.Kind),
		Match:  req.Match,
		Method: req.Method,
	}
	err := config.DeleteRouteFromFile(req.File, sel)
	if errors.Is(err, config.ErrRouteNotFound) {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ensureFileAllowed gates writes to paths inside the loader's config root
// (the file in file mode, or anywhere under the dir in directory mode). It
// also forbids paths with traversal segments.
func (s *Server) ensureFileAllowed(p string) error {
	cleaned := filepath.Clean(p)
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("invalid path %q", p)
	}
	ext := config.FormatForPath(cleaned)
	if ext == "" {
		return fmt.Errorf("file must end in .toml, .yaml, .yml, or .json: %s", p)
	}

	if !s.loader.IsDir() {
		if filepath.Clean(s.loader.Path()) != cleaned {
			return fmt.Errorf("file %q is outside the loaded config", p)
		}
		return nil
	}

	root, err := filepath.Abs(s.loader.Path())
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("file %q is outside the config directory", p)
	}
	return nil
}

// loadGRPCSchemaForValidate adapts the gRPC invoker's Schema() into a
// config.PersistSchema for validation, or returns nil when no invoker is
// wired (config validator then runs structural-only).
func (s *Server) loadGRPCSchemaForValidate() config.PersistSchema {
	inv := s.loadGRPCInvoker()
	if inv == nil {
		return nil
	}
	if schema, ok := inv.Schema().(config.PersistSchema); ok {
		return schema
	}
	return nil
}

// marshalRouteFromRaw unmarshals raw JSON into the concrete route type for
// kind and re-marshals it via config.MarshalRoute. The intermediate concrete
// type ensures toml/yaml emit the same field names the loader expects.
func marshalRouteFromRaw(kind config.RouteKind, raw json.RawMessage, format string) ([]byte, error) {
	switch kind {
	case config.KindHTTP:
		var r config.Route
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &r); err != nil {
				return nil, fmt.Errorf("invalid route json: %w", err)
			}
		}
		return config.MarshalRoute(kind, r, format)
	case config.KindGRPC:
		var r config.GRPCRoute
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &r); err != nil {
				return nil, fmt.Errorf("invalid route json: %w", err)
			}
		}
		return config.MarshalRoute(kind, r, format)
	}
	return nil, fmt.Errorf("unknown kind %q", kind)
}

// fieldPathFor maps a validator's ValidationError to a dotted form-field
// path the UI uses to attach the error to an input.
func fieldPathFor(e config.ValidationError) string {
	switch {
	case e.Case == "" && strings.HasPrefix(e.Reason, "fallback "):
		return "fallback"
	case e.Case != "":
		return "cases." + e.Case
	}
	return ""
}

func containsRoute(cfg *config.Config, sel config.Selector) bool {
	switch sel.Kind {
	case config.KindHTTP:
		for _, r := range cfg.Routes {
			if r.Match == sel.Match && strings.EqualFold(r.Method, sel.Method) {
				return true
			}
		}
	case config.KindGRPC:
		for _, r := range cfg.GRPCRoutes {
			if r.Match == sel.Match {
				return true
			}
		}
	}
	return false
}

func isWritable(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
