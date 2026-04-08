# Directory-Based Stub Storage

[Home](../README.md) > [Features](README.md) > Directory-Based Stubs

---

When `persist: true`, mutating requests operate on individual JSON files stored in directories. Each resource is a separate file, enabling a "single source of truth" convention.

## How it works

### Directory structure

```
stubs/
└── countries/
    ├── morocco.json     # {"code": "morocco", "name": "Morocco", "continent": "africa", ...}
    ├── germany.json     # {"code": "germany", "name": "Germany", "continent": "europe", ...}
    ├── japan.json       # {"code": "japan", "name": "Japan", "continent": "asia", ...}
    └── canada.json      # {"code": "canada", "name": "Canada", "continent": "north-america", ...}
```

### API operations

| HTTP Method | Operation | Behaviour |
|---|---|---|
| `GET` (list) | Aggregates all `.json` files in the directory into an array |
| `GET` (detail) | Reads a single file by ID |
| `POST` | Creates a new file (auto-generates UUID if ID is missing) |
| `PATCH`/`PUT` | Shallow-merges request body into an existing file |
| `DELETE` | Removes the file from disk |

---

## Configuration

### Create — `append`

```toml
[[routes]]
method   = "POST"
match    = "/api/countries"
enabled  = true
fallback = "created"

  [routes.cases.created]
  status  = 201
  file    = "stubs/countries/"       # Directory path (trailing /)
  persist = true
  merge   = "append"
  key     = "code"                   # Field used as filename
```

### Read — list (directory aggregation)

```toml
[[routes]]
method   = "GET"
match    = "/api/countries"
enabled  = true
fallback = "list"

  [routes.cases.list]
  file = "stubs/countries/"          # Returns array of all .json files
```

### Read — single file

```toml
[[routes]]
method   = "GET"
match    = "/api/countries/{countryId}"
enabled  = true
fallback = "country"

  [routes.cases.country]
  file = "stubs/countries/{path.countryId}.json"    # Dynamic filename from path
```

### Update — `update`

```toml
[[routes]]
method   = "PATCH"
match    = "/api/countries/{countryId}"
enabled  = true
fallback = "updated"

  [routes.cases.updated]
  file    = "stubs/countries/{path.countryId}.json"
  persist = true
  merge   = "update"             # Shallow merge into existing file
```

### Delete — `delete`

```toml
[[routes]]
method   = "DELETE"
match    = "/api/countries/{countryId}"
enabled  = true
fallback = "deleted"

  [routes.cases.deleted]
  status  = 204
  file    = "stubs/countries/{path.countryId}.json"
  persist = true
  merge   = "delete"             # Remove file from disk
```

---

## Key resolution for filenames

When `merge = "append"`, the `key` field determines the filename. apitwin resolves the value using this fallback chain:

1. **Request body** — if the body contains the `key` field, that value is used as the filename
2. **Defaults** — if a `defaults` file provides the `key` field (e.g. via `{{uuid}}`), that value is used
3. **Named path parameters** — if the route uses `{paramName}` and the param name matches `key`, the URL value is used
4. **Path wildcards** — for patterns containing a single `*`, the matched segment from the URL path is used as the value
5. **Query parameters** — URL query parameter matching the `key` name
6. **Auto-generated UUID** — if none of the above provide a value

> **Note:** Defaults are deep-merged into the request body *before* key resolution runs. A `defaults` file that sets the `key` field will therefore take precedence over path, wildcard, and query fallbacks, but will still be overridden by an explicit `key` in the request body. Wildcard-based key extraction is only reliable for routes with a single `*` in the pattern.

### Example: key from the request body

```sh
curl -X POST localhost:4000/api/countries -d '{"code": "morocco", "name": "Morocco"}'
# Creates file: stubs/countries/morocco.json
```

### Example: key from a named path parameter

```toml
[[routes]]
method   = "POST"
match    = "/api/continents/{continentId}/countries"
fallback = "created"

  [routes.cases.created]
  status  = 201
  file    = "stubs/countries/"
  persist = true
  merge   = "append"
  key     = "continentId"
```

```sh
curl -X POST localhost:4000/api/continents/africa/countries -d '{"name": "Tunisia"}'
# Creates file: stubs/countries/africa.json
# The continentId is extracted from the URL and injected into the saved record
```

### Example: auto-generated UUID fallback

```sh
# POST without code and no matching path parameter
curl -X POST localhost:4000/api/countries -d '{"name": "New Country"}'

# Creates file: stubs/countries/123e4567-e89b-12d3-a456-426614174000.json
# Response: {"code": "123e4567-...", "name": "New Country"}
```

---

## Defaults

When a `POST` creates a resource, the client typically sends only a subset of fields. The `defaults` field lets you enrich the response with server-generated values.

**Defaults file** (`stubs/defaults/country.json`):

```json
{
  "code": "{{uuid}}",
  "status": "active",
  "verified": false,
  "createdAt": "{{now}}"
}
```

**Config:**

```toml
[routes.cases.created]
status   = 201
file     = "stubs/countries/"
persist  = true
merge    = "append"
key      = "code"
defaults = "stubs/defaults/country.json"
```

**How it works:**

1. apitwin reads the defaults file and resolves [template tokens](template-tokens.md) (`{{uuid}}` becomes a real UUID, `{{now}}` becomes a timestamp)
2. Deep-merges: defaults as the base, request body overlaid on top — **body always wins on conflicts**
3. The merged result is saved to disk and returned as the response

```sh
# POST with just name and continent
curl -X POST localhost:4000/api/countries -d '{"name": "Morocco", "continent": "africa"}'

# Response (and saved file) includes defaults:
# {
#   "code": "a1b2c3d4-...",
#   "name": "Morocco",
#   "continent": "africa",
#   "status": "active",
#   "verified": false,
#   "createdAt": "2026-03-26T10:30:00Z"
# }
```

**Works with `update` too:**

```toml
[routes.cases.updated]
file     = "stubs/countries/{path.countryId}.json"
persist  = true
merge    = "update"
defaults = "stubs/defaults/country-update.json"
```

**Dynamic defaults path:**

```toml
defaults = "stubs/defaults/{path.continent}.json"
```

**Error handling:** If the defaults file is missing or contains invalid JSON, apitwin logs a warning and proceeds with the original request body.

---

## Nested subdirectories

Support sub-resources with nested directories:

```
stubs/
├── continents/
│   ├── africa/
│   │   ├── morocco.json
│   │   └── tunisia.json
│   └── europe/
│       └── germany.json
```

```toml
# GET /continents/{continentId}/countries — list
[routes.cases.list_countries]
file = "stubs/continents/{path.continentId}/"

# POST /continents/{continentId}/countries — create
[routes.cases.create_country]
file     = "stubs/continents/{path.continentId}/"
persist  = true
merge    = "append"
key      = "code"
defaults = "stubs/defaults/country.json"
```

---

## Response wrapping

Use the `wrap` field to wrap responses in a named key. Files on disk stay flat — wrapping is applied only in the response.

```toml
# GET /api/countries/:code → {"country": {"code": "morocco", "name": "Morocco", ...}}
[routes.cases.get]
file = "stubs/countries/{path.code}.json"
wrap = "country"

# GET /api/countries → {"countries": [{...}, ...]}
[routes.cases.list]
file = "stubs/countries/"
wrap = "countries"
```

Wrapping also applies to `persist` responses (`append` and `update`), so create and update operations return the wrapped shape while the file stays flat.

See [`examples/wrap-test/`](../../examples/wrap-test/) for a complete HTTP example and [`examples/grpc-wrap/`](../../examples/grpc-wrap/) for gRPC.

---

## Benefits

- **Single source of truth** — each resource is one file
- **Version control friendly** — clean diffs per resource
- **No size limits** — unlimited scalability vs. single-file arrays
- **Intuitive structure** — file layout mirrors API structure
- **Atomic operations** — each resource operation is independent

---

## Example

See [`examples/directory-stubs/`](../../examples/directory-stubs/) for a complete working example with country listing, creation, retrieval, updates, and deletion.

---

**See also:** [Cases](../configuration/cases.md) | [Dynamic File Resolution](dynamic-files.md) | [Named Parameters](named-parameters.md) | [Template Tokens](template-tokens.md)
