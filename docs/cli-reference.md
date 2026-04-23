# CLI Reference

## Usage

```
apitwin [flags]
apitwin generate [flags]
apitwin reset [flags]
```

---

## Flags

### HTTP server

| Flag | Short | Default | Description |
|---|---|---|---|
| `--target <url>` | `-t` | — | Upstream HTTP API to proxy unmatched requests to |
| `--port <n>` | `-p` | `4000` | HTTP port to listen on |
| `--config <path>` | `-c` | `apitwin.toml` or `.` | Config file or directory (auto-detected) |
| `--api-prefix <path>` | `-a` | — | Strip this prefix before matching routes and forwarding upstream (e.g. `/api`) |
| `--init` | | | Scaffold a `apitwin.toml` template in the current directory |
| `--record` | | | Record mode: proxy all requests and save responses as stubs |
| `--ephemeral` | | | Mirror stubs into a tempdir wiped on shutdown (nothing persists between runs) |
| `--no-runtime-dir` | | | Write mutations back to seed stubs — legacy mode, dirties git |
| `--reset-runtime` | | | Wipe `.apitwin/state/` on start and re-mirror from seed (default preserves runtime-only stubs and persisted transition state across restarts) |

### gRPC server

gRPC server starts **only** when `--grpc-proto` is provided.

| Flag | Default | Description |
|---|---|---|
| `--grpc-proto <file>` | — | Path to a `.proto` file; repeat for multiple files |
| `--grpc-port <n>` | `50051` | gRPC server port |
| `--grpc-target <addr>` | — | Upstream gRPC server for proxy mode (e.g. `localhost:9090`) |
| `--import-path <dir>` | — | Extra directory to search for proto imports; repeat for multiple |

### General

| Flag | Short | Description |
|---|---|---|
| `--help` | `-h` | Show help |
| `--version` | `-v` | Show version |

---

## Subcommands

### `generate`

Generate an apitwin config from an OpenAPI spec or `.proto` files.

```sh
apitwin generate [flags]
```

#### OpenAPI flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--spec <file\|url>` | `-s` | — | OpenAPI spec file path or URL |
| `--out <dir>` | `-o` | `mocks` | Output directory for config and stubs |
| `--format <fmt>` | `-f` | `toml` | Config format: `toml`, `yaml`, `json` |
| `--split` | | `true` | One file per tag; `--split=false` for a single file |

#### Proto flags

| Flag | Default | Description |
|---|---|---|
| `--proto <file>` | — | Path to a `.proto` file; repeat for multiple files |
| `--import-path <dir>` | — | Extra directory to search for proto imports; repeat for multiple |

Either `--spec` or `--proto` is required. `--proto` takes precedence if both are provided.

### `reset`

Delete the runtime state directory so the next run starts from a fresh mirror of the seed stubs. Clears runtime-only stubs (POST creations), persisted transition `FirstHit` timestamps, and pending scheduled mutations.

```sh
apitwin reset [flags]
```

| Flag | Short | Default | Description |
|---|---|---|---|
| `--config <path>` | `-c` | `apitwin.toml` or `.` | Config file or directory (used to locate `.apitwin/state/`) |

For an inline reset at server start without invoking the subcommand, pass `--reset-runtime` to the main command.

When running in `--ephemeral` mode the runtime state is per-process and cleaned up on shutdown, so `reset` is a no-op.

---

## Common usage patterns

**Scaffold and start:**

```sh
apitwin --init
apitwin --target https://restcountries.com/v3.1
```

**Generate from OpenAPI and serve:**

```sh
apitwin generate --spec openapi.yaml --out ./mocks
apitwin --config ./mocks
```

**Selective mock with proxy fallthrough:**

```sh
apitwin --config ./mocks --target https://restcountries.com/v3.1
```

**Record real API responses:**

```sh
apitwin --config ./mocks --target https://restcountries.com/v3.1 --record
```

**Start HTTP + gRPC servers:**

```sh
apitwin --config ./mocks --grpc-proto geo.proto
```

**gRPC with proto imports from a shared directory:**

```sh
apitwin --config ./mocks \
      --grpc-proto proto/service/v1/service.proto \
      --import-path proto/
```

**gRPC with upstream proxy:**

```sh
apitwin --config ./mocks \
      --grpc-proto geo.proto \
      --grpc-target localhost:9090
```

**Using Task runner:**

```sh
task generate SPEC=openapi.yaml
task generate SPEC=https://petstore3.swagger.io/api/v3/openapi.json
task generate SPEC=openapi.yaml OUT=./geo-mocks
```

---

**See also:** [Quick Start](quick-start.md) | [Configuration](configuration/README.md)
