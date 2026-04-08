# gRPC

[Home](../README.md) > gRPC

---

apitwin supports gRPC mock and proxy alongside the HTTP server — both run in the same process, activated by `--grpc-proto`.

## How it works

1. Provide one or more `.proto` files via `--grpc-proto` — no `protoc` or code generation required
2. Define `[[grpc_routes]]` in your config files alongside existing `[[routes]]`
3. apitwin starts a gRPC server on `--grpc-port` (default 50051) and an HTTP server on `--port` (default 4000)
4. Incoming gRPC calls are matched by full method path, decoded from protobuf to JSON for condition evaluation, and the stub response is encoded back to protobuf wire format
5. Unmatched calls are forwarded to `--grpc-target` if set, or return `UNIMPLEMENTED`

## Key features

- **No protoc** — apitwin parses `.proto` files at runtime using reflection
- **Unary mocking** — stub responses with protojson (JSON with proto field names)
- **`{body.field}` interpolation** — dynamic file paths from request body fields (e.g. `stubs/{body.id}.json`)
- **Directory wrapping** — directory aggregation auto-wraps into the correct response field for list RPCs
- **Condition routing** — route calls to different cases based on request body fields
- **Proxy fallthrough** — forward unmatched methods to an upstream gRPC server
- **Directory persistence** — stateful CRUD operations on directory-based stub files
- **Transitions** — time-based response progression, same as HTTP
- **Server reflection** — `grpcurl` and `grpc-ui` work out of the box
- **Import paths** — `--import-path` flag for resolving proto imports from shared directories

## Documentation

| Page | Description |
|---|---|
| [Quick Start](quick-start.md) | Get a gRPC mock server running in minutes |
| [Configuration](config.md) | `[[grpc_routes]]` format, match patterns, status codes |
| [Stubs & Conditions](stubs.md) | Stub format, conditions, proxy fallthrough |
| [Persistence](persistence.md) | Directory-based CRUD for gRPC |
| [Generation](generation.md) | `apitwin generate --proto` workflow |
