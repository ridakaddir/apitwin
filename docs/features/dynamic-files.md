# Dynamic File Resolution

---

Use `{source.field}` placeholders in `file` paths to resolve stub filenames from request data at runtime.

## Syntax

```toml
[routes.cases.country_cities]
status = 200
file   = "stubs/cities-{query.country}.json"
```

A request to `GET /api/cities?country=morocco` resolves to `stubs/cities-morocco.json`.

---

## Placeholder sources

| Placeholder | Resolves from | Example |
|---|---|---|
| `{query.fieldName}` | URL query parameter | `?country=morocco` |
| `{body.fieldName}` | JSON request body field | `{"geography": {"region": "north-africa"}}` |
| `{header.HeaderName}` | Request header | `X-Country: morocco` |
| `{path.paramName}` | Named path parameter | `/countries/{countryId}` |

### Examples

```toml
# From query parameter
file = "stubs/cities-{query.country}.json"
# ?country=morocco → stubs/cities-morocco.json

# From body field (dot-notation for nested)
file = "stubs/region-{body.geography.region}.json"
# {"geography": {"region": "north-africa"}} → stubs/region-north-africa.json

# From header
file = "stubs/continent-{header.X-Continent}.json"
# X-Continent: africa → stubs/continent-africa.json

# From named path parameter
file = "stubs/countries/{path.countryId}.json"
# /api/countries/morocco → stubs/countries/morocco.json
```

---

## Fallback behaviour

If the resolved file does not exist, apitwin falls through to the next condition or `fallback` — no 500 error. This lets you serve specific stubs for known values and a generic response for everything else.

---

## Security

Dynamic file resolution includes built-in protections against malicious input:

- **Path traversal prevention** — `.` and `..` patterns are neutralised
- **Hidden file protection** — leading dots in filenames are replaced
- **Character sanitisation** — unsafe characters are removed from file paths

---

**See also:** [Named Parameters](named-parameters.md) | [Cases](../configuration/cases.md) | [Directory-Based Stubs](directory-stubs.md)
