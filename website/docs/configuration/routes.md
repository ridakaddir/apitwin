# Routes

[Home](/) > [Configuration](index) > Routes

---

## Route fields

| Field | Type | Default | Description |
|---|---|---|---|
| `method` | string | — | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS` |
| `match` | string | — | Path pattern (see below) |
| `enabled` | bool | `true` | Whether the route is active |
| `fallback` | string | — | Case to serve when no condition matches; omit to proxy |
| `conditions` | array | — | Ordered list of conditions (see [Conditions](../features/conditions.md)) |
| `cases` | map | — | Named response definitions (see [Cases](cases.md)) |

---

## Path matching

apitwin supports three path matching styles:

### Exact match

```toml
match = "/api/countries"
```

Matches only `GET /api/countries` exactly.

### Wildcard

`*` matches any segment(s):

```toml
match = "/api/countries/*"          # /api/countries/morocco, /api/countries/canada
match = "/api/*/cities"             # /api/countries/cities, /api/continents/cities
```

### Regex

Prefix with `~` to use a regular expression:

```toml
match = "~^/api/countries/[a-z]+$"   # /api/countries/morocco but not /api/countries/123
```

### Named parameters

Use `{name}` placeholders to extract values from URL segments:

```toml
match = "/api/countries/{countryId}"                       # extracts countryId
match = "/api/continents/{continentId}/countries/{countryId}"  # extracts both
```

Named parameters match exactly **one** path segment. They can be mixed with wildcards:

```toml
match = "/api/v1/*/regions/{regionId}/countries/{countryId}"
```

Extracted values are available for:
- [Dynamic file resolution](../features/dynamic-files.md) — `file = "stubs/countries/{path.countryId}.json"`
- [Persistence key resolution](../features/directory-stubs.md) — `key = "countryId"` uses the path value
- [Conditions](../features/conditions.md) — `source = "path"`, `field = "countryId"`

See [Named Parameters](../features/named-parameters.md) for full details.

---

## Route behaviour

### Fallback

When `fallback` is set, apitwin serves that case when no [condition](../features/conditions.md) matches:

```toml
fallback = "success"    # serves the "success" case
```

When `fallback` is **omitted**, unmatched requests are forwarded to `--target` (proxy mode).

### Enabled

Set `enabled = false` to temporarily disable a route without deleting it. Disabled routes are skipped during matching.

### Match order

Routes are evaluated in the order they appear in the config file (or alphabetical order across files in directory config). The first matching route wins.

---

**See also:** [Cases](cases.md) | [Conditions](../features/conditions.md) | [Named Parameters](../features/named-parameters.md)
