# apitwin Documentation

Welcome to the **apitwin** documentation — your complete guide to mocking, stubbing, and proxying HTTP and gRPC APIs during development.

> Looking for the project README? See the [repository root](../README.md).

All examples throughout this documentation use a **geographic data API** (continents, countries, cities) to demonstrate features progressively — from simple CRUD to complex cross-referenced responses.

---

## Getting Started

| Page | Description |
|---|---|
| [Installation](installation.md) | Install via npm, binary download, `go install`, or build from source |
| [Quick Start](quick-start.md) | Get up and running in under a minute |
| [CLI Reference](cli-reference.md) | All flags, subcommands, and usage patterns |

## Configuration

| Page | Description |
|---|---|
| [Overview](configuration/README.md) | Config file basics, directory config, auto-detection |
| [Routes](configuration/routes.md) | Route fields, path matching (exact, wildcard, regex, named parameters) |
| [Cases](configuration/cases.md) | Case fields, response definitions, persistence options |
| [Config Formats](configuration/formats.md) | Side-by-side TOML, YAML, and JSON examples |

## Features

| Page | Description |
|---|---|
| [Conditions](features/conditions.md) | Route requests by body, query, header, or path parameter values |
| [Named Parameters](features/named-parameters.md) | `{name}` syntax for path extraction, dynamic files, and persistence |
| [Directory-Based Stubs](features/directory-stubs.md) | CRUD operations with one JSON file per resource |
| [Dynamic File Resolution](features/dynamic-files.md) | `{source.field}` placeholders in file paths |
| [Template Tokens](features/template-tokens.md) | `{{uuid}}`, `{{now}}`, `{{timestamp}}` in responses |
| [Cross-Endpoint References](features/cross-endpoint-references.md) | `{{ref:...}}` syntax to reference data from other stub files with filtering and transformation |
| [Array Processing](features/array-processing.md) | `$each` / `$template` syntax for iterating over collections and reshaping data |
| [Response Transitions](features/response-transitions.md) | Time-based state progression across response cases |
| [Record Mode](features/record-mode.md) | Proxy a real API, save responses, replay offline |
| [Hot Reload](features/hot-reload.md) | Edit config files and see changes on the next request |
| [API Prefix](features/api-prefix.md) | Strip path prefixes before matching and forwarding |
| [Devtool UI](features/devtool-ui.md) | Browser dashboard for inspecting routes, testing requests, and watching config changes |

## gRPC

| Page | Description |
|---|---|
| [Overview](grpc/README.md) | How gRPC mocking works in apitwin |
| [Quick Start](grpc/quick-start.md) | Get a gRPC mock server running in minutes |
| [Configuration](grpc/config.md) | `[[grpc_routes]]` format, match patterns, status codes |
| [Stubs & Conditions](grpc/stubs.md) | Stub format, conditions, proxy fallthrough |
| [Persistence](grpc/persistence.md) | Directory-based CRUD for gRPC |
| [Generation](grpc/generation.md) | `apitwin generate --proto` workflow |

## OpenAPI

| Page | Description |
|---|---|
| [Overview](openapi/README.md) | Generate a complete mock from any OpenAPI 3 spec |
| [Generate Command](openapi/generate.md) | Full workflow, flags, output structure |
| [Stub Quality](openapi/stub-quality.md) | How stubs are synthesised and format hints |

## Examples

| Page | Description |
|---|---|
| [All Examples](examples.md) | Runnable examples for every feature |

---

## Quick Links

- [GitHub Repository](https://github.com/ridakaddir/apitwin)
- [Releases](https://github.com/ridakaddir/apitwin/releases)
- [npm Package](https://www.npmjs.com/package/@ridakaddir/apitwin)
- [License (MIT)](../LICENSE)
