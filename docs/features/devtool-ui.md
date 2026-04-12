# Devtool UI

---

apitwin ships with an embedded browser dashboard for inspecting your mocked routes, testing requests, and watching config changes in real time. The UI is a React SPA bundled into the Go binary via `go:embed` — no extra install or flags required.

## Accessing the UI

Start apitwin as usual:

```sh
apitwin --config ./mocks
```

Open your browser at:

```
http://localhost:4000/__ui/
```

The URL uses whatever port apitwin is running on. If you start with `--port 8080`, the UI is at `http://localhost:8080/__ui/`.

## Route dashboard

The dashboard shows every route in your config — both HTTP and gRPC — in a searchable list.

- **Search** — filter routes by method, path, or case name
- **Method badges** — colour-coded `GET`, `POST`, `PUT`, `DELETE`, etc.
- **Path display** — full match pattern including named parameters (e.g. `/countries/{countryId}`)

## Route cards

Click any route to expand its detail card. Each card shows:

| Section | What it displays |
|---|---|
| Method + Path | The route's HTTP method and match pattern |
| Response cases | All named cases with status codes and file/JSON references |
| Conditions | Condition rules (source, field, operator, value) that route to each case |
| Transitions | Time-based state progression between cases (e.g. `pending` at 0s, `approved` at 5s) |
| Fallback | The default case used when no condition matches |

### Example

A route for `GET /countries/{countryId}` might show:

- **Case `detail`** — `200`, file: `countries/{path.countryId}.json`
- **Case `not_found`** — `404`, json: `{"error": "Country not found"}`
- **Fallback** — `detail`
- **Condition** — `path.countryId eq "invalid"` activates `not_found`

## Request tester

The request tester is a slide-out panel that lets you send requests to any route directly from the browser — both HTTP and gRPC.

### HTTP requests

1. Click the **Test** button on an HTTP route card
2. The panel opens with the route's method and path pre-filled
3. Edit any of:
   - **Path** — modify path parameters (e.g. replace `{countryId}` with `morocco`)
   - **Query parameters** — add key-value pairs
   - **Headers** — add custom request headers
   - **Body** — edit JSON body for POST/PUT/PATCH requests
4. Click **Send**

### gRPC requests

1. Click the **Test** button on a gRPC route card
2. The panel opens with the fully-qualified method pre-filled and the input/output message types shown
3. Edit the JSON request message (`{}` by default — all fields optional)
4. Optionally add gRPC **metadata** (`name: value`, one per line)
5. Expand **Input fields** to see the method's top-level request fields with types
6. Click **Invoke**

Behind the scenes the browser POSTs to `/__api/grpc/invoke`; apitwin transcodes the JSON to protobuf, dials its own gRPC server on `localhost`, and returns the decoded JSON response. You do not need `grpcurl` or a browser gRPC runtime — everything flows over HTTP.

Streaming RPCs (client/server/bidi) are not supported by the devtool client yet; the **Invoke** button is disabled for them.

### Inspecting the response

For HTTP:
- **Status code** — e.g. `200 OK`, `404 Not Found`
- **Response headers** — all headers returned by apitwin
- **Response body** — formatted JSON with syntax highlighting
- **Latency** — round-trip time in milliseconds

For gRPC:
- **Status code name** — `OK`, `NOT_FOUND`, `INVALID_ARGUMENT`, etc.
- **Status message** — on non-OK responses, the server's status message
- **Response body** — the decoded response message as JSON
- **Latency** — round-trip time in milliseconds

## Live config updates

The dashboard polls the internal `/__api/routes` endpoint every 3 seconds. When you edit a config file:

1. Save the file
2. The dashboard detects the change on the next poll (within 3 seconds)
3. Route cards update automatically — new routes appear, removed routes disappear, changed cases reflect immediately

No manual refresh needed. This pairs with apitwin's [hot reload](hot-reload.md) — edit your config and see the result in both the terminal and the UI.

## Internal API

The devtool UI is powered by three endpoints under `/__api/`:

| Endpoint | Method | Purpose |
|---|---|---|
| `/__api/routes` | GET | Current config as JSON (HTTP + gRPC routes, cases, conditions, transitions). Always available. |
| `/__api/grpc/methods` | GET | Per-method input/output types and input field descriptors. Returns `503` when apitwin was started without `--grpc-proto`. |
| `/__api/grpc/invoke` | POST | Dispatches a JSON-shaped unary gRPC call. Body: `{method, message, metadata?}`. Response: `{code, message?, response?, latencyMs}`. Returns `503` when gRPC is disabled. |

> **Note:** The `/__api/` and `/__ui/` paths are reserved by apitwin and cannot be used as mock route patterns.

## Development

To work on the UI itself during apitwin development:

```sh
task ui:dev
```

This starts the Vite dev server on port `5173` with a proxy to `localhost:4000`, enabling hot module replacement for the React app while apitwin handles API requests.

---

**See also:** [Hot Reload](hot-reload.md) | [CLI Reference](../cli-reference.md) | [Configuration](../configuration/README.md)
