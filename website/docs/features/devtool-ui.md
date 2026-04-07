# Devtool UI

[Home](/) > [Features](/features/) > Devtool UI

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

The request tester is a slide-out panel that lets you send requests to any route directly from the browser.

### Sending a request

1. Click the **Test** button on a route card
2. The panel opens with the route's method and path pre-filled
3. Edit any of:
   - **Path** — modify path parameters (e.g. replace `{countryId}` with `morocco`)
   - **Query parameters** — add key-value pairs
   - **Headers** — add custom request headers
   - **Body** — edit JSON body for POST/PUT/PATCH requests
4. Click **Send**

### Inspecting the response

The response panel shows:

- **Status code** — e.g. `200 OK`, `404 Not Found`
- **Response headers** — all headers returned by apitwin
- **Response body** — formatted JSON with syntax highlighting
- **Latency** — round-trip time in milliseconds

## Live config updates

The dashboard polls the internal `/__api/routes` endpoint every 3 seconds. When you edit a config file:

1. Save the file
2. The dashboard detects the change on the next poll (within 3 seconds)
3. Route cards update automatically — new routes appear, removed routes disappear, changed cases reflect immediately

No manual refresh needed. This pairs with apitwin's [hot reload](hot-reload.md) — edit your config and see the result in both the terminal and the UI.

## Internal API

The devtool UI is powered by a single read-only endpoint:

```
GET /__api/routes
```

Returns the current config as JSON, including all routes, cases, conditions, and transitions. This endpoint is always available alongside the UI.

> **Note:** The `/__api/` and `/__ui/` paths are reserved by apitwin and cannot be used as mock route patterns.

## Development

To work on the UI itself during apitwin development:

```sh
task ui:dev
```

This starts the Vite dev server on port `5173` with a proxy to `localhost:4000`, enabling hot module replacement for the React app while apitwin handles API requests.

---

**See also:** [Hot Reload](hot-reload.md) | [CLI Reference](../cli-reference.md) | [Configuration](../configuration/)
