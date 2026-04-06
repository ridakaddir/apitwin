# gRPC Generation

[Home](../README.md) > [gRPC](README.md) > Generation

---

Scaffold a complete `[[grpc_routes]]` config and synthetic stub files from a `.proto` file in one command.

## Usage

```sh
apitwin generate --proto geo.proto --out ./mocks

# Multiple proto files
apitwin generate --proto countries.proto --proto cities.proto --out ./mocks

# With extra import paths for proto imports
apitwin generate --proto geo.proto --import-path ./vendor/protos --format yaml
```

---

## Generated output

For a `CountryService` with three methods:

```
mocks/
├── apitwin.toml            # [[grpc_routes]] for all methods
└── stubs/
    ├── CountryService_GetCountry.json
    ├── CountryService_ListCountries.json
    └── CountryService_CreateCountry.json
```

---

## Stub synthesis

Stubs are synthesised from the output message descriptor — field names, types, and common naming patterns produce sensible placeholder values:

| Field name pattern | Synthesised value |
|---|---|
| contains `id` | `"{{uuid}}"` |
| contains `email` | `"user@example.com"` |
| contains `url` / `uri` | `"https://example.com"` |
| contains `time` / `at` / `date` | `"{{now}}"` |
| contains `name` | `"Example Name"` |
| `bool` type | `true` |
| `int32` / `int64` etc. | `1` |
| `float` / `double` | `1.0` |

---

## Flags

| Flag | Default | Description |
|---|---|---|
| `--proto <file>` | — | Path to a `.proto` file; repeat for multiple files |
| `--import-path <dir>` | — | Extra directory for proto imports; repeat for multiple |
| `--out <dir>` | `mocks` | Output directory |
| `--format <fmt>` | `toml` | Config format: `toml`, `yaml`, `json` |

---

## Serve immediately

After generating, start the server:

```sh
apitwin --config ./mocks --grpc-proto geo.proto
```

---

**See also:** [gRPC Quick Start](quick-start.md) | [Configuration](config.md) | [OpenAPI Generation](../openapi/generate.md)
