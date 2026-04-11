# Type Conversion with `$as`

---

Convert data between types using the `$as` directive. This is useful when a reference resolves to one type (e.g. an array of objects) but you need a different type (e.g. a single merged object).

## Syntax

```json
{
  "$as": "<targetType>",
  "from": "{{ref:path}}"
}
```

- **`$as`** -- the target type to convert to
- **`from`** -- a `{{ref:...}}` token that resolves to the source data

---

## Supported Conversions

| Target Type | Input | Output | JS Equivalent |
|---|---|---|---|
| `"object"` | Array of objects | Single merged object | `Object.assign({}, ...arr)` |

---

## Convert Array to Object

The `"object"` conversion merges an array of objects into a single flat object. Later keys overwrite earlier ones when there are conflicts.

### Basic Example

**`stubs/splits.json`:**
```json
[
  {"deployment-a": 60},
  {"deployment-b": 40}
]
```

**Usage:**
```json
{
  "$as": "object",
  "from": "{{ref:stubs/splits.json}}"
}
```

**Result:**
```json
{
  "deployment-a": 60,
  "deployment-b": 40
}
```

### Nested Usage

Use `$as` inside a larger response to convert a specific field:

```json
{
  "name": "My Endpoint",
  "status": "active",
  "trafficSplit": {
    "$as": "object",
    "from": "{{ref:stubs/deployments/endpoint-1/?template=stubs/templates/traffic-split.json}}"
  }
}
```

The `trafficSplit` field will contain the merged object while the surrounding fields remain unchanged.

---

## Real-World Example: Traffic Split Aggregation

A common pattern is aggregating a field from multiple files into a single object. For example, each deployment has its own `trafficSplit` entry, and the endpoint response needs all of them merged.

### Directory Structure

```
stubs/
  deployments/
    endpoint-1/
      dep-a.json      # {"deploymentId": "dep-a", "deploymentSpec": {"trafficSplit": {"dep-a": 60}}}
      dep-b.json      # {"deploymentId": "dep-b", "deploymentSpec": {"trafficSplit": {"dep-b": 40}}}
  templates/
    traffic-split.json
```

### Template File

Create a Go template that extracts just the `trafficSplit` from each deployment:

**`stubs/templates/traffic-split.json`:**
```
{{json .deploymentSpec.trafficSplit}}
```

When applied to the directory via `{{ref:stubs/deployments/endpoint-1/?template=stubs/templates/traffic-split.json}}`, this produces:
```json
[{"dep-a": 60}, {"dep-b": 40}]
```

### Using `$as` to Merge

```json
{
  "$as": "object",
  "from": "{{ref:stubs/deployments/endpoint-1/?template=stubs/templates/traffic-split.json}}"
}
```

**Result:**
```json
{
  "dep-a": 60,
  "dep-b": 40
}
```

### Complete Endpoint Response

Combine `$as` with `$spread` and other directives for a full response:

**Inline JSON (in config):**
```json
{
  "$spread": "{{ref:stubs/endpoints/{path.endpointId}.json}}",
  "trafficSplit": {
    "$as": "object",
    "from": "{{ref:stubs/deployments/{path.endpointId}/?template=stubs/templates/traffic-split.json}}"
  },
  "deployedModels": "{{ref:stubs/deployments/{path.endpointId}/?template=stubs/templates/deployed-model.json}}"
}
```

**View file (with `$each` / `$template`):**
```json
{
  "$each": "{{ref:stubs/endpoints/}}",
  "$template": {
    "endpointId": "{{.endpointId}}",
    "displayName": "{{.displayName}}",
    "trafficSplit": {
      "$as": "object",
      "from": "{{ref:stubs/deployments/{{.endpointId}}/?template=stubs/templates/traffic-split.json}}"
    },
    "deployedModels": "{{ref:stubs/deployments/{{.endpointId}}/?template=stubs/templates/deployed-model.json}}"
  }
}
```

---

## Combining with Other Directives

`$as` works alongside all other directives:

| Directive | Combination |
|---|---|
| `$spread` | Use `$as` for a nested field while `$spread` merges the parent object |
| `$each` / `$template` | Use `$as` inside a template to convert per-item data |
| `?template=` | Use a template to extract a specific field, then `$as` to merge the results |
| `?filter=` | Filter the source data before converting |

---

## Edge Cases

| Situation | Behaviour |
|---|---|
| Empty array `[]` | Returns empty object `{}` |
| Empty objects in array `[{}, {"a": 1}]` | Skipped gracefully, result: `{"a": 1}` |
| Overlapping keys `[{"a": 1}, {"a": 2}]` | Last value wins: `{"a": 2}` |
| Single-item array `[{"a": 1}]` | Unwrapped: `{"a": 1}` |

---

## Error Handling

| Situation | Error |
|---|---|
| `$as` without `"from"` field | `$as directive requires a "from" field` |
| `"from"` is not a `{{ref:...}}` token | `$as "from" value must be a {{ref:...}} token` |
| Source resolves to an object (not array) | `$as "object": source must be an array` |
| Array contains non-object items | `$as "object": array item at index N must be an object` |
| Unsupported target type | `unsupported $as target type: "banana"` |

---

## Processing Order

`$as` is resolved in step 3 of the reference resolution pipeline:

1. `$each` / `$template` -- array iteration
2. `$spread` -- object spreading
3. **`$as` -- type conversion**
4. Dynamic placeholders
5. Regular `{{ref:...}}` resolution

---

**See also:** [Cross-Endpoint References](cross-endpoint-references.md) | [Object Spreading](cross-endpoint-references.md#object-spreading) | [Array Processing](array-processing.md) | [Template Tokens](template-tokens.md)
