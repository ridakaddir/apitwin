# Config Formats

---

apitwin auto-detects the config format from the file extension. All three formats support the same feature set.

## TOML

```toml
[[routes]]
method   = "GET"
match    = "/api/countries"
enabled  = true
fallback = "success"

  [[routes.conditions]]
  source = "query"
  field  = "continent"
  op     = "eq"
  value  = "africa"
  case   = "african_countries"

  [routes.cases.success]
  status = 200
  file   = "stubs/countries.json"

  [routes.cases.african_countries]
  status = 200
  file   = "stubs/african-countries.json"

[[routes]]
method   = "POST"
match    = "/api/countries"
enabled  = true
fallback = "created"

  [routes.cases.created]
  status = 201
  json   = '{"code": "{{uuid}}", "status": "active"}'
```

---

## YAML

```yaml
routes:
  - method: GET
    match: /api/countries
    enabled: true
    fallback: success
    conditions:
      - source: query
        field: continent
        op: eq
        value: africa
        case: african_countries
    cases:
      success:
        status: 200
        file: stubs/countries.json
      african_countries:
        status: 200
        file: stubs/african-countries.json

  - method: POST
    match: /api/countries
    enabled: true
    fallback: created
    cases:
      created:
        status: 201
        json: '{"code": "{{uuid}}", "status": "active"}'
```

---

## JSON

```json
{
  "routes": [
    {
      "method": "GET",
      "match": "/api/countries",
      "enabled": true,
      "fallback": "success",
      "conditions": [
        {
          "source": "query",
          "field": "continent",
          "op": "eq",
          "value": "africa",
          "case": "african_countries"
        }
      ],
      "cases": {
        "success": { "status": 200, "file": "stubs/countries.json" },
        "african_countries": { "status": 200, "file": "stubs/african-countries.json" }
      }
    },
    {
      "method": "POST",
      "match": "/api/countries",
      "enabled": true,
      "fallback": "created",
      "cases": {
        "created": {
          "status": 201,
          "json": "{\"code\": \"{{uuid}}\", \"status\": \"active\"}"
        }
      }
    }
  ]
}
```

---

## Mixing formats

When using [directory config](README.md#directory-config), you can mix formats freely:

```
mocks/
├── continents.toml
├── countries.yaml
└── cities.json
```

All files are loaded and their routes merged in alphabetical order.

---

**See also:** [Configuration Overview](README.md) | [Routes](routes.md) | [Cases](cases.md)
