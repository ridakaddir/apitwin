package pii

// fhirRules maps FHIR resourceType values to the per-field replacements
// the walker should apply when it sees a resource of that type.
//
// Coverage focuses on the "person-like" resources (Patient, Practitioner,
// RelatedPerson, Person) and the Bundle wrapper that holds them. Other
// resource types still get the generic field-name + value-regex passes,
// so anything PII-shaped (emails, SSNs, etc.) inside e.g. an Observation
// is still caught.
var fhirRules = map[string][]fhirField{
	"Patient":       personFields(),
	"Practitioner":  personFields(),
	"RelatedPerson": personFields(),
	"Person":        personFields(),
}

// fhirField is one field-mask rule inside a FHIR resource. The path is
// applied as a sequence of steps; "[*]" means "for each element of an
// array".
type fhirField struct {
	path []string
	kind fakeKind
}

// personFields returns the masking rules shared by all person-shaped
// FHIR resources (Patient, Practitioner, RelatedPerson, Person). They
// expose the same demographic surface, so they share the rule set.
func personFields() []fhirField {
	return []fhirField{
		{path: []string{"name", "[*]", "given", "[*]"}, kind: kindGivenName},
		{path: []string{"name", "[*]", "family"}, kind: kindFamilyName},
		{path: []string{"name", "[*]", "text"}, kind: kindFullName},
		{path: []string{"name", "[*]", "prefix", "[*]"}, kind: kindGenericPII},
		{path: []string{"name", "[*]", "suffix", "[*]"}, kind: kindGenericPII},
		{path: []string{"birthDate"}, kind: kindDOB},
		{path: []string{"deceasedDateTime"}, kind: kindDOB},
		{path: []string{"identifier", "[*]", "value"}, kind: kindIdentifier},
		{path: []string{"telecom", "[*]", "value"}, kind: kindGenericPII},
		{path: []string{"address", "[*]", "line", "[*]"}, kind: kindAddressLine},
		{path: []string{"address", "[*]", "city"}, kind: kindCity},
		{path: []string{"address", "[*]", "district"}, kind: kindCity},
		{path: []string{"address", "[*]", "postalCode"}, kind: kindPostalCode},
		{path: []string{"address", "[*]", "text"}, kind: kindAddressLine},
		{path: []string{"photo", "[*]", "data"}, kind: kindGenericPII},
		// Contact subobjects mirror the patient demographics.
		{path: []string{"contact", "[*]", "name", "given", "[*]"}, kind: kindGivenName},
		{path: []string{"contact", "[*]", "name", "family"}, kind: kindFamilyName},
		{path: []string{"contact", "[*]", "telecom", "[*]", "value"}, kind: kindGenericPII},
	}
}

// applyFHIR walks a parsed FHIR resource (a map[string]any rooted at a
// resourceType) and applies the rule set for that type. Bundle is
// handled specially: each Bundle.entry[].resource is recursively masked
// as its own typed resource.
func (m *masker) applyFHIR(node map[string]any) {
	rt, ok := node["resourceType"].(string)
	if !ok {
		return
	}
	if rt == "Bundle" {
		entries, _ := node["entry"].([]any)
		for _, e := range entries {
			em, ok := e.(map[string]any)
			if !ok {
				continue
			}
			res, ok := em["resource"].(map[string]any)
			if !ok {
				continue
			}
			m.applyFHIR(res)
			// Even if there's no specific rule set for the resource
			// type, fall through to the generic walker so emails/SSNs
			// in unknown resources still get caught.
			m.walkAny(res, "")
		}
		return
	}
	rules, ok := fhirRules[rt]
	if !ok {
		return
	}
	for _, f := range rules {
		m.applyPath(node, f.path, f.kind)
	}
}

// applyPath drills into the node along the path steps and masks each
// matching leaf. "[*]" steps fan out across array elements; missing
// keys simply terminate that branch.
func (m *masker) applyPath(node any, path []string, kind fakeKind) {
	if len(path) == 0 {
		// Reached a leaf — mask if it's a string.
		if s, ok := node.(string); ok && s != "" {
			_ = s // value substitution happens via the parent (see below)
		}
		return
	}
	step := path[0]
	rest := path[1:]
	if step == "[*]" {
		arr, ok := node.([]any)
		if !ok {
			return
		}
		for i, child := range arr {
			if len(rest) == 0 {
				if s, ok := child.(string); ok && s != "" {
					arr[i] = m.cached(s, kind)
				}
				continue
			}
			m.applyPath(child, rest, kind)
		}
		return
	}
	obj, ok := node.(map[string]any)
	if !ok {
		return
	}
	child, exists := obj[step]
	if !exists {
		return
	}
	if len(rest) == 0 {
		if s, ok := child.(string); ok && s != "" {
			obj[step] = m.cached(s, kind)
		}
		return
	}
	m.applyPath(child, rest, kind)
}
