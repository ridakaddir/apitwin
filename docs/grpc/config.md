# gRPC Configuration

[Home](../README.md) > [gRPC](README.md) > Configuration

---

gRPC routes live in the same config files as HTTP routes. All existing features work: conditions, transitions, fallback, delay, and template tokens.

## Basic config

```toml
[[grpc_routes]]
match    = "/geo.CountryService/GetCountry"
enabled  = true
fallback = "ok"

  [grpc_routes.cases.ok]
  status = 0   # gRPC OK
  file   = "stubs/get_country.json"

  [grpc_routes.cases.not_found]
  status = 5   # gRPC NOT_FOUND
  json   = '{"message": "country not found"}'

  [grpc_routes.cases.error]
  status = 13  # gRPC INTERNAL
  json   = '{"message": "internal server error"}'
  delay  = 1
```

---

## `match` format

The `match` field is the full gRPC method path: `"/package.Service/Method"`. All three matching styles work:

```toml
match = "/geo.CountryService/GetCountry"   # exact
match = "/geo.CountryService/*"            # wildcard — all methods in the service
match = "~/geo\\..*Service/.*"             # regex (prefix with ~)
```

---

## gRPC status codes

`Case.status` is a [gRPC status code](https://grpc.github.io/grpc/core/md_doc_statuscodes.html) integer:

| Code | Name | Meaning |
|---|---|---|
| `0` | OK | Success (default when status is omitted) |
| `1` | CANCELLED | Request cancelled |
| `2` | UNKNOWN | Unknown error |
| `3` | INVALID_ARGUMENT | Bad input |
| `4` | DEADLINE_EXCEEDED | Timeout |
| `5` | NOT_FOUND | Resource not found |
| `6` | ALREADY_EXISTS | Resource already exists |
| `7` | PERMISSION_DENIED | Authorisation failure |
| `9` | FAILED_PRECONDITION | Operation rejected |
| `13` | INTERNAL | Server error |
| `14` | UNAVAILABLE | Service temporarily unavailable |
| `16` | UNAUTHENTICATED | Missing or invalid credentials |

---

## `{body.field}` interpolation

File paths in gRPC cases support `{body.field}` placeholders, resolved from the decoded request message. This enables per-resource file lookups without hardcoding paths:

```toml
[[grpc_routes]]
match    = "/geo.CountryService/GetCountry"
fallback = "ok"

  [grpc_routes.cases.ok]
  file = "stubs/countries/{body.country_code}.json"
```

Both snake_case (`country_code`) and camelCase (`countryCode`) field names are matched automatically. Characters unsafe for filenames are replaced with `_`.

---

## Directory wrapping

When `file` points to a directory (trailing `/`), apitwin aggregates all `.json` files into an array. For gRPC, this array is automatically wrapped into the correct response field.

**Auto-detection:** apitwin inspects the response message descriptor. If there is exactly one `repeated` field, the array is wrapped into that field:

```toml
# ListCountriesResponse has: repeated Country countries = 1;
# → auto-wraps into {"countries": [...]}
[[grpc_routes]]
match    = "/geo.CountryService/ListCountries"
fallback = "list"

  [grpc_routes.cases.list]
  file = "stubs/countries/"
```

**Explicit `wrap` field:** when auto-detection is ambiguous (multiple repeated fields) or you want to be explicit, use the `wrap` field:

```toml
  [grpc_routes.cases.list]
  file = "stubs/countries/"
  wrap = "countries"          # wraps array into {"countries": [...]}
```

The `wrap` field takes precedence over auto-detection when both are present.

> **Note:** For HTTP routes, directory aggregation returns a raw JSON array as before — `wrap` only applies to gRPC.

---

## Transitions

Time-based transitions work identically to HTTP. The gRPC route key is the `match` pattern:

```toml
[[grpc_routes]]
match    = "/geo.CountryService/GetVisaStatus"
enabled  = true
fallback = "submitted"

  [[grpc_routes.transitions]]
  case     = "submitted"
  duration = 10

  [[grpc_routes.transitions]]
  case     = "under_review"
  duration = 50

  [[grpc_routes.transitions]]
  case     = "approved"

  [grpc_routes.cases.submitted]
  status = 0
  json   = '{"status": "submitted"}'

  [grpc_routes.cases.under_review]
  status = 0
  json   = '{"status": "under_review"}'

  [grpc_routes.cases.approved]
  status = 0
  json   = '{"status": "approved"}'
```

See [Response Transitions](../features/response-transitions.md) for full details on timeline behaviour.

---

**See also:** [Stubs & Conditions](stubs.md) | [Persistence](persistence.md) | [Generation](generation.md)
