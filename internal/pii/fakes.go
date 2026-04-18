package pii

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// fakeKind enumerates the categories of synthetic replacements the masker
// can produce. Each kind maps to a shape-preserving generator so masked
// stubs remain structurally valid (a masked SSN still parses as SSN, a
// masked DOB still parses as YYYY-MM-DD, etc.).
type fakeKind int

const (
	kindEmail fakeKind = iota
	kindSSN
	kindPhone
	kindCreditCard
	kindDOB
	kindGivenName
	kindFamilyName
	kindFullName
	kindAddressLine
	kindCity
	kindPostalCode
	kindIdentifier
	kindGenericPII
)

// hashHex returns the first n hex chars of sha256(s). Used as a
// deterministic seed so the same original value always masks to the same
// fake within a single masking pass (preserving referential integrity
// inside one stub body).
func hashHex(s string, n int) string {
	sum := sha256.Sum256([]byte(s))
	enc := hex.EncodeToString(sum[:])
	if n > len(enc) {
		n = len(enc)
	}
	return enc[:n]
}

// hashMod returns sha256(s) interpreted as a number, modulo m. Used to
// pick deterministic "last digits" for SSN/phone fakes.
func hashMod(s string, m int) int {
	sum := sha256.Sum256([]byte(s))
	// 4 bytes is plenty for m up to 2^32.
	v := uint32(sum[0])<<24 | uint32(sum[1])<<16 | uint32(sum[2])<<8 | uint32(sum[3])
	return int(v % uint32(m))
}

// fake returns a deterministic synthetic value for the given kind.
// Generators preserve the original format wherever possible.
func fake(kind fakeKind, original string) string {
	switch kind {
	case kindEmail:
		return fmt.Sprintf("user-%s@example.com", hashHex(original, 6))
	case kindSSN:
		// 900-series SSNs are reserved by SSA and never issued — using
		// them makes any mask that leaks into prod immediately obvious.
		return fmt.Sprintf("900-00-%04d", hashMod(original, 10000))
	case kindPhone:
		return fakePhonePreservingFormat(original)
	case kindCreditCard:
		// Visa test PAN — passes Luhn.
		return preserveCardFormat(original, "4111111111111111")
	case kindDOB:
		return fakeDOBPreservingFormat(original)
	case kindGivenName:
		return "Given-" + hashHex(original, 6)
	case kindFamilyName:
		return "Family-" + hashHex(original, 6)
	case kindFullName:
		return "Patient-" + hashHex(original, 6)
	case kindAddressLine:
		return "123 Main St"
	case kindCity:
		return "Anytown"
	case kindPostalCode:
		return preserveDigitsLen(original, "00000")
	case kindIdentifier:
		return "ID-" + strings.ToUpper(hashHex(original, 8))
	default:
		return "REDACTED-" + hashHex(original, 6)
	}
}

var phoneDigitsOnly = regexp.MustCompile(`\D`)

// Shape regexes matching values that `fake` already produced. Used by
// alreadyMasked to make Mask idempotent across calls — without these,
// hash-based generators would re-hash their own outputs on a second
// pass and produce drift. Generators whose output is either fixed
// ("123 Main St", "Anytown") or derived deterministically from the
// input shape (phone, card, DOB, postalCode) are idempotent without
// needing a regex here.
var (
	reFakeEmail      = regexp.MustCompile(`^user-[0-9a-f]{6}@example\.com$`)
	reFakeSSN        = regexp.MustCompile(`^900-00-\d{4}$`)
	reFakeGivenName  = regexp.MustCompile(`^Given-[0-9a-f]{6}$`)
	reFakeFamilyName = regexp.MustCompile(`^Family-[0-9a-f]{6}$`)
	reFakeFullName   = regexp.MustCompile(`^Patient-[0-9a-f]{6}$`)
	reFakeIdentifier = regexp.MustCompile(`^ID-[0-9A-F]{8}$`)
	reFakeGenericPII = regexp.MustCompile(`^REDACTED-[0-9a-f]{6}$`)
)

// isFakePhone reports whether s, stripped of non-digits, is
// 555-555-01XX (10 digits) or 1-555-555-01XX (11 digits with country
// code) — i.e. already one of our phone fakes.
func isFakePhone(s string) bool {
	digits := phoneDigitsOnly.ReplaceAllString(s, "")
	switch len(digits) {
	case 10:
		return strings.HasPrefix(digits, "55555501")
	case 11:
		return strings.HasPrefix(digits, "155555501")
	}
	return false
}

// alreadyMasked reports whether s is already in the shape a previous
// fake(kind, _) call would have produced. Used to short-circuit Mask on
// values we've already replaced, so Mask(Mask(x)) == Mask(x).
func alreadyMasked(kind fakeKind, s string) bool {
	switch kind {
	case kindEmail:
		return reFakeEmail.MatchString(s)
	case kindSSN:
		return reFakeSSN.MatchString(s)
	case kindPhone:
		return isFakePhone(s)
	case kindGivenName:
		return reFakeGivenName.MatchString(s)
	case kindFamilyName:
		return reFakeFamilyName.MatchString(s)
	case kindFullName:
		return reFakeFullName.MatchString(s)
	case kindIdentifier:
		return reFakeIdentifier.MatchString(s)
	case kindGenericPII:
		return reFakeGenericPII.MatchString(s)
	}
	// kindCreditCard, kindDOB, kindAddressLine, kindCity, kindPostalCode
	// all have idempotent generators — running fake() on their own
	// output yields the same value, so no shape check is needed.
	return false
}

// fakePhonePreservingFormat emits a fake US phone number that keeps the
// punctuation pattern of the original. 555-555-01XX is fully fictional:
// NPA 555 is reserved by NANP and 555-01XX is the TV/film subscriber
// range, so the result can never collide with a real number. A leading
// "+" in the original is naturally preserved because we copy original
// into `out` and only overwrite digits.
func fakePhonePreservingFormat(original string) string {
	digits := phoneDigitsOnly.ReplaceAllString(original, "")
	suffix := fmt.Sprintf("%02d", hashMod(original, 100))
	// 10 digits: area "555" + exchange "555" + line "01XX".
	fakeDigits := "55555501" + suffix
	if len(digits) == 11 {
		fakeDigits = "1" + fakeDigits
	}
	// Re-stamp the original separators into the fake digits.
	out := []byte(original)
	di := 0
	for i, ch := range original {
		if ch >= '0' && ch <= '9' {
			if di < len(fakeDigits) {
				out[i] = fakeDigits[di]
				di++
			}
		}
	}
	return string(out)
}

// preserveCardFormat re-stamps a 16-digit replacement onto the original
// card string keeping any spaces/dashes in place.
func preserveCardFormat(original, replacement string) string {
	out := []byte(original)
	di := 0
	for i, ch := range original {
		if ch >= '0' && ch <= '9' {
			if di < len(replacement) {
				out[i] = replacement[di]
				di++
			}
		}
	}
	return string(out)
}

// preserveDigitsLen returns a fake postal-code-ish string matching the
// digit count of the original (handles 5-digit US zips and others).
// If the original has no digits, returns the fallback.
func preserveDigitsLen(original, fallback string) string {
	hasDigits := false
	for _, ch := range original {
		if ch >= '0' && ch <= '9' {
			hasDigits = true
			break
		}
	}
	if !hasDigits {
		return fallback
	}
	out := []byte(original)
	for i, ch := range original {
		if ch >= '0' && ch <= '9' {
			out[i] = '0'
		} else if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' {
			out[i] = 'X'
		}
	}
	return string(out)
}

// fakeDOBPreservingFormat replaces a date with a same-format fake that
// keeps the year (so age-bucketing analysis on a masked stub stays
// roughly representative) but resets month/day to 01-01.
func fakeDOBPreservingFormat(original string) string {
	// Try YYYY-MM-DD, YYYY-MM, YYYY.
	if len(original) >= 4 {
		year := original[:4]
		if _, err := strconv.Atoi(year); err == nil {
			switch len(original) {
			case 4:
				return year
			case 7: // YYYY-MM
				return year + "-01"
			case 10: // YYYY-MM-DD
				return year + "-01-01"
			}
			// Longer (e.g. ISO datetime): preserve year + reset rest.
			if len(original) >= 10 && original[4] == '-' && original[7] == '-' {
				return year + "-01-01" + original[10:]
			}
		}
	}
	return "1970-01-01"
}
