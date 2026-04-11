# gRPC Persistence

[Home](../README.md) > [gRPC](README.md) > Persistence

---

gRPC routes support the same directory-based persistence as HTTP routes. Each resource is stored as a separate JSON file.

## Full CRUD example

```toml
# Create country — append to directory
[[grpc_routes]]
match    = "/geo.CountryService/CreateCountry"
enabled  = true
fallback = "created"

  [grpc_routes.cases.created]
  status   = 0
  file     = "stubs/countries/"               # Directory path
  persist  = true
  merge    = "append"
  key      = "countryCode"                    # Field used as filename; auto-generated if missing
  defaults = "stubs/defaults/country.json"    # Server-generated fields ({{uuid}}, {{now}})

# List countries — directory aggregation (auto-wraps into repeated field)
[[grpc_routes]]
match    = "/geo.CountryService/ListCountries"
enabled  = true
fallback = "list"

  [grpc_routes.cases.list]
  file = "stubs/countries/"                   # Auto-wraps into {"countries": [...]}

# Get country — single file read
[[grpc_routes]]
match    = "/geo.CountryService/GetCountry"
enabled  = true
fallback = "country"

  [grpc_routes.cases.country]
  file = "stubs/countries/{body.countryCode}.json"  # Dynamic filename from request

# Update country — shallow merge into existing file
[[grpc_routes]]
match    = "/geo.CountryService/UpdateCountry"
enabled  = true
fallback = "updated"

  [grpc_routes.cases.updated]
  status  = 0
  file    = "stubs/countries/{body.countryCode}.json"
  persist = true
  merge   = "update"

# Delete country — remove file
[[grpc_routes]]
match    = "/geo.CountryService/DeleteCountry"
enabled  = true
fallback = "deleted"

  [grpc_routes.cases.deleted]
  status  = 0
  file    = "stubs/countries/{body.countryCode}.json"
  persist = true
  merge   = "delete"
```

---

## Field name mapping

Both snake_case (`country_code`) and camelCase (`countryCode`) field names in protobuf requests are matched against `key` automatically.

---

## Error codes

| Situation | gRPC Code |
|---|---|
| File/record not found | `5` NOT_FOUND |
| Directory required for append | `3` INVALID_ARGUMENT |
| File read/write error | `13` INTERNAL |

---

## Response body

Persist operations return the updated or created data in the response. For `update` and `append`, the response contains the persisted object. For `delete`, an empty response (`{}`) is returned. The gRPC status code signals success or failure.

---

## Source extraction

When a gRPC request wraps the entity inside a nested field (e.g. `UpdateCityRequest` has top-level routing fields plus a nested `City city`), use `source` to extract only the sub-object before merging. Without it, all top-level fields — including routing metadata — leak into the stub file.

```toml
[grpc_routes.cases.updated]
file    = "stubs/cities/{body.city.name}.json"
persist = true
merge   = "update"
source  = "city"           # only body.city is persisted
wrap    = "city"           # response: {"city": {...}}
```

`source` accepts a dot-path for deeply nested objects:

```toml
source = "terrain.summit"   # extracts body.terrain.summit
```

**Without source**, the entire request body is merged into the stub — routing fields like `continentCode`, `regionId` leak in, and the nested object is added as-is rather than being unwrapped for the merge.

### Auto-derived source (Google API convention)

When `source` is omitted, apitwin inspects the proto descriptor and **automatically extracts a wrapper field if the request follows the Google `Update<X>Request { X x = N }` convention** — i.e. the request input message has a non-repeated field whose message type equals the response output type.

Given:

```proto
service DatabaseService {
  rpc UpdateDatabaseInstance (UpdateDatabaseInstanceRequest) returns (DatabaseInstance);
}

message UpdateDatabaseInstanceRequest {
  string           name              = 1;
  DatabaseInstance database_instance = 2;   // ← matches response type → auto-extracted
}
```

…the following case persists cleanly without an explicit `source`:

```toml
[grpc_routes.cases.updated]
file    = "stubs/instances/{body.databaseInstance.id}.json"
persist = true
merge   = "update"
# source = "databaseInstance"   # inferred from proto descriptor
```

The auto-derive only fires when the input has **exactly one** matching wrapper field. The match is on the response message's fully-qualified type name, so methods that return wrapper envelopes like `google.protobuf.Empty` or `google.longrunning.Operation` are unaffected unless the request happens to carry a field of that exact type. Ambiguous matches (two or more input fields whose type equals the response type) are skipped, and apitwin logs a one-time warning per method so you can disambiguate by setting `source` explicitly. An explicit `source` always wins. For protos that don't follow the convention (e.g. `UpdateCountryRequest { string code = 1; string name = 2; }`), the behaviour is unchanged — there is no wrapper to extract.

See [`examples/grpc-source-persist/`](../../examples/grpc-source-persist/) for a complete working example with four test scenarios.

---

## Response wrapping

Use the `wrap` field to match the proto response message shape. The file on disk stays flat, but the response is wrapped:

```toml
  [grpc_routes.cases.country]
  file = "stubs/countries/{body.countryCode}.json"
  wrap = "country"              # response: {"country": {...}}
```

See [Configuration — Response wrapping](config.md#single-file-wrapping) for details.

---

## Example

See [`examples/grpc-directory-persist/`](../../examples/grpc-directory-persist/) for a complete working example.
See [`examples/grpc-wrap/`](../../examples/grpc-wrap/) for an example with response wrapping.
See [`examples/grpc-source-persist/`](../../examples/grpc-source-persist/) for an example with source extraction.

---

**See also:** [Directory-Based Stubs (HTTP)](../features/directory-stubs.md) | [Configuration](config.md) | [Stubs & Conditions](stubs.md)
