# Quick Start

[Home](README.md) > Quick Start

---

## HTTP — from an OpenAPI spec

Generate a complete mock config from any OpenAPI 3 spec and start serving immediately:

```sh
apitwin generate --spec openapi.yaml --out ./mocks
apitwin --config ./mocks
```

Your mock server is now running at `http://localhost:4000` with routes for every operation in the spec.

> **Tip:** Open `http://localhost:4000/__ui/` in your browser to see all routes, test requests, and watch config changes in real time. The devtool UI is always available — no extra flags needed.

---

## HTTP — manual scaffold

Create a starter config and point your frontend at apitwin:

```sh
apitwin --init
apitwin --target https://api.example.com
```

Your frontend points at `http://localhost:4000`:
- **Matched routes** return mock responses
- **Everything else** proxies to `--target`

Edit `apitwin.toml` to add routes, change responses, or switch between cases — changes apply instantly with no restart.

> **Tip:** Visit `http://localhost:4000/__ui/` for a visual dashboard of all your mocked routes.

### Minimal config example

```toml
[[routes]]
method   = "GET"
match    = "/api/countries"
enabled  = true
fallback = "success"

  [routes.cases.success]
  status = 200
  file   = "stubs/countries.json"

  [routes.cases.empty]
  status = 200
  json   = '{"countries": []}'

  [routes.cases.error]
  status = 500
  json   = '{"message": "Internal Server Error"}'
  delay  = 2
```

Change `fallback` from `"success"` to `"empty"` or `"error"` to switch responses. The next request picks up the change automatically.

### Add geographic data

Create stub files for your mock data:

**`stubs/countries.json`:**
```json
[
  {"code": "morocco", "name": "Morocco", "continent": "africa", "capital": "Rabat"},
  {"code": "germany", "name": "Germany", "continent": "europe", "capital": "Berlin"},
  {"code": "japan", "name": "Japan", "continent": "asia", "capital": "Tokyo"},
  {"code": "canada", "name": "Canada", "continent": "north-america", "capital": "Ottawa"}
]
```

Or use directory-based stubs for full CRUD:

```toml
[[routes]]
method   = "GET"
match    = "/api/countries"
fallback = "list"

  [routes.cases.list]
  file = "stubs/countries/"    # Aggregates all .json files in the directory
```

```
stubs/countries/
├── morocco.json
├── germany.json
├── japan.json
└── canada.json
```

---

## gRPC — from a `.proto` file

Generate config and stubs from a proto file, then start both HTTP and gRPC servers:

```sh
# Generate config + stubs
apitwin generate --proto service.proto --out ./mocks

# Start servers (HTTP on :4000, gRPC on :50051)
apitwin --config ./mocks --grpc-proto service.proto
```

Inspect with `grpcurl`:

```sh
# List all services (via server reflection)
grpcurl -plaintext localhost:50051 list

# Describe a service
grpcurl -plaintext localhost:50051 describe geo.CountryService

# Call a method
grpcurl -plaintext -d '{"country_code":"morocco"}' localhost:50051 geo.CountryService/GetCountry
```

---

## What's next?

- [Devtool UI](features/devtool-ui.md) — visual dashboard for routes and request testing
- [CLI Reference](cli-reference.md) — all flags and subcommands
- [Configuration](configuration/README.md) — config file format and options
- [Features](features/conditions.md) — conditions, persistence, transitions, and more
- [gRPC](grpc/README.md) — gRPC-specific documentation
- [Examples](examples.md) — runnable examples for every feature
