# Features

---

apitwin provides a rich set of features for mocking, stubbing, and proxying APIs. This section covers each feature in detail.

| Feature | Description |
|---|---|
| [Conditions](conditions.md) | Route requests to different responses based on body, query, header, or path values |
| [Named Parameters](named-parameters.md) | Extract values from URL paths with `{name}` syntax |
| [Directory-Based Stubs](directory-stubs.md) | Full CRUD with one JSON file per resource |
| [Dynamic File Resolution](dynamic-files.md) | `{source.field}` placeholders in stub file paths |
| [Template Tokens](template-tokens.md) | `{{uuid}}`, `{{now}}`, `{{timestamp}}`, and `{{ref:...}}` in responses |
| [Cross-Endpoint References](cross-endpoint-references.md) | Reference data from other stub files with filtering and transformation |
| [Array Processing](array-processing.md) | `$each` / `$template` syntax for iterating over collections and reshaping data |
| [Type Conversion](type-conversion.md) | `$as` directive to convert between types (e.g. merge an array of objects into one) |
| [Response Transitions](response-transitions.md) | Time-based state progression across cases |
| [Record Mode](record-mode.md) | Proxy a real API, save responses, replay offline |
| [Hot Reload](hot-reload.md) | Edit config, see changes on the next request |
| [API Prefix](api-prefix.md) | Strip path prefixes before matching and forwarding |
| [Devtool UI](devtool-ui.md) | Browser dashboard for inspecting routes, testing requests, and watching config changes |
