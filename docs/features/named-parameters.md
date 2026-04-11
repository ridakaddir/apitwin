# Named Path Parameters

---

apitwin supports `{name}` placeholders in route patterns to extract values from URL paths. Extracted values can be used for key resolution, conditions, and dynamic file paths.

## Syntax

Use curly braces to define named parameters that match exactly one path segment:

```toml
[[routes]]
method   = "GET"
match    = "/api/countries/{countryId}"
enabled  = true
fallback = "success"
```

A request to `GET /api/countries/morocco` extracts `countryId = "morocco"`.

---

## Multiple parameters

Extract several values from a single route:

```toml
[[routes]]
method   = "GET"
match    = "/api/continents/{continentId}/countries/{countryId}"
enabled  = true
fallback = "country_detail"
```

`GET /api/continents/africa/countries/morocco` extracts `continentId = "africa"` and `countryId = "morocco"`.

---

## Mixed patterns

Named parameters can coexist with wildcard `*` patterns:

```toml
[[routes]]
method   = "GET"
match    = "/api/v1/*/regions/{regionId}/countries/{countryId}"
enabled  = true
fallback = "success"
```

---

## Dynamic file resolution

Use `{path.paramName}` in file paths to serve resource-specific stub files:

```toml
[[routes]]
method   = "GET"
match    = "/api/countries/{countryId}/profile"
enabled  = true
fallback = "country_profile"

  [routes.cases.country_profile]
  status = 200
  file   = "stubs/countries/{path.countryId}.json"
```

**Request:** `GET /api/countries/morocco/profile`
**Resolves to:** `stubs/countries/morocco.json`

Nested directory structures work too:

```toml
file = "stubs/continents/{path.continentId}/countries/{path.countryId}.json"
```

---

## Persistence with named parameters

Named path parameters work as a fallback for key resolution in `merge = "append"` operations. When the request body doesn't contain the `key` field, apitwin resolves it from the URL path:

```toml
[[routes]]
method   = "POST"
match    = "/api/continents/{continentId}/countries/{countryId}/cities"
enabled  = true
fallback = "created"

  [routes.cases.created]
  status   = 201
  file     = "stubs/cities/"
  persist  = true
  merge    = "append"
  key      = "countryId"    # resolved from {countryId} in the URL path
```

A `POST` to `/api/continents/africa/countries/morocco/cities` with body `{"name": "Marrakech"}` creates `stubs/cities/morocco.json` — the `countryId` value is extracted from the path and used as both the filename and injected into the saved record.

### Key resolution priority

When using `merge = "append"`, the filename is determined by the `key` field value, resolved in this order:

1. **Request body** — if the body contains the `key` field, that value is used
2. **Defaults** — if a `defaults` file provides the `key` field (e.g. via `{{uuid}}`), that value is used
3. **Named path parameters** — `{countryId}`, `{continentId}` from the URL path
4. **Path wildcards (single `*`)** — for patterns with a single `*`, the first wildcard match may be used as the key; patterns with multiple `*` are not guaranteed to produce a stable filename key
5. **Query parameters** — URL query parameters
6. **Auto-generated UUID** — if none of the above provide a value

> **Note:** Defaults are deep-merged into the request body before key resolution runs. If your `defaults` file sets the `key` field (e.g. `"countryId": "{{uuid}}"`), that value will be used as the filename even when a named path parameter could provide one. To use path parameter keys with defaults, ensure the defaults file does **not** include the `key` field.

---

## Conditions with named parameters

Use `source = "path"` to match on extracted values:

```toml
[[routes]]
method   = "GET"
match    = "/api/countries/{countryId}/cities"
enabled  = true
fallback = "default"

  [[routes.conditions]]
  source = "path"
  field  = "countryId"
  op     = "eq"
  value  = "morocco"
  case   = "moroccan_cities"

  [routes.cases.moroccan_cities]
  status = 200
  file   = "stubs/moroccan-cities.json"

  [routes.cases.default]
  status = 200
  file   = "stubs/generic-cities.json"
```

---

## Security

Named parameter file resolution includes built-in protections:

- **Path traversal prevention** — `.` and `..` patterns are neutralised
- **Hidden file protection** — leading dots in filenames are replaced
- **Character sanitisation** — unsafe characters are removed from file paths

---

## Backward compatibility

Named path parameters are **100% backward compatible**:

- All existing route patterns continue to work unchanged
- Existing wildcard `*` behaviour is preserved
- No breaking changes to configuration format
- Routes without named parameters use existing fast paths

---

**See also:** [Dynamic File Resolution](dynamic-files.md) | [Directory-Based Stubs](directory-stubs.md) | [Conditions](conditions.md)
