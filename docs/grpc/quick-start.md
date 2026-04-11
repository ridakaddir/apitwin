# gRPC Quick Start

---

## Generate and serve

The fastest way to start is to generate config and stubs from a `.proto` file:

```sh
# Generate config and stubs
apitwin generate --proto geo.proto --out ./mocks

# Start HTTP (port 4000) + gRPC (port 50051)
apitwin --config ./mocks --grpc-proto geo.proto
```

## With upstream proxy

Stub only the methods you care about — forward everything else to a real gRPC server:

```sh
apitwin --config ./mocks \
      --grpc-proto geo.proto \
      --grpc-target localhost:9090
```

## Inspect with grpcurl

apitwin includes built-in [server reflection](https://grpc.github.io/grpc/core/md_doc_server_reflection_tutorial.html), so `grpcurl`, `grpc-ui`, and other tools work without specifying a proto file:

```sh
# List all services
grpcurl -plaintext localhost:50051 list

# Describe a service
grpcurl -plaintext localhost:50051 describe geo.CountryService

# Describe a message
grpcurl -plaintext localhost:50051 describe geo.GetCountryRequest

# Call a method
grpcurl -plaintext -d '{"country_code":"morocco"}' localhost:50051 geo.CountryService/GetCountry
```

---

## What's next?

- [Configuration](config.md) — `[[grpc_routes]]` format and match patterns
- [Stubs & Conditions](stubs.md) — stub format, condition routing, proxy fallthrough
- [Persistence](persistence.md) — directory-based CRUD for gRPC
- [Generation](generation.md) — `apitwin generate --proto` in detail
