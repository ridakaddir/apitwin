# gRPC Stubs & Conditions

[Home](/) > [gRPC](index) > Stubs & Conditions

---

## Stub file format

Stub files and inline `json = "..."` values use [protojson](https://protobuf.dev/programming-guides/proto3/#json) — JSON with field names matching the proto field names (camelCase by default):

```json
{
  "countryCode": "morocco",
  "name": "Morocco",
  "capital": "Rabat",
  "continent": "africa"
}
```

[Template tokens](../features/template-tokens.md) work in gRPC stubs exactly as they do in HTTP:

```toml
[grpc_routes.cases.ok]
status = 0
json   = '{"countryId": "{{uuid}}", "createdAt": "{{now}}"}'
```

---

## Conditions

Conditions evaluate fields from the decoded request message. Use `source = "body"` and dot-notation field paths. Both the proto field name (`country_code`) and its camelCase equivalent (`countryCode`) are accepted automatically:

```toml
[[grpc_routes]]
match    = "/geo.CountryService/GetCountryInfo"
enabled  = true
fallback = "ok"

  [[grpc_routes.conditions]]
  source = "body"
  field  = "country_code"     # snake_case or camelCase both work
  op     = "eq"
  value  = "morocco"
  case   = "morocco_info"

  [[grpc_routes.conditions]]
  source = "body"
  field  = "geography.continent"  # nested field, dot-notation
  op     = "eq"
  value  = "africa"
  case   = "african_country"

  [grpc_routes.cases.ok]
  status = 0
  file   = "stubs/country_default.json"

  [grpc_routes.cases.morocco_info]
  status = 0
  json   = '{"name": "Morocco", "capital": "Rabat", "languages": ["Arabic", "Berber", "French"]}'

  [grpc_routes.cases.african_country]
  status = 0
  file   = "stubs/african_country.json"
```

All condition operators work: `eq`, `neq`, `contains`, `regex`, `exists`, `not_exists`.

> **Note:** `source = "query"` and `source = "header"` are not applicable to gRPC and are ignored.

See [Conditions](../features/conditions.md) for the full operator reference.

---

## Proxy fallthrough

When a gRPC route has no matching condition and no `fallback`, apitwin forwards the call to `--grpc-target`. This lets you stub only the methods you care about:

```toml
# ListCities is mocked for Morocco only; all other countries are proxied
[[grpc_routes]]
match   = "/geo.CityService/ListCities"
enabled = true
# No fallback — unmatched conditions go to --grpc-target

  [[grpc_routes.conditions]]
  source = "body"
  field  = "country_code"
  op     = "eq"
  value  = "morocco"
  case   = "moroccan_cities"

  [grpc_routes.cases.moroccan_cities]
  status = 0
  file   = "stubs/cities_morocco.json"

# GetPopulation is not defined at all — always proxied to --grpc-target
```

When no `--grpc-target` is set, unmatched methods return gRPC `UNIMPLEMENTED`.

---

**See also:** [Configuration](config.md) | [Persistence](persistence.md) | [Conditions](../features/conditions.md)
