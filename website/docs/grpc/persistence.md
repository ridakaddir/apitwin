# gRPC Persistence

[Home](/) > [gRPC](index) > Persistence

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

All persist operations return an empty proto response (`{}`). The gRPC status code signals success or failure.

---

## Response wrapping

Use the `wrap` field to wrap persist responses. The file on disk stays flat, but the API response is wrapped:

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

---

**See also:** [Directory-Based Stubs (HTTP)](../features/directory-stubs.md) | [Configuration](config.md) | [Stubs & Conditions](stubs.md)
