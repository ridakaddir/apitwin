# Cross-Endpoint References

---

Cross-endpoint references allow you to include data from other stub files in your responses using the `{{ref:...}}` syntax. This enables building interconnected mock APIs where endpoints reference and share data with optional filtering and transformation.

## Syntax

```
{{ref:path}}                           # Reference all items from a directory or single file
{{ref:path?filter=field:value}}        # Filter by field equality
{{ref:path?template=template.json}}    # Transform data shape using Go templates
{{ref:path?filter=field:value&template=template.json}}  # Both filter and transform
"$spread": "{{ref:path}}"              # Spread object properties into containing object
```

---

## Basic Usage

### Directory References

Reference all JSON files in a directory:

```json
{
  "name": "Africa",
  "area_km2": 30300000,
  "countries": "{{ref:stubs/countries/}}"
}
```

### Single File References

Reference a specific JSON file:

```json
{
  "name": "Casablanca",
  "country": "Morocco",
  "countryDetails": "{{ref:stubs/countries/morocco.json}}"
}
```

---

## Filtering

Filter referenced data by field values using `?filter=field:value` syntax.

### Simple Field Filtering

```json
{
  "africanCountries": "{{ref:stubs/countries/?filter=continent:africa}}",
  "largeCities": "{{ref:stubs/cities/?filter=coastal:true}}"
}
```

### Nested Field Filtering

Use dot notation to filter by nested object properties:

```json
{
  "arabicSpeaking": "{{ref:stubs/countries/?filter=languages.primary:arabic}}",
  "atlanticPorts": "{{ref:stubs/cities/?filter=geography.coast:atlantic}}"
}
```

### Multiple Filters

Chain multiple filters with `&` (all must match):

```json
{
  "filtered": "{{ref:stubs/cities/?filter=country:morocco&filter=coastal:true}}"
}
```

---

## Template Transformation

Transform the shape of referenced data using Go template files to rename fields, select specific properties, or restructure the output.

### Template File

Create a template file using Go's `text/template` syntax:

**`stubs/templates/city-summary.json`:**
```json
{
  "cityName": "{{.name}}",
  "pop": "{{.population}}",
  "country": "{{.country}}"
}
```

### Usage

Reference the template to transform data:

```json
{
  "cities": "{{ref:stubs/cities/?template=stubs/templates/city-summary.json}}"
}
```

This transforms:
```json
{"name": "Casablanca", "country": "morocco", "population": 3360000, "coastal": true, "coordinates": {"lat": 33.57, "lon": -7.59}}
```

Into:
```json
{"cityName": "Casablanca", "pop": 3360000, "country": "morocco"}
```

---

## Combining Filter and Template

Apply both filtering and transformation:

```json
{
  "moroccanCities": "{{ref:stubs/cities/?filter=country:morocco&template=stubs/templates/city-summary.json}}"
}
```

This workflow:
1. Loads all cities from `stubs/cities/`
2. Filters to only `country:morocco` cities
3. Transforms each city using the template
4. Returns the transformed array

---

## Security Considerations

Cross-endpoint references are restricted for security:

- **Path Traversal Prevention**: References cannot use `../` or absolute paths
- **Config Directory Boundary**: All referenced files must be within the config directory
- **Template Path Validation**: Template paths follow the same restrictions
- **No External File Access**: References cannot read files outside the project directory

Examples of blocked references:
```json
{"data": "{{ref:../secret.json}}"}           // Directory traversal
{"data": "{{ref:/etc/passwd}}"}              // Absolute path
{"data": "{{ref:data/?template=../tpl.json}}"} // Template traversal
```

---

## Advanced Features

### Nested References

Referenced files can contain their own `{{ref:...}}` tokens (with circular reference detection):

**`stubs/countries/morocco.json`:**
```json
{
  "name": "Morocco",
  "continent": "Africa",
  "capital": "Rabat",
  "cities": "{{ref:stubs/cities/?filter=country:morocco}}",
  "continentInfo": "{{ref:stubs/continents/africa.json}}"
}
```

**`stubs/continents/africa.json`:**
```json
{
  "name": "Africa",
  "area_km2": 30300000,
  "totalCountries": 54
}
```

### Empty Results

When filters match no items, an empty array `[]` is returned (never `null`):

```json
{
  "noMatches": "{{ref:stubs/cities/?filter=country:nonexistent}}"
}
// Returns: {"noMatches": []}
```

### Error Handling

- **Missing files**: Clear error with file path
- **Circular references**: Automatic detection and prevention
- **Invalid syntax**: Descriptive parsing errors
- **Template errors**: Go template compilation/execution errors

---

## Example: Geographic API

**Directory Structure:**
```
stubs/
├── continents/
│   ├── africa.json       # {"name": "Africa", "area_km2": 30300000}
│   └── europe.json       # {"name": "Europe", "area_km2": 10180000}
├── countries/
│   ├── morocco.json      # {"code": "morocco", "name": "Morocco", "continent": "africa", "capital": "Rabat"}
│   ├── germany.json      # {"code": "germany", "name": "Germany", "continent": "europe", "capital": "Berlin"}
│   ├── japan.json        # {"code": "japan", "name": "Japan", "continent": "asia", "capital": "Tokyo"}
│   └── canada.json       # {"code": "canada", "name": "Canada", "continent": "north-america", "capital": "Ottawa"}
├── cities/
│   ├── casablanca.json   # {"name": "Casablanca", "country": "morocco", "population": 3360000}
│   ├── berlin.json       # {"name": "Berlin", "country": "germany", "population": 3645000}
│   ├── tokyo.json        # {"name": "Tokyo", "country": "japan", "population": 13960000}
│   └── toronto.json      # {"name": "Toronto", "country": "canada", "population": 2930000}
└── templates/
    └── city-summary.json
```

**`stubs/continents/africa.json`:**
```json
{
  "name": "Africa",
  "area_km2": 30300000,
  "countries": "{{ref:stubs/countries/?filter=continent:africa}}",
  "moroccanCities": "{{ref:stubs/cities/?filter=country:morocco&template=stubs/templates/city-summary.json}}"
}
```

**`stubs/templates/city-summary.json`:**
```json
{
  "cityName": "{{.name}}",
  "pop": "{{.population}}"
}
```

**Result:**
```json
{
  "name": "Africa",
  "area_km2": 30300000,
  "countries": [
    {"code": "morocco", "name": "Morocco", "continent": "africa", "capital": "Rabat"}
  ],
  "moroccanCities": [
    {"cityName": "Casablanca", "pop": 3360000}
  ]
}
```

---

## Usage in Config

Cross-endpoint references work in both file-based stubs and inline JSON:

### File-Based Stubs

```toml
[[routes]]
method = "GET"
match = "/continents"

  [routes.cases.list]
  file = "stubs/continents/"  # Files in this directory can contain {{ref:...}}
```

### Inline JSON

```toml
[[routes]]
method = "POST"
match = "/countries"

  [routes.cases.created]
  status = 201
  json = '''
  {
    "code": "{{uuid}}",
    "createdAt": "{{now}}",
    "existingCountries": "{{ref:stubs/countries/}}"
  }
  '''
```

---

## Usage in Defaults Files

Cross-endpoint references support **dynamic placeholders** in defaults files, allowing you to reference different stub files based on request data. This enables region-specific, continent-specific, or language-specific data loading in both regular operations and background transitions.

### Dynamic Placeholder Syntax

Use these placeholders inside `{{ref:...}}` tokens:

| Placeholder | Description | Example |
|-------------|-------------|---------|
| `{.field}` | Request body field | `{{ref:stubs/{.continent}/countries/}}` |
| `{.nested.field}` | Nested body field | `{{ref:stubs/{.geo.region}/countries/}}` |
| `{path.param}` | URL path parameter | `{{ref:stubs/{path.continentId}/countries/}}` |
| `{query.param}` | Query parameter | `{{ref:stubs/{query.region}/countries/}}` |
| `{header.Name}` | Request header | `{{ref:stubs/{header.X-Region}/countries/}}` |

### Example: Continent-Specific Defaults

**Config:**
```toml
[[routes]]
method = "POST"
match = "/api/countries"

  [routes.cases.created]
  status = 201
  persist = true
  merge = "append"
  key = "code"
  defaults = "defaults/country.json"
```

**`defaults/country.json`:**
```json
{
  "status": "active",
  "continent": "{.continent}",
  "neighboringCountries": "{{ref:countries/{.continent}/}}",
  "continentInfo": "{{ref:continents/{.continent}.json}}"
}
```

**Request:**
```bash
POST /api/countries
{
  "code": "tunisia",
  "name": "Tunisia",
  "continent": "africa"
}
```

This resolves to defaults that load:
- `countries/africa/` directory (countries on the same continent)
- `continents/africa.json` file (continent details)

### Live Directory References

When a defaults file contains a `{{ref:...}}` token that points to a **directory** (path ending with `/`), the reference is preserved as a live token in the created file rather than being resolved at creation time. This means directory references resolve dynamically on every read, so they always reflect the current state of the referenced directory.

For example, if `defaults/continent.json` contains:
```json
{
  "continentId": "{{uuid}}",
  "countries": "{{ref:stubs/countries/{.continentId}/?template=stubs/templates/country-summary.json}}"
}
```

When a continent is created, the `countries` field is stored as `"{{ref:stubs/countries/africa/?template=...}}"` (with `{.continentId}` resolved to the concrete value). Each subsequent GET request resolves this reference against the current contents of the country directory — so newly added countries appear immediately.

File-based refs (not ending with `/`) are still resolved at creation time, since they point to static data.

### Background Transitions

Dynamic refs work in **background transitions** by storing the original request context when the transition is scheduled:

**Config with transitions:**
```toml
[[routes]]
method   = "POST"
match    = "/api/cities"
fallback = "created"

  [[routes.transitions]]
  case     = "pending"
  duration = 30

  [[routes.transitions]]
  case     = "verified"

  [routes.cases.created]
  status   = 201
  persist  = true
  merge    = "append"
  defaults = "defaults/city.json"

  [routes.cases.verified]
  persist  = true
  merge    = "update"
  defaults = "defaults/city-verified.json"
```

**`defaults/city-verified.json`:**
```json
{
  "status": "verified",
  "countryInfo": "{{ref:countries/{.country}.json}}",
  "nearbyCity": "{{ref:cities/{.country}/}}"
}
```

When the background transition fires after 30 seconds, the dynamic placeholders `{.country}` are resolved using the **original request data** that was stored when the transition was scheduled.

### Error Handling

Dynamic refs use **strict error handling** for data integrity:

- **Missing field**: Error if placeholder field not found in request
- **Empty value**: Error if placeholder resolves to empty string
- **Missing file**: Error for non-existent file references
- **Missing directory**: Returns empty array `[]` (standard behavior)

```json
// These cause errors:
{"data": "{{ref:stubs/{.missingField}/}}"}      // Field not in request
{"data": "{{ref:stubs/{.emptyField}/}}"}        // Field exists but empty
{"data": "{{ref:stubs/{.field}/missing.json}}"} // File doesn't exist

// This succeeds (returns empty array):
{"data": "{{ref:stubs/{.field}/missing-dir/}}"} // Directory doesn't exist
```

### Advanced Example: Region-Based API

**Directory Structure:**
```
stubs/
├── regions/
│   ├── north-africa/
│   │   └── countries/
│   │       └── morocco.json
│   └── western-europe/
│       └── countries/
│           └── germany.json
└── defaults/
    ├── region-country.json
    └── region-city.json
```

**`defaults/region-country.json`:**
```json
{
  "region": "{header.X-Region}",
  "continent": "{.continent}",
  "countries": "{{ref:regions/{header.X-Region}/{.continent}/countries/}}",
  "metadata": {
    "createdAt": "{{now}}",
    "countryId": "{{uuid}}"
  }
}
```

**Request:**
```bash
POST /api/countries
X-Region: north-africa

{
  "name": "Tunisia",
  "continent": "africa"
}
```

**Resolved defaults:**
```json
{
  "region": "north-africa",
  "continent": "africa",
  "countries": [{"code": "morocco", "name": "Morocco", "capital": "Rabat"}],
  "metadata": {
    "createdAt": "2026-03-26T10:30:00Z",
    "countryId": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

---

## Best Practices

1. **Organize by domain**: Group related stubs in directories (`continents/`, `countries/`, `cities/`)

2. **Use descriptive template names**: `city-summary.json`, `country-brief.json`

3. **Filter for relevance**: Only include data that makes sense for the context

4. **Template for clean APIs**: Transform internal data structures to clean API responses

5. **Avoid deep nesting**: Keep reference chains shallow for maintainability

6. **Handle empty cases**: Design UIs to handle empty arrays gracefully

---

## Object Spreading

Object spreading allows you to merge the properties of a referenced object directly into the containing object using the `$spread` field with a `{{ref:...}}` reference. This is particularly useful for combining data from multiple sources into a flat response structure.

### Syntax

```
"$spread": "{{ref:path}}"                    # Spread all properties from referenced object
"$spread": "{{ref:path?filter=field:value}}" # Spread filtered properties
"$spread": "{{ref:path?template=template.json}}" # Spread transformed properties
```

### Basic Spreading

Spread all properties from a referenced file:

```json
{
  "id": "morocco-detail",
  "$spread": "{{ref:stubs/countries/morocco.json}}",
  "cities": "{{ref:stubs/cities/?filter=country:morocco&template=stubs/templates/city-summary.json}}"
}
```

**Referenced file** (`stubs/countries/morocco.json`):
```json
{
  "code": "morocco",
  "name": "Morocco",
  "continent": "africa",
  "capital": "Rabat",
  "population": 37000000
}
```

**Result** (flat structure):
```json
{
  "id": "morocco-detail",
  "code": "morocco",
  "name": "Morocco",
  "continent": "africa",
  "capital": "Rabat",
  "population": 37000000,
  "cities": [{"cityName": "Casablanca", "pop": 3360000}]
}
```

### Property Override

Explicit properties override spread properties when there are conflicts:

```json
{
  "$spread": "{{ref:stubs/countries/morocco.json}}",
  "capital": "Casablanca"  // This overrides "Rabat" from the spread object
}
```

### Spreading with Dynamic Placeholders

Combine spreading with path parameters and other dynamic placeholders:

```json
{
  "$spread": "{{ref:stubs/countries/{path.countryId}.json}}",
  "continent": "{path.continent}",
  "cities": "{{ref:stubs/cities/?filter=country:{path.countryId}}}"
}
```

### Use Cases

**1. Country Detail Enhancement**: Add related data to existing country data
```json
{
  "$spread": "{{ref:stubs/countries/{path.countryId}.json}}",
  "cities": "{{ref:stubs/cities/?filter=country:{path.countryId}}}",
  "lastUpdated": "{{now}}"
}
```

**2. Configuration Merging**: Combine base configuration with region-specific overrides
```json
{
  "$spread": "{{ref:stubs/defaults/country-base.json}}",
  "continent": "{path.continent}",
  "timezone": "UTC+1"
}
```

**3. Response Composition**: Build complex responses from multiple data sources
```json
{
  "$spread": "{{ref:stubs/countries/{path.countryId}.json}}",
  "cities": "{{ref:stubs/cities/?filter=country:{path.countryId}}}",
  "continentInfo": "{{ref:stubs/continents/{.continent}.json}}"
}
```

**4. Nested Object Spreading**: Spread properties into nested structures
```json
{
  "code": "morocco",
  "geography": {
    "$spread": "{{ref:stubs/geography/morocco.json}}",
    "isCoastal": true
  },
  "status": "active"
}
```

### Limitations

1. **Objects only**: Can only spread objects (`map[string]interface{}`), not arrays or primitives
2. **Per-object scope**: Each `$spread` only merges into its immediate containing object, but you can use `$spread` inside nested objects for nested spreading
3. **Key conflicts**: Later properties override earlier ones (explicit > spread)
4. **Processing order**: Spread resolution happens before regular reference resolution

### Error Handling

**Invalid Syntax:**
```json
{
  "$spread": "invalid-value"  // ERROR: Must be {{ref:...}} token
}
```

**Non-Object Reference:**
```json
{
  "$spread": "{{ref:stubs/cities/}}"  // ERROR: Cannot spread array
}
```

The above will result in an error: `$spread ref must resolve to an object, got []interface {}`

**Invalid Type:**
```json
{
  "$spread": 123  // ERROR: Must be string
}
```

Result: `$spread field must be a string, got int`

---

**See also:** [Array Processing](array-processing.md) | [Type Conversion](type-conversion.md) | [Template Tokens](template-tokens.md) | [Directory-Based Stubs](directory-stubs.md) | [Dynamic Files](dynamic-files.md)
