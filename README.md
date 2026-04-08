<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.svg">
    <img src="assets/logo.svg" alt="apitwin" width="220">
  </picture>
</p>

<p align="center">
  A fast, zero-dependency-on-your-app CLI tool for developers to<br>
  <strong>mock, stub, and proxy HTTP and gRPC APIs</strong> — written in Go.
</p>

<p align="center">
  <a href="https://github.com/ridakaddir/apitwin/actions/workflows/ci.yml"><img src="https://github.com/ridakaddir/apitwin/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/ridakaddir/apitwin/releases"><img src="https://github.com/ridakaddir/apitwin/actions/workflows/release.yml/badge.svg" alt="Release"></a>
  <a href="https://goreportcard.com/report/github.com/ridakaddir/apitwin"><img src="https://goreportcard.com/badge/github.com/ridakaddir/apitwin" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
</p>

<p align="center">
  Point your frontend or service at <code>apitwin</code> instead of the real API.<br>
  Mock only the endpoints you're actively building. Forward everything else to the real backend.<br>
  Switch between response scenarios by editing a config file — changes apply instantly with no restart.
</p>

<p align="center">
  <a href="https://ridakaddir.github.io/apitwin"><strong>Documentation Site</strong></a> | <a href="docs/README.md"><strong>Markdown Docs</strong></a>
</p>

---

## Features

- **Route-based mocking** — define routes with named response cases and condition routing
- **Cascade mutations** — atomic multi-file updates with rollback for synchronized data changes
- **Cross-endpoint references** — `{{ref:...}}` syntax to reference data from other stub files with filtering and transformation
- **Array processing** — `$each` / `$template` syntax to iterate over collections and reshape each item
- **Object spreading** — `$spread` syntax to merge referenced object properties into the containing object
- **Type conversion** — `$as` directive to convert between types (e.g. merge an array of objects into a single object)
- **Dynamic refs in defaults** — `{{ref:stubs/{.field}/data/}}` with request data placeholders for environment-specific defaults
- **gRPC mock & proxy** — mock unary gRPC methods from `.proto` files with `{body.field}` interpolation and directory wrapping; no `protoc` or codegen required
- **Named path parameters** — `{name}` syntax for path extraction, dynamic files, and persistence
- **Directory-based stubs** — CRUD operations with one JSON file per resource
- **Reverse proxy fallthrough** — unmatched routes forward to a real upstream API
- **Hot reload** — edit config files and see changes on the next request
- **Embedded devtool UI** — browser dashboard at `/__ui/` for inspecting routes, testing requests, and watching config changes live
- **Record mode** — proxy a real API, save responses as stubs, replay offline
- **OpenAPI generation** — generate a complete mock from any OpenAPI 3 spec
- **Response transitions** — time-based state progression (e.g. `pending` → `approved`)
- **Multi-format config** — TOML, YAML, or JSON — auto-detected by file extension

---

## Install

```sh
npm install -D @ridakaddir/apitwin
```

Or download a binary from the [latest release](https://github.com/ridakaddir/apitwin/releases). See the [installation guide](docs/installation.md) for all options.

---

## Quick Start

**From an OpenAPI spec:**

```sh
apitwin generate --spec openapi.yaml --out ./mocks
apitwin --config ./mocks
```

**Manual scaffold:**

```sh
apitwin --init
apitwin --target https://api.example.com
```

**gRPC:**

```sh
apitwin generate --proto service.proto --out ./mocks
apitwin --config ./mocks --grpc-proto service.proto
```

See the [quick start guide](docs/quick-start.md) for more details.

---

## How It Works

All examples in this documentation use a **geographic data API** (continents, countries, cities) to demonstrate every feature progressively.

### 1. Define your stubs

```
mocks/
├── continents/
│   ├── africa.json
│   └── europe.json
├── countries/
│   ├── morocco.json
│   └── germany.json
├── cities/
│   ├── casablanca.json
│   └── berlin.json
└── templates/
    └── city-summary.json
```

**`countries/morocco.json`:**
```json
{
  "name": "Morocco",
  "continent": "Africa",
  "capital": "Rabat",
  "population": 37000000,
  "languages": ["Arabic", "Berber", "French"]
}
```

### 2. Write a config

```toml
[[routes]]
method   = "GET"
match    = "/countries"
enabled  = true
fallback = "list"

  [routes.cases.list]
  status = 200
  file   = "countries/"

[[routes]]
method   = "GET"
match    = "/countries/{countryId}"
enabled  = true
fallback = "detail"

  [routes.cases.detail]
  status = 200
  file   = "countries/{path.countryId}.json"
```

### 3. Start apitwin

```sh
apitwin --config ./mocks
```

```
GET /countries          → [morocco, germany, ...]
GET /countries/morocco  → {"name": "Morocco", ...}
```

### 4. Add cross-references

Countries can reference their cities:

```json
{
  "name": "Morocco",
  "continent": "Africa",
  "capital": "Rabat",
  "cities": "{{ref:cities/?filter=country:morocco}}"
}
```

### 5. Process arrays with `$each` / `$template`

Continents can list their countries with enriched data:

```json
{
  "name": "Africa",
  "countries": {
    "$each": "{{ref:countries/?filter=continent:africa}}",
    "$template": {
      "name": "{{.name}}",
      "capital": "{{.capital}}",
      "topCities": "{{ref:cities/?filter=country:{{.code}}&template=templates/city-summary.json}}"
    }
  }
}
```

---

## CLI Reference

```
apitwin [flags]

Flags:
  -t, --target        <url>    Upstream HTTP API to proxy unmatched requests to
  -p, --port          <n>      HTTP port (default: 4000)
  -c, --config        <path>   Config file or directory (default: apitwin.toml)
  -a, --api-prefix    <path>   Strip prefix before matching (e.g. /api)
      --init                   Scaffold an apitwin.toml template
      --record                 Record mode: proxy and save responses as stubs
      --grpc-proto    <file>   Path to .proto file (starts gRPC server)
      --grpc-port     <n>      gRPC port (default: 50051)
      --grpc-target   <addr>   Upstream gRPC server for proxy mode
      --import-path   <dir>    Extra directory to search for proto imports
  -h, --help
  -v, --version

Subcommands:
  generate    Generate config from OpenAPI spec or .proto files
```

See the [full CLI reference](docs/cli-reference.md) for all flags and usage patterns.

---

## Documentation

Full usage documentation lives in the [`docs/`](docs/README.md) directory:

| Section | Description |
|---|---|
| [Installation](docs/installation.md) | npm, binary, `go install`, build from source |
| [Quick Start](docs/quick-start.md) | Get running in under a minute |
| [CLI Reference](docs/cli-reference.md) | All flags and subcommands |
| [Configuration](docs/configuration/README.md) | Config files, routes, cases, formats |
| [Features](docs/features/README.md) | Conditions, persistence, transitions, recording, and more |
| [gRPC](docs/grpc/README.md) | gRPC mocking, proxy, persistence, generation |
| [OpenAPI](docs/openapi/README.md) | Generate mocks from OpenAPI 3 specs |
| [Examples](docs/examples.md) | Runnable examples for every feature |

---

## Project Structure

```
apitwin/
├── main.go                    # Entrypoint
├── cmd/
│   ├── root.go                # CLI (cobra) — HTTP + gRPC flags
│   └── generate.go            # generate subcommand (OpenAPI + proto)
├── internal/
│   ├── config/                # Config types + hot-reload loader
│   ├── generate/              # OpenAPI + proto config generation
│   ├── grpc/                  # gRPC server, handler, codec, proxy, persist
│   ├── logger/                # Terminal logger
│   ├── persist/               # Directory-based stub operations
│   └── proxy/                 # HTTP server, handler, matcher, conditions, mocking
├── examples/                  # Runnable examples for each feature
├── docs/                      # Usage documentation
├── ui/                        # Embedded devtool UI (React + Vite, go:embed)
├── npm/                       # npm distribution packages
├── assets/                    # Logos
└── .github/workflows/         # CI, release, npm publish
```

---

## Development

This project uses [devbox](https://www.jetify.com/devbox) for a reproducible dev environment and [Task](https://taskfile.dev) as the task runner.

### Setup

```sh
devbox shell                   # Go 1.25.1, golangci-lint, goreleaser
task --list                    # See available tasks
```

### Common tasks

```sh
task build                     # Compile binary
task test                      # go test ./... -race
task lint                      # golangci-lint
task check                     # fmt + vet + lint + test
task run:basic                 # Run with examples/basic config
```

### Without devbox

```sh
git clone https://github.com/ridakaddir/apitwin.git
cd apitwin
go build -o apitwin .
./apitwin --init
```

### Testing

```sh
task test                      # All tests with race detection
go test ./... -race -v         # Verbose output
go test ./internal/proxy/...   # Specific package
```

### CI / Release

| Workflow | Trigger | What it does |
|---|---|---|
| CI | push to `main`, PRs | vet, lint, test (race), build — Linux + macOS |
| Release | `v*` tag push | GoReleaser cross-platform binaries + GitHub Release |
| npm Publish | GitHub Release | Publish `@ridakaddir/apitwin` to npm |

```sh
git tag v0.1.0
git push origin v0.1.0
```

---

## Contributing

Contributions are welcome.

1. Fork the repo and create a branch from `main`
2. Make your changes with clear, focused commits
3. Run `task check` — fmt, vet, lint, and tests must all pass
4. Open a pull request against `main`

### Reporting a bug

Open an issue with:
- apitwin version (`apitwin --version`)
- Your config file (redact secrets)
- Steps to reproduce + expected vs actual behaviour

### Areas open for contribution

- Additional condition operators (`lt`, `gt` for numeric comparisons)
- Response headers in case definitions
- Shell completions (`apitwin completion bash|zsh|fish`)
- Homebrew formula

---

## License

MIT
