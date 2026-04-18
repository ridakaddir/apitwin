package proxy

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ridakaddir/apitwin/internal/config"
	"github.com/stretchr/testify/require"
)

// recorderTestLoader is the minimal routeLoader the recorder needs.
// Captures the route it would have injected so we can assert it was
// wired correctly.
type recorderTestLoader struct {
	added []config.Route
}

func (l *recorderTestLoader) AddRoute(r config.Route) { l.added = append(l.added, r) }

// TestRecorder_MasksPIIBeforeWritingStub drives the recorder callback
// directly with a synthetic FHIR Patient response and asserts the
// on-disk stub has had its PII redacted while the original input we
// would have served the client is unaffected by the recorder (the
// recorder runs after capturingWriter has already flushed).
func TestRecorder_MasksPIIBeforeWritingStub(t *testing.T) {
	seed := t.TempDir()
	loader := &recorderTestLoader{}

	rec := recorder(seed, seed, loader, true /* maskPII */)

	// Simulate an upstream FHIR Patient response.
	const realFamily = "Lovelace"
	const realGiven = "Ada"
	const realEmail = "ada@example.org"
	const realMRN = "MR-1234567"
	body := []byte(`{
		"resourceType":"Patient",
		"id":"patient-001",
		"identifier":[{"system":"MRN","value":"` + realMRN + `"}],
		"name":[{"given":["` + realGiven + `"],"family":"` + realFamily + `"}],
		"birthDate":"1815-12-10",
		"telecom":[{"system":"email","value":"` + realEmail + `"}]
	}`)
	header := http.Header{}
	header.Set("Content-Type", "application/fhir+json")

	req := httptest.NewRequest(http.MethodGet, "/Patient/patient-001", nil)
	rec(req, 200, header, body, 50*time.Millisecond)

	// slugify preserves case, so "/Patient/patient-001" → "get_Patient_patient-001.json".
	// On case-insensitive filesystems (macOS default) the lowercased form
	// also matched, but Linux CI is case-sensitive.
	stubPath := filepath.Join(seed, "stubs", "get_Patient_patient-001.json")
	got, err := os.ReadFile(stubPath)
	require.NoError(t, err, "stub file should be written")

	for _, leak := range []string{realFamily, realGiven, realEmail, realMRN, "1815-12-10"} {
		if strings.Contains(string(got), leak) {
			t.Errorf("stub leaked %q:\n%s", leak, got)
		}
	}

	// Year of birth is intentionally preserved so masked stubs remain
	// useful for age-bucketing tests.
	if !strings.Contains(string(got), "1815") {
		t.Errorf("stub should preserve birth year, got: %s", got)
	}

	// And the route was injected so subsequent requests are served from
	// the stub.
	require.Len(t, loader.added, 1)
	require.Equal(t, "GET", loader.added[0].Method)
	require.Equal(t, "/Patient/patient-001", loader.added[0].Match)
}

// TestRecorder_MaskPIIDisabledLeavesBytesAsIs proves the opt-out path.
// With maskPII=false the recorder writes the upstream body verbatim
// (modulo the JSON pretty-printer that always runs).
func TestRecorder_MaskPIIDisabledLeavesBytesAsIs(t *testing.T) {
	seed := t.TempDir()
	loader := &recorderTestLoader{}

	rec := recorder(seed, seed, loader, false /* maskPII */)

	body := []byte(`{"email":"resident@portugal.example","city":"Lisbon"}`)
	header := http.Header{}
	header.Set("Content-Type", "application/json")

	req := httptest.NewRequest(http.MethodGet, "/visitors", nil)
	rec(req, 200, header, body, 0)

	stubPath := filepath.Join(seed, "stubs", "get_visitors.json")
	got, err := os.ReadFile(stubPath)
	require.NoError(t, err)

	if !strings.Contains(string(got), "resident@portugal.example") {
		t.Errorf("--no-mask-pii should preserve the original email, got: %s", got)
	}
}
