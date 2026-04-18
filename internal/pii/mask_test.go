package pii

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// helper: round-trip JSON through Mask and return it as a parsed map for
// easy structural assertions.
func maskAndParse(t *testing.T, body string) map[string]any {
	t.Helper()
	out := Mask([]byte(body), "application/json")
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("masked output is not valid JSON: %v\nout=%s", err, out)
	}
	return got
}

// Generic JSON: a clear-text email behind an "email" key gets masked.
// Per project memory we keep test fixtures in the continent/country/city
// domain except where a healthcare scenario specifically requires it.
func TestMask_GenericEmailField(t *testing.T) {
	in := `{"city":"Lisbon","country":"Portugal","email":"resident@portugal.example"}`
	got := maskAndParse(t, in)

	if got["city"] != "Lisbon" {
		t.Errorf("city should be untouched, got %v", got["city"])
	}
	if got["country"] != "Portugal" {
		t.Errorf("country should be untouched, got %v", got["country"])
	}
	email, _ := got["email"].(string)
	if email == "resident@portugal.example" {
		t.Fatalf("email was not masked")
	}
	if !strings.HasSuffix(email, "@example.com") {
		t.Errorf("masked email should end @example.com, got %q", email)
	}
}

// FHIR Patient: name, birthDate, identifier, telecom, address are all
// reached via the FHIR-aware walker.
func TestMask_FHIRPatient(t *testing.T) {
	in := `{
		"resourceType": "Patient",
		"id": "patient-001",
		"identifier": [{"system": "MRN", "value": "MR-1234567"}],
		"name": [{"given": ["Ada", "Augusta"], "family": "Lovelace"}],
		"birthDate": "1815-12-10",
		"telecom": [{"system": "email", "value": "ada@example.org"}],
		"address": [{
			"line": ["10 Downing Street"],
			"city": "London",
			"postalCode": "SW1A2AA"
		}]
	}`
	got := maskAndParse(t, in)

	names := got["name"].([]any)
	first := names[0].(map[string]any)
	given := first["given"].([]any)
	if given[0] == "Ada" || given[1] == "Augusta" {
		t.Errorf("given names not masked: %v", given)
	}
	if first["family"] == "Lovelace" {
		t.Errorf("family name not masked")
	}

	if got["birthDate"] == "1815-12-10" {
		t.Errorf("birthDate not masked")
	}
	// Year preserved, month/day reset.
	if got["birthDate"] != "1815-01-01" {
		t.Errorf("birthDate should preserve year and reset to 01-01, got %v", got["birthDate"])
	}

	idents := got["identifier"].([]any)
	if idents[0].(map[string]any)["value"] == "MR-1234567" {
		t.Errorf("identifier.value not masked")
	}

	tel := got["telecom"].([]any)
	if tel[0].(map[string]any)["value"] == "ada@example.org" {
		t.Errorf("telecom.value not masked")
	}

	addr := got["address"].([]any)[0].(map[string]any)
	lines := addr["line"].([]any)
	if lines[0] == "10 Downing Street" {
		t.Errorf("address.line not masked")
	}
	if addr["city"] == "London" {
		t.Errorf("address.city not masked")
	}
	if addr["postalCode"] == "SW1A2AA" {
		t.Errorf("postalCode not masked")
	}
}

// FHIR Bundle: each entry.resource is masked according to its own type.
func TestMask_FHIRBundle(t *testing.T) {
	in := `{
		"resourceType": "Bundle",
		"type": "collection",
		"entry": [
			{"resource": {"resourceType":"Patient","id":"p1","name":[{"family":"Lovelace"}]}},
			{"resource": {"resourceType":"Practitioner","id":"pr1","name":[{"family":"Curie"}]}}
		]
	}`
	got := maskAndParse(t, in)
	entries := got["entry"].([]any)
	for i, name := range []string{"Lovelace", "Curie"} {
		res := entries[i].(map[string]any)["resource"].(map[string]any)
		family := res["name"].([]any)[0].(map[string]any)["family"]
		if family == name {
			t.Errorf("entry %d family %q not masked", i, name)
		}
	}
}

// Referential integrity: the same input value masks to the same fake
// within one body, so cross-references between FHIR resources stay
// linked after masking.
func TestMask_DeterministicWithinBody(t *testing.T) {
	in := `{"a":{"email":"shared@example.org"},"b":{"email":"shared@example.org"}}`
	got := maskAndParse(t, in)
	a := got["a"].(map[string]any)["email"]
	b := got["b"].(map[string]any)["email"]
	if a != b {
		t.Errorf("same input should mask to same output within a body: %v vs %v", a, b)
	}
	if a == "shared@example.org" {
		t.Errorf("email was not masked")
	}
}

// Non-JSON content types are returned unchanged so we don't corrupt
// e.g. recorded HTML or binary payloads.
func TestMask_NonJSONPassthrough(t *testing.T) {
	in := []byte("hello, my email is real@example.org")
	out := Mask(in, "text/plain")
	if string(out) != string(in) {
		t.Errorf("text/plain body should be unchanged, got %q", out)
	}
}

// Malformed JSON falls back to passthrough — we'd rather record a usable
// (unmasked) stub than corrupt it.
func TestMask_MalformedJSONPassthrough(t *testing.T) {
	in := []byte(`{not really json`)
	out := Mask(in, "application/json")
	if string(out) != string(in) {
		t.Errorf("malformed JSON should be returned unchanged, got %q", out)
	}
}

// Shape preservation: a masked SSN is still SSN-shaped; a masked DOB is
// still YYYY-MM-DD; a masked email still has an @. Downstream consumers
// of the stub continue to validate.
func TestMask_PreservesShape(t *testing.T) {
	in := `{
		"ssn": "123-45-6789",
		"birthDate": "1980-05-12",
		"email": "x@y.com",
		"phone": "(415) 555-2671"
	}`
	got := maskAndParse(t, in)

	ssn := got["ssn"].(string)
	if !regexp.MustCompile(`^\d{3}-\d{2}-\d{4}$`).MatchString(ssn) {
		t.Errorf("masked SSN lost shape: %q", ssn)
	}

	dob := got["birthDate"].(string)
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(dob) {
		t.Errorf("masked DOB lost shape: %q", dob)
	}

	email := got["email"].(string)
	if !strings.Contains(email, "@") {
		t.Errorf("masked email lost @: %q", email)
	}

	phone := got["phone"].(string)
	if !regexp.MustCompile(`^\(\d{3}\) \d{3}-\d{4}$`).MatchString(phone) {
		t.Errorf("masked phone lost punctuation shape: %q", phone)
	}
}

// Value-shape regex pass: PII embedded in a free-text field gets masked
// even when no key name flagged it.
func TestMask_RegexPassCatchesEmbeddedPII(t *testing.T) {
	in := `{"notes": "Contact ada at ada@example.org or 415-555-2671. SSN 123-45-6789."}`
	got := maskAndParse(t, in)
	notes := got["notes"].(string)
	for _, leak := range []string{"ada@example.org", "415-555-2671", "123-45-6789"} {
		if strings.Contains(notes, leak) {
			t.Errorf("regex pass missed embedded leak %q in: %s", leak, notes)
		}
	}
}

// Luhn gating: a 16-digit non-Luhn order number is NOT replaced as a
// credit card. (The Visa test PAN 4111... passes Luhn so we use a
// known-bad example here.)
func TestMask_LuhnGating(t *testing.T) {
	in := `{"orderId": "1234567890123456"}`
	out := Mask([]byte(in), "application/json")
	// orderId is an unrecognised field name and the value fails Luhn,
	// so it should pass through unchanged.
	if !strings.Contains(string(out), "1234567890123456") {
		t.Errorf("non-Luhn 16-digit value should not be masked, got %s", out)
	}
}

// Common system timestamp keys (createdAt, updatedAt, ...) are NOT
// classified as DOBs by the field-name pass — those are not PII and
// masking them would needlessly break test fixtures.
func TestMask_SkipsSystemTimestamps(t *testing.T) {
	in := `{"createdAt":"2024-03-12","updatedAt":"2024-03-13","expiresAt":"2025-01-01"}`
	out := Mask([]byte(in), "application/json")
	for _, ts := range []string{"2024-03-12", "2024-03-13", "2025-01-01"} {
		if !strings.Contains(string(out), ts) {
			t.Errorf("system timestamp %q was incorrectly masked: %s", ts, out)
		}
	}
}

// FHIR content type alias — application/fhir+json is the canonical FHIR
// MIME and must trigger masking just like application/json.
func TestMask_FHIRContentType(t *testing.T) {
	in := []byte(`{"resourceType":"Patient","name":[{"family":"Curie"}]}`)
	out := Mask(in, "application/fhir+json; charset=utf-8")
	if strings.Contains(string(out), "Curie") {
		t.Errorf("FHIR content type did not trigger masking: %s", out)
	}
}

// Mask is idempotent: running it twice produces the same output as once.
// Without the alreadyMasked shape check, hash-based generators
// (kindIdentifier, kindGivenName/Family, kindEmail) would re-hash their
// own outputs on the second pass and drift. Covers FHIR and generic
// field-name paths plus the value-shape SSN regex.
func TestMask_Idempotent(t *testing.T) {
	inputs := []string{
		// FHIR Patient — exercises kindIdentifier, kindGivenName,
		// kindFamilyName, kindDOB, kindGenericPII (telecom).
		`{
			"resourceType":"Patient",
			"id":"abc",
			"identifier":[{"value":"MR-99"}],
			"birthDate":"1990-05-12",
			"name":[{"family":"Smith","given":["John"]}],
			"telecom":[{"system":"email","value":"john@example.org"}]
		}`,
		// Generic field-name pass — kindEmail, kindPhone, kindSSN.
		`{"email":"resident@portugal.example","phone":"(415) 555-2671","ssn":"123-45-6789"}`,
		// Free-text regex pass — same three shapes embedded in prose.
		`{"notes":"Contact resident@portugal.example or 415-555-2671. SSN 123-45-6789."}`,
	}
	for i, in := range inputs {
		first := Mask([]byte(in), "application/json")
		second := Mask(first, "application/json")
		if string(first) != string(second) {
			t.Errorf("input %d: Mask not idempotent\nfirst:\n%s\nsecond:\n%s", i, first, second)
		}
	}
}
