# Cases

---

Cases are named response definitions attached to a route. Each route can have multiple cases — one is selected at request time based on [conditions](../features/conditions.md), [transitions](../features/response-transitions.md), or the route's `fallback`.

## Case fields

| Field | Type | Default | Description |
|---|---|---|---|
| `status` | int | `200` | HTTP status code (or gRPC status code for `[[grpc_routes]]`) |
| `json` | string | — | Inline JSON body (supports [template tokens](../features/template-tokens.md)) |
| `file` | string | — | Stub file path (supports [dynamic resolution](../features/dynamic-files.md)) |
| `delay` | int | `0` | Seconds to wait before responding |
| `persist` | bool | `false` | Mutate the stub file/directory on disk |
| `merge` | string | — | `update`, `append`, or `delete` (requires `persist: true`) |
| `key` | string | — | Field name for filename when using `append` with directories (resolved from body, path params, or query — see [directory stubs](../features/directory-stubs.md#key-resolution-for-filenames)) |
| `defaults` | string | — | JSON file with default values for `append`/`update` operations |
| `source` | string | — | Dot-path into the request body; only that sub-object is persisted (e.g. `"city"` or `"config.endpoint"`) |
| `wrap` | string | — | Wrap response into `{"field": <content>}` — works with both directories and single files; for gRPC directories, auto-detected from proto if omitted |

---

## Response body

Provide the response body with **either** `json` (inline) or `file` (from disk) — not both.

### Inline JSON

```toml
[routes.cases.success]
status = 200
json   = '{"countries": [{"code": "morocco", "name": "Morocco"}, {"code": "canada", "name": "Canada"}]}'
```

Inline JSON supports [template tokens](../features/template-tokens.md):

```toml
[routes.cases.created]
status = 201
json   = '{"code": "{{uuid}}", "created_at": "{{now}}"}'
```

### File-based response

```toml
[routes.cases.success]
status = 200
file   = "stubs/countries.json"
```

File paths support [dynamic resolution](../features/dynamic-files.md):

```toml
[routes.cases.country_profile]
status = 200
file   = "stubs/countries/{path.countryId}.json"
```

### Directory aggregation

When `file` points to a directory (trailing `/`), apitwin aggregates all `.json` files into an array:

```toml
[routes.cases.list]
file = "stubs/countries/"    # returns array of all files in stubs/countries/
```

For gRPC routes, the array is automatically wrapped into the response message's `repeated` field (e.g. `{"countries": [...]}`). Use the `wrap` field to override auto-detection or to wrap HTTP responses:

```toml
[grpc_routes.cases.list]
file = "stubs/countries/"
wrap = "countries"           # wraps into {"countries": [...]}
```

### Response wrapping (single files)

The `wrap` field also works with individual files. The file stores a flat object, but the response wraps it in a named field:

```toml
# File contains {"code": "morocco", ...} → response: {"country": {"code": "morocco", ...}}
[routes.cases.get]
file = "stubs/countries/{path.code}.json"
wrap = "country"
```

This works for both HTTP and gRPC routes. Wrapping is also applied to `persist` responses (`append` and `update`), so the file on disk stays flat while the API response is wrapped.

---

## Delay

Simulate network latency with `delay` (in seconds):

```toml
[routes.cases.slow_response]
status = 200
json   = '{"data": "slow"}'
delay  = 3
```

---

## Persistence

When `persist: true`, the request body is written to disk. Used with `merge` to define the operation:

| Merge | Operation | Description |
|---|---|---|
| `append` | Create | Writes a new file to a directory |
| `update` | Update | Shallow-merges request body into an existing file |
| `delete` | Delete | Removes a file from disk |

```toml
# Create
[routes.cases.created]
status  = 201
file    = "stubs/countries/"
persist = true
merge   = "append"
key     = "code"

# Update
[routes.cases.updated]
file    = "stubs/countries/{path.countryId}.json"
persist = true
merge   = "update"

# Delete
[routes.cases.deleted]
status  = 204
file    = "stubs/countries/{path.countryId}.json"
persist = true
merge   = "delete"
```

See [Directory-Based Stubs](../features/directory-stubs.md) for the full persistence guide.

### Source extraction

When the request wraps the entity inside a nested field (common in gRPC), use `source` to extract only that sub-object before merging. Without it, routing and metadata fields leak into the stub file.

```toml
# Request body: {"continentCode":"africa", "regionId":"north-africa", "city":{"name":"marrakech","elevation":"470m"}}
# Only the city sub-object is persisted — continentCode and regionId are excluded.
[grpc_routes.cases.updated]
file    = "stubs/cities/{body.city.name}.json"
persist = true
merge   = "update"
source  = "city"
wrap    = "city"
```

`source` accepts a dot-path for deeply nested objects:

```toml
source = "terrain.summit"    # extracts body.terrain.summit
```

For gRPC routes, when `source` is omitted apitwin auto-derives the wrapper field from the proto descriptor if the request follows the Google `Update<X>Request { X x = N }` convention (input has a non-repeated field whose message type equals the response output type). This makes `merge = "update"` cases persist cleanly without an explicit `source` for Google-style protos. Explicit `source` always wins. See [gRPC persistence — auto-derived source](../grpc/persistence.md#auto-derived-source-google-api-convention).

See [`examples/grpc-source-persist/`](https://github.com/ridakaddir/apitwin/tree/main/examples/grpc-source-persist/) for a complete working example.

### Defaults

Enrich `append`/`update` operations with server-generated fields:

```toml
[routes.cases.created]
status   = 201
file     = "stubs/countries/"
persist  = true
merge    = "append"
key      = "code"
defaults = "stubs/defaults/country.json"
```

The defaults file is deep-merged under the request body — body values always win on conflicts. Template tokens in the defaults file are resolved before merging.

---

**See also:** [Routes](routes.md) | [Directory-Based Stubs](../features/directory-stubs.md) | [Template Tokens](../features/template-tokens.md)
