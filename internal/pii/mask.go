// Package pii detects and masks personally identifiable information in
// JSON payloads recorded by the proxy. It is invoked from the recorder
// just before the upstream response is persisted to a stub file, so
// that committed stubs never contain real PII even when proxying live
// healthcare or other sensitive APIs.
//
// The masking pass is layered:
//
//  1. FHIR-aware: when the JSON has a recognised resourceType (Patient,
//     Practitioner, RelatedPerson, Person, or a Bundle wrapping any of
//     those), known-PII paths are masked by field map.
//  2. Field-name: any object key matching a sensitive name (ssn, email,
//     phone, dob, mrn, name, address, ...) has its string value masked.
//  3. Value-shape: remaining string values are scanned for PII-shaped
//     content (SSNs, emails, US phones, Luhn-valid card numbers).
//
// All replacements are deterministic per call: the same original value
// always masks to the same fake within one body, so cross-references
// between FHIR resources (Observation.subject -> Patient/abc) survive.
package pii

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Mask returns body with PII redacted. Non-JSON content types are
// returned unchanged. JSON that fails to parse is also returned
// unchanged — the recorder still has a usable stub even if our masker
// can't help.
//
// contentType comes straight from the upstream Content-Type header. We
// match "application/json" and "application/fhir+json" (FHIR's canonical
// MIME); anything else falls through.
func Mask(body []byte, contentType string) []byte {
	if !isMaskableContentType(contentType) {
		return body
	}
	var root any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // preserve numeric formatting on round-trip
	if err := dec.Decode(&root); err != nil {
		return body
	}

	m := &masker{cache: make(map[string]string)}
	root = m.walkAny(root, "")

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return body
	}
	return out
}

func isMaskableContentType(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.Contains(ct, "application/json") ||
		strings.Contains(ct, "application/fhir+json") ||
		strings.Contains(ct, "+json")
}

// masker carries per-call state. The cache guarantees that the same
// original value masks to the same fake within one body so references
// stay consistent (e.g. a patient ID appearing in multiple resources of
// a Bundle stays linked after masking).
type masker struct {
	cache map[string]string
}

// cached returns the deterministic fake for original under kind, reusing
// any earlier fake produced for the same original in this pass.
func (m *masker) cached(original string, kind fakeKind) string {
	if v, ok := m.cache[original]; ok {
		return v
	}
	v := fake(kind, original)
	m.cache[original] = v
	return v
}

// walkAny recursively masks a parsed JSON value. parentKey is the name
// of the object key whose value is currently being walked — empty at
// the root and inside arrays. The caller is responsible for re-stamping
// the returned value into its parent.
func (m *masker) walkAny(node any, parentKey string) any {
	switch v := node.(type) {
	case map[string]any:
		// FHIR-aware pass first: if this is a typed FHIR resource, apply
		// the resource-specific rule set. Walk continues afterwards so
		// the generic field-name + value-regex passes also see the
		// (now partially masked) tree, catching anything the FHIR rules
		// don't cover.
		if _, ok := v["resourceType"].(string); ok {
			m.applyFHIR(v)
		}
		for k, child := range v {
			v[k] = m.walkAny(child, k)
		}
		return v
	case []any:
		for i, child := range v {
			v[i] = m.walkAny(child, parentKey)
		}
		return v
	case string:
		// Field-name pass: if our parent key flags this as a known PII
		// field, mask the whole value via the configured fake.
		if parentKey != "" {
			if kind, ok := fieldKind(parentKey); ok {
				return m.cached(v, kind)
			}
		}
		// Value-shape pass: catch PII embedded in arbitrary string
		// fields (e.g. an SSN inside a "notes" or "description" blob).
		if out, changed := m.scanString(v); changed {
			return out
		}
		return v
	default:
		return v
	}
}
