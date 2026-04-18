package pii

import (
	"regexp"
	"strings"
)

// fieldName matches a JSON object key against the list of substrings
// known to carry PII and returns the appropriate fakeKind. Matching is
// case-insensitive and substring-based so variations like "user_email",
// "EmailAddress", "primaryEmail" all resolve to kindEmail.
//
// Order matters: more specific matches must be checked first (e.g.
// "birthDate" before generic "date").
func fieldKind(name string) (fakeKind, bool) {
	n := strings.ToLower(name)
	for _, r := range fieldRules {
		if strings.Contains(n, r.needle) {
			// Reject substrings that would over-match. e.g. "name" inside
			// "filename" or "username". Specific exclusions keep the rule
			// list small instead of forcing exact matches.
			if r.exclude != nil && r.exclude.MatchString(n) {
				continue
			}
			return r.kind, true
		}
	}
	return 0, false
}

type fieldRule struct {
	needle  string
	kind    fakeKind
	exclude *regexp.Regexp // optional — skip the rule if name matches this
}

// Excludes for over-matching needles. "name" is the worst offender —
// "filename", "username", "hostname", "tagname", "displayname",
// "classname" are all common keys that aren't human names.
var nameExclude = regexp.MustCompile(`(file|user|host|tag|display|class|field|table|column|path|key|var|param|type|status|brand|product|company|org|store|service|operation|method|step|state|env|module|package|node|cluster|region|zone|domain|app|event|action|role|rule|model|schema|metric|label|alias|category|department|group|team|project|repo|branch|tag|version|release|build|stage|enum|flag|setting|preference|option|locale|language|currency|country|continent|city|county|district|province|territory|nickname|surname)`)

var dateExclude = regexp.MustCompile(`(create|update|modif|delet|expir|issu|effect|recorded|publish|start|end|begin|finish|valid|due|sent|received|posted|fetched|sync)`)

var fieldRules = []fieldRule{
	// Healthcare / FHIR-flavoured first so they win over generic "id".
	{needle: "mrn", kind: kindIdentifier},
	{needle: "medicalrecord", kind: kindIdentifier},
	{needle: "patientid", kind: kindIdentifier},
	{needle: "patient_id", kind: kindIdentifier},
	{needle: "nationalid", kind: kindIdentifier},
	{needle: "national_id", kind: kindIdentifier},

	// Contact info.
	{needle: "email", kind: kindEmail},
	{needle: "phone", kind: kindPhone},
	{needle: "mobile", kind: kindPhone},
	{needle: "telephone", kind: kindPhone},
	{needle: "fax", kind: kindPhone},

	// Government / financial.
	{needle: "ssn", kind: kindSSN},
	{needle: "socialsecurity", kind: kindSSN},
	{needle: "social_security", kind: kindSSN},
	{needle: "creditcard", kind: kindCreditCard},
	{needle: "credit_card", kind: kindCreditCard},
	{needle: "cardnumber", kind: kindCreditCard},
	{needle: "card_number", kind: kindCreditCard},
	{needle: "cardnum", kind: kindCreditCard},
	{needle: "pan", kind: kindCreditCard},

	// Dates of birth.
	{needle: "birthdate", kind: kindDOB},
	{needle: "birth_date", kind: kindDOB},
	{needle: "dateofbirth", kind: kindDOB},
	{needle: "date_of_birth", kind: kindDOB},
	{needle: "dob", kind: kindDOB},

	// Dates that may carry PII (admission, discharge). Excluded common
	// system timestamps.
	{needle: "date", kind: kindDOB, exclude: dateExclude},

	// Names.
	{needle: "firstname", kind: kindGivenName},
	{needle: "first_name", kind: kindGivenName},
	{needle: "givenname", kind: kindGivenName},
	{needle: "given_name", kind: kindGivenName},
	{needle: "lastname", kind: kindFamilyName},
	{needle: "last_name", kind: kindFamilyName},
	{needle: "familyname", kind: kindFamilyName},
	{needle: "family_name", kind: kindFamilyName},
	{needle: "fullname", kind: kindFullName},
	{needle: "full_name", kind: kindFullName},
	{needle: "patientname", kind: kindFullName},
	{needle: "patient_name", kind: kindFullName},
	{needle: "name", kind: kindFullName, exclude: nameExclude},

	// Addresses.
	{needle: "addressline", kind: kindAddressLine},
	{needle: "address_line", kind: kindAddressLine},
	{needle: "streetaddress", kind: kindAddressLine},
	{needle: "street_address", kind: kindAddressLine},
	{needle: "street", kind: kindAddressLine},
	{needle: "address", kind: kindAddressLine},
	{needle: "postalcode", kind: kindPostalCode},
	{needle: "postal_code", kind: kindPostalCode},
	{needle: "zipcode", kind: kindPostalCode},
	{needle: "zip_code", kind: kindPostalCode},
	{needle: "zip", kind: kindPostalCode},
}

// Value-shape regexes. Run after the field-name pass to catch leaks in
// arbitrary string values (e.g. an SSN buried in a "notes" field).
var (
	reEmail = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	reSSN   = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	// US phone variants: (xxx) xxx-xxxx, xxx-xxx-xxxx, xxx.xxx.xxxx, +1 xxx xxx xxxx.
	rePhone = regexp.MustCompile(`(\+?1[\s\-.]?)?\(?\b\d{3}\)?[\s\-.]\d{3}[\s\-.]\d{4}\b`)
	// 13-19 digit numbers with optional space/dash separators.
	// Luhn-validated downstream so we don't replace e.g. order numbers.
	reCardLike = regexp.MustCompile(`\b(?:\d[ -]?){12,18}\d\b`)
)

// luhn returns true if the digits-only candidate passes the Luhn
// checksum. Used to gate creditCard regex matches so plain numeric IDs
// of similar length don't get clobbered.
func luhn(s string) bool {
	digits := 0
	sum := 0
	alt := false
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if c < '0' || c > '9' {
			continue
		}
		d := int(c - '0')
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
		digits++
	}
	return digits >= 13 && digits <= 19 && sum%10 == 0
}

// scanString applies the regex pass to a single string value. Returns
// the (possibly rewritten) string and a flag indicating whether any
// substitution happened. The masker uses this for any string that
// wasn't already replaced by a field-name or FHIR-path rule.
func (m *masker) scanString(s string) (string, bool) {
	out := s
	changed := false
	out = reEmail.ReplaceAllStringFunc(out, func(match string) string {
		changed = true
		return m.cached(match, kindEmail)
	})
	out = reSSN.ReplaceAllStringFunc(out, func(match string) string {
		changed = true
		return m.cached(match, kindSSN)
	})
	out = rePhone.ReplaceAllStringFunc(out, func(match string) string {
		changed = true
		return m.cached(match, kindPhone)
	})
	out = reCardLike.ReplaceAllStringFunc(out, func(match string) string {
		if !luhn(match) {
			return match
		}
		changed = true
		return m.cached(match, kindCreditCard)
	})
	return out, changed
}
