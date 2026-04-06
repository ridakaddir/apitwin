# Template Tokens

[Home](../README.md) > [Features](README.md) > Template Tokens

---

apitwin supports template tokens in inline JSON values, file-based stubs, directory aggregations, and defaults files. Tokens are replaced with generated values or referenced data at request time.

## Available tokens

| Token | Output | Example |
|---|---|---|
| `{{uuid}}` | Random UUID v4 | `"a1b2c3d4-e5f6-7890-abcd-ef1234567890"` |
| `{{now}}` | RFC 3339 timestamp | `"2026-03-26T10:30:00Z"` |
| `{{timestamp}}` | Unix epoch in milliseconds | `1711273800000` |
| `{{ref:path}}` | Data from other stub files | `[{"name": "Morocco", "capital": "Rabat"}]` |

---

## Usage in inline JSON

```toml
[routes.cases.created]
status = 201
json   = '{"code": "{{uuid}}", "created_at": "{{now}}", "ts": {{timestamp}}}'
```

**Response:**

```json
{
  "code": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "created_at": "2026-03-26T10:30:00Z",
  "ts": 1711273800000
}
```

> **Note:** `{{uuid}}` and `{{now}}` are string tokens (include quotes). `{{timestamp}}` is a numeric token (no quotes needed).

---

## Usage in defaults files

Template tokens work in [defaults files](directory-stubs.md#defaults) for persistence operations. Additionally, defaults files support **request data placeholders** that resolve to values from the current HTTP request:

**`stubs/defaults/country.json`:**

```json
{
  "code": "{{uuid}}",
  "status": "active",
  "continent": "{.continent}",
  "region": "{header.X-Region}",
  "verified": false,
  "createdAt": "{{now}}",
  "neighboringCountries": "{{ref:countries/{.continent}/}}"
}
```

**Supported placeholders in defaults:**

| Placeholder | Description | Example |
|-------------|-------------|---------|
| `{.field}` | Request body field | `{.continent}` → `"africa"` |
| `{path.param}` | URL path parameter | `{path.continentId}` → `"africa"` |
| `{query.param}` | Query parameter | `{query.region}` → `"north-africa"` |
| `{header.Name}` | Request header | `{header.X-Region}` → `"north-africa"` |

All tokens and placeholders are resolved before the defaults are merged with the request body. This enables **dynamic defaults** that adapt based on the incoming request data.

---

## Usage in gRPC stubs

Standard template tokens (`{{uuid}}`, `{{now}}`, `{{timestamp}}`) work identically in gRPC stub responses:

```toml
[grpc_routes.cases.ok]
status = 0
json   = '{"countryId": "{{uuid}}", "createdAt": "{{now}}"}'
```

**Note:** Cross-endpoint references (`{{ref:...}}`) are currently supported for HTTP stubs only, not gRPC.

---

## Cross-Endpoint References

The `{{ref:...}}` token allows referencing data from other stub files:

```toml
[routes.cases.list]
json = '''
{
  "countries": "{{ref:stubs/countries/}}",
  "africanCountries": "{{ref:stubs/countries/?filter=continent:africa}}",
  "citySummaries": "{{ref:stubs/cities/?template=stubs/templates/city-summary.json}}"
}
'''
```

This powerful feature enables building interconnected APIs where endpoints share and reference each other's data with optional filtering and transformation.

**For full details:** See [Cross-Endpoint References](cross-endpoint-references.md)

---

**See also:** [Cases](../configuration/cases.md) | [Directory-Based Stubs](directory-stubs.md) | [Cross-Endpoint References](cross-endpoint-references.md)
