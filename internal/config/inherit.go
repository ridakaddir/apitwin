package config

// ResolveCase looks up caseName in cases and returns it with sparse fields
// inherited from the route's fallback case. The inherited fields are those
// that describe the route's *response shape* (Wrap, Source, Key, ArrayKey,
// Defaults, Merge, File) — not the case's intent (Persist, Status, JSON,
// Delay) or its mutation spec (Primary, Cascade).
//
// Why inheritance: transition cases are typically authored as a thin overlay
// on top of the fallback (e.g. a `ready` case that just sets `defaults` to
// flip the entity's status to STATUS_READY). When such a sparse case is
// resolved at request time after the transition window elapses, it would
// otherwise lose the route's response shape and corrupt the persisted stub
// by writing the full request envelope into a flat entity file. Inheriting
// the fallback's wrap/source means the sparse case "just works" as a
// request handler.
//
// Why some fields are not inherited:
//   - Persist: a hard intent flag — inheriting it would silently turn a
//     read-only case into a mutator, which is dangerous.
//   - Status / JSON / Delay: per-case shape choices the user makes
//     deliberately (a transition case should be free to return a different
//     status code or inline body without inheriting the fallback's).
//   - Primary / Cascade: independent mutation specs, not response shape.
//
// When caseName is empty, equals fallback, fallback is empty, or the
// fallback case does not exist in cases, the case is returned unchanged
// (no inheritance applies). When caseName itself is not present in cases,
// returns (Case{}, false) — the caller should treat this as "case not
// found".
func ResolveCase(cases map[string]Case, caseName, fallback string) (Case, bool) {
	c, ok := cases[caseName]
	if !ok {
		return Case{}, false
	}
	if fallback == "" || caseName == fallback {
		return c, true
	}
	fb, ok := cases[fallback]
	if !ok {
		return c, true
	}
	// Wrap and Source describe the route's response shape; they always
	// apply, even to inline-JSON cases that wrap the content.
	if c.Wrap == "" {
		c.Wrap = fb.Wrap
	}
	if c.Source == "" {
		c.Source = fb.Source
	}
	// File/Merge/Key/ArrayKey/Defaults only make sense for file-backed
	// cases. When the case sets JSON inline, it is not a persisting,
	// file-backed case — inheriting these would either turn it into one
	// (wrong) or override loadStub's content-source precedence (also
	// wrong). Skip them.
	if c.JSON == "" {
		if c.Key == "" {
			c.Key = fb.Key
		}
		if c.ArrayKey == "" {
			c.ArrayKey = fb.ArrayKey
		}
		if c.Defaults == "" {
			c.Defaults = fb.Defaults
		}
		if c.Merge == "" {
			c.Merge = fb.Merge
		}
		if c.File == "" {
			c.File = fb.File
		}
	}
	return c, true
}
