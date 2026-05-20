package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validationLoader is a minimal configLoader for validation tests — we don't
// touch the filesystem, so ConfigDir/StubRoot return empty strings.
type validationLoader struct{ cfg *config.Config }

func (l *validationLoader) Get() *config.Config { return l.cfg }
func (l *validationLoader) AddRoute(config.Route)   {}
func (l *validationLoader) ConfigDir() string    { return "" }
func (l *validationLoader) StubRoot() string     { return "" }

func newValidationHandler(cfg *config.Config) *Handler {
	return NewHandler(&validationLoader{cfg: cfg}, nil, false, "")
}

func cityRoute(rules []config.ValidationRule) config.Route {
	return config.Route{
		Method:     "POST",
		Match:      "/cities",
		Fallback:   "created",
		Validation: rules,
		Cases: map[string]config.Case{
			"created": {Status: 201, JSON: `{"ok":true}`},
		},
	}
}

func TestValidation_RejectsMissingRequiredField(t *testing.T) {
	cfg := &config.Config{Routes: []config.Route{cityRoute([]config.ValidationRule{
		{Field: "name", Op: "required"},
		{Field: "country_code", Op: "pattern", Value: "^[A-Z]{2}$"},
	})}}
	h := newValidationHandler(cfg)

	body := bytes.NewBufferString(`{"country_code":"FR"}`)
	req := httptest.NewRequest(http.MethodPost, "/cities", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp struct {
		Error      string `json:"error"`
		Violations []struct {
			Field   string `json:"field"`
			Rule    string `json:"rule"`
			Message string `json:"message"`
		} `json:"violations"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "validation failed", resp.Error)
	require.Len(t, resp.Violations, 1)
	assert.Equal(t, "name", resp.Violations[0].Field)
	assert.Equal(t, "required", resp.Violations[0].Rule)
}

func TestValidation_PassesThroughOnValidPayload(t *testing.T) {
	cfg := &config.Config{Routes: []config.Route{cityRoute([]config.ValidationRule{
		{Field: "name", Op: "required"},
		{Field: "continent", Op: "in", Value: "africa,europe,asia,americas,oceania,antarctica"},
		{Field: "population", Op: "gte", Value: "0"},
	})}}
	h := newValidationHandler(cfg)

	body := bytes.NewBufferString(`{"name":"Lima","continent":"americas","population":9700000}`)
	req := httptest.NewRequest(http.MethodPost, "/cities", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
}

func TestValidation_CollectsAllViolations(t *testing.T) {
	cfg := &config.Config{Routes: []config.Route{cityRoute([]config.ValidationRule{
		{Field: "name", Op: "required"},
		{Field: "country_code", Op: "pattern", Value: "^[A-Z]{2}$"},
		{Field: "population", Op: "gte", Value: "0"},
		{Field: "continent", Op: "in", Value: "africa,europe,asia,americas,oceania,antarctica"},
	})}}
	h := newValidationHandler(cfg)

	body := bytes.NewBufferString(`{"country_code":"fra","population":-1,"continent":"atlantis"}`)
	req := httptest.NewRequest(http.MethodPost, "/cities", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var resp struct {
		Violations []struct {
			Field string `json:"field"`
			Rule  string `json:"rule"`
		} `json:"violations"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Len(t, resp.Violations, 4)
}

func TestValidation_EmptyBodyFailsRequired(t *testing.T) {
	cfg := &config.Config{Routes: []config.Route{cityRoute([]config.ValidationRule{
		{Field: "name", Op: "required"},
	})}}
	h := newValidationHandler(cfg)

	req := httptest.NewRequest(http.MethodPost, "/cities", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestValidation_NoRulesNoCheck(t *testing.T) {
	// Route with no validation block should not even attempt to parse
	// the body — verifies the early-exit before parseJSONBody.
	cfg := &config.Config{Routes: []config.Route{cityRoute(nil)}}
	h := newValidationHandler(cfg)

	body := bytes.NewBufferString(`not even json`)
	req := httptest.NewRequest(http.MethodPost, "/cities", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
}
