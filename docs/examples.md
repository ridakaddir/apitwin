# Examples

The `examples/` directory contains runnable examples for each feature. Each example is a self-contained directory with its own config and stubs.

## Running examples

**HTTP examples:**

```sh
apitwin --config examples/<name>
```

**gRPC examples:**

```sh
apitwin --config examples/<name> --grpc-proto examples/<name>/<file>.proto
```

---

## HTTP examples

| Example | What it demonstrates |
|---|---|
| [`examples/basic`](https://github.com/ridakaddir/apitwin/tree/main/examples/basic) | Static stubs, named cases, hot reload |
| [`examples/conditions`](https://github.com/ridakaddir/apitwin/tree/main/examples/conditions) | Body / query / header condition routing |
| [`examples/cross-refs`](https://github.com/ridakaddir/apitwin/tree/main/examples/cross-refs) | Cross-endpoint references with `{{ref:...}}` syntax, filtering, templates, and dynamic refs in defaults |
| [`examples/directory-stubs`](https://github.com/ridakaddir/apitwin/tree/main/examples/directory-stubs) | Directory-based CRUD — each resource as a separate JSON file |
| [`examples/dynamic-files`](https://github.com/ridakaddir/apitwin/tree/main/examples/dynamic-files) | `{source.field}` file path placeholders and named path parameters |
| [`examples/transitions`](https://github.com/ridakaddir/apitwin/tree/main/examples/transitions) | Time-based response transitions and state progression |
| [`examples/record-mode`](https://github.com/ridakaddir/apitwin/tree/main/examples/record-mode) | Proxy + auto-record workflow |
| [`examples/openapi-generate`](https://github.com/ridakaddir/apitwin/tree/main/examples/openapi-generate) | Generate config from OpenAPI spec (Petstore example) |

## gRPC examples

| Example | What it demonstrates |
|---|---|
| [`examples/grpc-mock`](https://github.com/ridakaddir/apitwin/tree/main/examples/grpc-mock) | gRPC unary mock — named cases, error codes, template tokens |
| [`examples/grpc-conditions`](https://github.com/ridakaddir/apitwin/tree/main/examples/grpc-conditions) | gRPC condition routing on request body fields |
| [`examples/grpc-proxy`](https://github.com/ridakaddir/apitwin/tree/main/examples/grpc-proxy) | gRPC selective mock + transparent upstream proxy fallthrough |
| [`examples/grpc-directory-persist`](https://github.com/ridakaddir/apitwin/tree/main/examples/grpc-directory-persist) | gRPC directory-based CRUD — same as directory-stubs but for gRPC |
| [`examples/grpc-source-persist`](https://github.com/ridakaddir/apitwin/tree/main/examples/grpc-source-persist) | gRPC source extraction — persist only a nested sub-object, not the full request |
| [`examples/grpc-wrap`](https://github.com/ridakaddir/apitwin/tree/main/examples/grpc-wrap) | gRPC response wrapping — flat stub files wrapped into proto message shape |
| [`examples/wrap-test`](https://github.com/ridakaddir/apitwin/tree/main/examples/wrap-test) | HTTP response wrapping — same concept for REST APIs |

## Standalone example files

| File | What it demonstrates |
|---|---|
| [`examples/named-params-example.toml`](https://github.com/ridakaddir/apitwin/blob/main/examples/named-params-example.toml) | Named path parameters demonstration |
| [`examples/dynamic-files-example.toml`](https://github.com/ridakaddir/apitwin/blob/main/examples/dynamic-files-example.toml) | Dynamic file resolution patterns |

---

## Detailed usage

See [`examples/README.md`](https://github.com/ridakaddir/apitwin/blob/main/examples/README.md) for complete usage instructions including `curl` and `grpcurl` commands for each example.
