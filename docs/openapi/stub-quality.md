# Stub Quality

---

When generating from an OpenAPI spec, stubs are populated using the best available data from the spec.

## Priority order

| Priority | Source | Description |
|---|---|---|
| 1 | Spec `examples` | Used verbatim when present in the spec |
| 2 | Schema `example` / `default` / `enum` | First value used |
| 3 | Schema synthesis | Objects built from properties, arrays of one item, strings with format hints |

---

## Format hints

When synthesising stubs from schema definitions, apitwin uses format hints to produce realistic placeholder values:

| Schema format | Synthesised value |
|---|---|
| `uuid` | `"{{uuid}}"` — rendered as a real UUID at request time |
| `date-time` | `"{{now}}"` — rendered as RFC 3339 timestamp at request time |
| `date` | `"2026-01-01"` |
| `email` | `"user@example.com"` |
| `uri` | `"https://example.com"` |

---

## Tips for better stubs

1. **Add `examples` to your spec** — they are used verbatim and produce the highest quality stubs
2. **Use `format` hints** — `uuid`, `date-time`, `email`, etc. produce realistic values
3. **Define `default` or `enum` values** — the first value is used in the generated stub
4. **Fully define schemas** — the more properties and types defined, the better the synthesis

---

**See also:** [Generate Command](generate.md) | [Template Tokens](../features/template-tokens.md)
