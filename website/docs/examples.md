# Examples

[Home](/) > Examples

---

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
| [`examples/basic`](../examples/basic) | Static stubs, named cases, hot reload |
| [`examples/conditions`](../examples/conditions) | Body / query / header condition routing |
| [`examples/cross-refs`](../examples/cross-refs) | Cross-endpoint references with <code v-pre>{{ref:...}}</code> syntax, filtering, templates, and dynamic refs in defaults |
| [`examples/directory-stubs`](../examples/directory-stubs) | Directory-based CRUD — each resource as a separate JSON file |
| [`examples/dynamic-files`](../examples/dynamic-files) | <code v-pre>{source.field}</code> file path placeholders and named path parameters |
| [`examples/transitions`](../examples/transitions) | Time-based response transitions and state progression |
| [`examples/record-mode`](../examples/record-mode) | Proxy + auto-record workflow |
| [`examples/openapi-generate`](../examples/openapi-generate) | Generate config from OpenAPI spec (Petstore example) |

## gRPC examples

| Example | What it demonstrates |
|---|---|
| [`examples/grpc-mock`](../examples/grpc-mock) | gRPC unary mock — named cases, error codes, template tokens |
| [`examples/grpc-conditions`](../examples/grpc-conditions) | gRPC condition routing on request body fields |
| [`examples/grpc-proxy`](../examples/grpc-proxy) | gRPC selective mock + transparent upstream proxy fallthrough |
| [`examples/grpc-directory-persist`](../examples/grpc-directory-persist) | gRPC directory-based CRUD — same as directory-stubs but for gRPC |
| [`examples/grpc-wrap`](../examples/grpc-wrap) | gRPC response wrapping — flat stub files wrapped into proto message shape |
| [`examples/wrap-test`](../examples/wrap-test) | HTTP response wrapping — same concept for REST APIs |

## Standalone example files

| File | What it demonstrates |
|---|---|
| [`examples/named-params-example.toml`](../examples/named-params-example.toml) | Named path parameters demonstration |
| [`examples/dynamic-files-example.toml`](../examples/dynamic-files-example.toml) | Dynamic file resolution patterns |

---

## Detailed usage

See [`examples/README.md`](../examples/README.md) for complete usage instructions including `curl` and `grpcurl` commands for each example.
