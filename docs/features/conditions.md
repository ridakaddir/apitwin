# Conditions

---

Conditions let you route requests to different response cases based on request data. They are evaluated top-to-bottom — the first passing condition activates its case. If none match, the route's `fallback` case is used.

## Example

```toml
[[routes]]
method   = "POST"
match    = "/api/countries"
enabled  = true
fallback = "default"

  [[routes.conditions]]
  source = "body"
  field  = "continent"
  op     = "eq"
  value  = "africa"
  case   = "african_country"

  [[routes.conditions]]
  source = "query"
  field  = "format"
  op     = "eq"
  value  = "brief"
  case   = "brief_response"

  [[routes.conditions]]
  source = "header"
  field  = "X-Language"
  op     = "eq"
  value  = "fr"
  case   = "french_response"

  [routes.cases.african_country]
  status = 200
  json   = '{"region": "Africa", "currency_zone": "varied"}'

  [routes.cases.brief_response]
  status = 200
  json   = '{"format": "brief"}'

  [routes.cases.french_response]
  status = 200
  json   = '{"langue": "francais"}'

  [routes.cases.default]
  status = 200
  json   = '{"region": "unknown"}'
```

---

## Condition fields

| Field | Type | Description |
|---|---|---|
| `source` | string | Where to look: `body`, `query`, `header`, or `path` |
| `field` | string | Field name (dot-notation for nested body fields) |
| `op` | string | Comparison operator (see below) |
| `value` | string | Value to compare against (not used with `exists`/`not_exists`) |
| `case` | string | Case key to activate when the condition passes |

---

## Sources

### `body`

Match on JSON request body fields. Supports dot-notation for nested fields:

```toml
[[routes.conditions]]
source = "body"
field  = "geography.coastline"
op     = "eq"
value  = "atlantic"
case   = "atlantic_coast"
```

### `query`

Match on URL query parameters:

```toml
[[routes.conditions]]
source = "query"
field  = "continent"
op     = "eq"
value  = "africa"
case   = "african_countries"
```

### `header`

Match on request headers:

```toml
[[routes.conditions]]
source = "header"
field  = "Authorization"
op     = "exists"
case   = "authenticated"
```

### `path`

Match on [named path parameters](named-parameters.md):

```toml
# Route: match = "/api/countries/{countryId}/cities"
[[routes.conditions]]
source = "path"
field  = "countryId"
op     = "eq"
value  = "morocco"
case   = "moroccan_cities"
```

> **Note:** `source = "query"` and `source = "header"` are not applicable to gRPC routes and are ignored.

---

## Operators

| Operator | Description | Example |
|---|---|---|
| `eq` | Exact match | `value = "africa"` |
| `neq` | Not equal | `value = "antarctica"` |
| `contains` | String contains | `value = "north"` |
| `regex` | Regular expression match | `value = "^[a-z]{2,3}$"` |
| `exists` | Field is present | (no `value` needed) |
| `not_exists` | Field is absent | (no `value` needed) |

---

## Evaluation order

1. Conditions are evaluated **top-to-bottom** in array order
2. The **first** passing condition activates its case
3. If **no condition** matches, the `fallback` case is served
4. If **no fallback** is set, the request is forwarded to `--target` (proxy mode)

---

## Multiple conditions example

```toml
[[routes]]
method   = "POST"
match    = "/api/cities"
enabled  = true
fallback = "default"

  # Check body field first
  [[routes.conditions]]
  source = "body"
  field  = "population"
  op     = "regex"
  value  = "^[0-9]{7,}$"
  case   = "megacity"

  # Then check query param
  [[routes.conditions]]
  source = "query"
  field  = "country"
  op     = "eq"
  value  = "morocco"
  case   = "moroccan_city"

  # Then check header
  [[routes.conditions]]
  source = "header"
  field  = "X-Priority"
  op     = "exists"
  case   = "priority_request"
```

---

**See also:** [Routes](../configuration/routes.md) | [Cases](../configuration/cases.md) | [Named Parameters](named-parameters.md)
