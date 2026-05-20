# Payload validation

Apitwin can reject malformed request payloads before they reach your stub
responses. Rules are declared in TOML on each route and apply identically
to REST and gRPC — the same operator vocabulary, the same failure shape,
mapped onto each transport's idioms (HTTP 400 vs `INVALID_ARGUMENT`).

## Rule shape

Every rule is a single TOML table with up to four fields:

```toml
[[routes.validation]]
field   = "country_code"          # dot-notation path into the request body
op      = "pattern"               # operator name (see table below)
value   = "^[A-Z]{2}$"            # comparand — parsed per-op
message = "must be ISO 3166-1"    # optional override for the failure text
```

`field` walks the JSON / proto request payload using dots. Nested objects
work: `city.country.continent` reads
`{"city": {"country": {"continent": "asia"}}}`.

`value` is always a string in TOML; operators parse it as the right type
(`gte` parses it as a float, `min_len` as an int, `in` splits on commas,
`pattern` compiles it as a regexp). Bad comparands are caught at config
load time — the server refuses to start if `pattern` does not compile or
`gte` does not parse.

`message`, when set, replaces the auto-generated failure text shown to
the client. Useful when the default ("country_code must match pattern
^[A-Z]{2}$") is less helpful than a domain-specific hint.

## Operator reference

| Op | Applies to | buf.validate equivalent | OpenAPI equivalent |
|---|---|---|---|
| `required` | any | `(buf.validate.field).required = true` | `required: [...]` |
| `forbidden` | any | — | — |
| `type` | any | (implicit from proto type) | `type: string \| number \| integer \| boolean \| array \| object` |
| `const` | scalar | `*.const` | `const` |
| `in` | scalar | `*.in = [...]` | `enum: [...]` |
| `not_in` | scalar | `*.not_in = [...]` | — |
| `gt`, `gte`, `lt`, `lte` | number | `int32/int64/float/double.{gt,gte,lt,lte}` | `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum` |
| `min_len`, `max_len` | string | `string.min_len`, `string.max_len` | `minLength`, `maxLength` |
| `pattern` | string | `string.pattern` | `pattern` |
| `prefix`, `suffix`, `contains` | string | `string.prefix`, `string.suffix`, `string.contains` | — |
| `email`, `uri`, `uuid`, `ipv4`, `ipv6`, `hostname` | string | `string.email`, `string.uri`, `string.uuid`, `string.ip` | `format: email \| uri \| uuid \| ipv4 \| ipv6 \| hostname` |
| `min_items`, `max_items`, `unique` | array | `repeated.min_items`, `repeated.max_items`, `repeated.unique` | `minItems`, `maxItems`, `uniqueItems` |

Rules whose target field is absent **pass silently** — only `required`
and `forbidden` enforce presence. This matches buf.validate semantics:
missing optional fields skip rule evaluation.

## Where rules live

`[[routes.validation]]` for REST, `[[grpc_routes.validation]]` for gRPC.
Multiple rules on the same field are independent (all must pass).

### REST example

```toml
[[routes]]
method   = "POST"
match    = "/cities"
fallback = "created"

  [[routes.validation]]
  field = "name"
  op    = "required"

  [[routes.validation]]
  field = "country_code"
  op    = "pattern"
  value = "^[A-Z]{2}$"

  [[routes.validation]]
  field = "population"
  op    = "gte"
  value = "0"

  [[routes.validation]]
  field = "continent"
  op    = "in"
  value = "africa,europe,asia,americas,oceania,antarctica"

  [routes.cases.created]
  status = 201
  file   = "stubs/city.json"
```

### gRPC example

```toml
[[grpc_routes]]
match    = "/cities.Cities/CreateCity"
fallback = "created"

  [[grpc_routes.validation]]
  field = "name"
  op    = "required"

  [[grpc_routes.validation]]
  field = "tags"
  op    = "min_items"
  value = "1"

  [grpc_routes.cases.created]
  status = 0
  file   = "stubs/city.json"
```

`field` paths in gRPC rules use the **proto field name** (snake_case),
matching what `protojson` produces when decoding the request.

## Failure responses

### REST — HTTP 400

```json
{
  "error": "validation failed",
  "violations": [
    { "field": "name",        "rule": "required", "message": "name is required" },
    { "field": "population",  "rule": "gte",      "message": "population must be >= 0" }
  ]
}
```

`Content-Type: application/json`. All violations from one request are
returned in a single response — the validator does not short-circuit.

### gRPC — INVALID_ARGUMENT (status code 3)

The status message joins every violation: `"name is required;
population must be >= 0"`. Per-field detail rides in a standard
`google.rpc.BadRequest` detail block, so canonical gRPC clients
(`grpcurl`, Buf Studio, generated stubs) render each violation against
the right field:

```
ERROR:
  Code: InvalidArgument
  Message: name is required; population must be >= 0
  Details:
    1) {
         "@type": "type.googleapis.com/google.rpc.BadRequest",
         "fieldViolations": [
           { "field": "name",       "description": "name is required" },
           { "field": "population", "description": "population must be >= 0" }
         ]
       }
```

## When validation runs

Validation fires after route matching and before condition / case
resolution. A failed validation never reaches your stub — the request
short-circuits at the 400 / `INVALID_ARGUMENT` boundary. A route with no
`[[validation]]` block bypasses the validator entirely; the body is not
re-parsed and the existing behaviour is unchanged.

## Try it

A runnable demo lives in `examples/validation/`. Start the server and
post a deliberately bad payload:

```sh
apitwin --config examples/validation
http POST :4000/cities name=Lima country_code=fra population:=-1 continent=atlantis
```

You'll see three violations in one 400 response.
