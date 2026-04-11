# Array Processing with `$each` / `$template`

---

Process arrays from cross-endpoint references by iterating over each item and applying a template to reshape or enrich the data. This is useful for building responses that combine data from multiple stub directories.

## Syntax

```json
{
  "$each": "{{ref:path}}",
  "$template": {
    "field": "{{.sourceField}}",
    "nested": "{{ref:other-path/{.id}/}}"
  }
}
```

- **`$each`** — a `{{ref:...}}` token that resolves to an array. Each item in the array is processed individually.
- **`$template`** — the shape applied to every item. Use `{{.fieldName}}` to access properties of the current item.

---

## Basic Example

List all countries in a continent with their capital cities:

**Stub directory:**
```
stubs/
├── continents/
│   └── africa.json
├── countries/
│   ├── morocco.json       # {"code": "morocco", "name": "Morocco", "continent": "africa", "capital": "Rabat", "population": 37000000}
│   └── canada.json        # {"code": "canada", "name": "Canada", "continent": "north-america", "capital": "Ottawa", "population": 38000000}
└── cities/
    ├── casablanca.json    # {"name": "Casablanca", "country": "morocco", "population": 3360000}
    ├── rabat.json         # {"name": "Rabat", "country": "morocco", "population": 580000}
    └── toronto.json       # {"name": "Toronto", "country": "canada", "population": 2930000}
```

**`stubs/continents/africa.json`:**
```json
{
  "name": "Africa",
  "countries": {
    "$each": "{{ref:countries/?filter=continent:africa}}",
    "$template": {
      "name": "{{.name}}",
      "capital": "{{.capital}}",
      "population": "{{.population}}"
    }
  }
}
```

**Result:**
```json
{
  "name": "Africa",
  "countries": [
    {
      "name": "Morocco",
      "capital": "Rabat",
      "population": 37000000
    }
  ]
}
```

---

## Enriching with Nested References

The real power of `$each` / `$template` is combining data from multiple directories. Each item can pull in related data using `{{ref:...}}` with the current item's fields:

**`stubs/continents/africa.json`:**
```json
{
  "name": "Africa",
  "countries": {
    "$each": "{{ref:countries/?filter=continent:africa}}",
    "$template": {
      "name": "{{.name}}",
      "capital": "{{.capital}}",
      "cities": "{{ref:cities/?filter=country:{{.code}}}}"
    }
  }
}
```

**Result:**
```json
{
  "name": "Africa",
  "countries": [
    {
      "name": "Morocco",
      "capital": "Rabat",
      "cities": [
        {"name": "Casablanca", "country": "morocco", "population": 3360000},
        {"name": "Rabat", "country": "morocco", "population": 580000}
      ]
    }
  ]
}
```

---

## Using Templates for Nested Data

Combine `$each` with the `?template=` query parameter to reshape nested references:

**`stubs/templates/city-summary.json`:**
```json
{
  "cityName": "{{.name}}",
  "pop": "{{.population}}"
}
```

**`stubs/continents/africa.json`:**
```json
{
  "name": "Africa",
  "countries": {
    "$each": "{{ref:countries/?filter=continent:africa}}",
    "$template": {
      "name": "{{.name}}",
      "capital": "{{.capital}}",
      "cities": "{{ref:cities/?filter=country:{{.code}}&template=templates/city-summary.json}}"
    }
  }
}
```

**Result:**
```json
{
  "name": "Africa",
  "countries": [
    {
      "name": "Morocco",
      "capital": "Rabat",
      "cities": [
        {"cityName": "Casablanca", "pop": 3360000},
        {"cityName": "Rabat", "pop": 580000}
      ]
    }
  ]
}
```

---

## Context Variables

Inside `$template`, use `{{.fieldName}}` to access any property of the current array item:

| Syntax | Description | Example |
|---|---|---|
| `{{.name}}` | Top-level field of the current item | `"Morocco"` |
| `{{.capital}}` | Another top-level field | `"Rabat"` |
| `{{.code}}` | Used in nested `{{ref:...}}` paths | `"morocco"` |

---

## Using in Config

Array processing works in file-based stubs referenced from your config:

```toml
[[routes]]
method   = "GET"
match    = "/continents/{continentId}"
enabled  = true
fallback = "detail"

  [routes.cases.detail]
  status = 200
  file   = "stubs/continents/{path.continentId}.json"
```

A request to `GET /continents/africa` serves `stubs/continents/africa.json`, which resolves all `$each` / `$template` blocks and nested `{{ref:...}}` tokens before returning the response.

---

## Combining with `$spread`

Use `$each` / `$template` alongside [object spreading](cross-endpoint-references.md#object-spreading):

```json
{
  "$spread": "{{ref:continents/africa.json}}",
  "enrichedCountries": {
    "$each": "{{ref:countries/?filter=continent:africa}}",
    "$template": {
      "name": "{{.name}}",
      "cities": "{{ref:cities/?filter=country:{{.code}}}}"
    }
  }
}
```

This spreads all properties from `africa.json` into the response and adds an `enrichedCountries` array with nested city data.

---

## Error Handling

| Situation | Behaviour |
|---|---|
| `$each` resolves to an empty array | Returns an empty array `[]` |
| `$each` resolves to a non-array | Error: `$each ref must resolve to an array` |
| `$each` without `$template` | Error: `$each requires $template` |
| `$template` without `$each` | Error: `$template requires $each` |
| Missing field in `{{.fieldName}}` | Empty string in output |
| Invalid `{{ref:...}}` in `$template` | Error with descriptive message |

---

## Best Practices

1. **Keep templates simple** — extract only the fields you need in `$template`
2. **Use filters** — narrow down `$each` sources with `?filter=` to avoid processing unnecessary data
3. **Name files by key** — use meaningful filenames (e.g. `morocco.json`, `canada.json`) for readability
4. **Combine with template files** — use `?template=` for deeply nested reshaping instead of complex inline templates
5. **Test incrementally** — start with a basic `$each` / `$template`, then add nested references

---

**See also:** [Cross-Endpoint References](cross-endpoint-references.md) | [Type Conversion](type-conversion.md) | [Template Tokens](template-tokens.md) | [Directory-Based Stubs](directory-stubs.md)
