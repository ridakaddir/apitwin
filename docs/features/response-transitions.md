# Response Transitions

[Home](../README.md) > [Features](README.md) > Response Transitions

---

Automatically advance resources through a sequence of states over time. Useful for simulating visa processing, city verification workflows, country admission pipelines, or any state machine.

apitwin supports two transition modes:

| Mode | Defined on | Timer starts | File mutated? | Use case |
|---|---|---|---|---|
| [Request-time](#request-time-transitions) | GET routes | First GET request | No | Read-only state progression |
| [Background](#background-transitions-on-post) | POST routes | Resource creation | Yes | CRUD with lifecycle states |

---

## Request-time transitions

Transitions on a **GET route** serve different cases based on elapsed time since the first request. The response changes automatically — no file mutation, no POST required.

### Example

```toml
[[routes]]
method   = "GET"
match    = "/countries/{countryId}/visa-status"
enabled  = true
fallback = "submitted"

  [[routes.transitions]]
  case     = "submitted"
  duration = 30          # serve for 30 seconds

  [[routes.transitions]]
  case     = "under_review"
  duration = 60          # serve for 60 seconds

  [[routes.transitions]]
  case     = "approved"
  # no duration — terminal state

  [routes.cases.submitted]
  status = 200
  json   = '{"country": "morocco", "status": "submitted"}'

  [routes.cases.under_review]
  status = 200
  json   = '{"country": "morocco", "status": "under_review"}'

  [routes.cases.approved]
  status = 200
  json   = '{"country": "morocco", "status": "approved"}'
```

### Timeline

The timer starts on the **first request** to the route:

```
t = 0s   first request  → submitted       (duration: 30s)
t = 30s  next request   → under_review    (duration: 60s)
t = 90s  next request   → approved        (terminal — stays here)
```

### Behaviour

- **Conditions take priority** — if the route also has [conditions](conditions.md), they are evaluated first. Transitions only activate when no condition matches
- **Shared timeline per route pattern** — all requests to `GET /countries/*/visa-status` share one clock regardless of the specific country (`/countries/morocco/visa-status` and `/countries/canada/visa-status` advance together)
- **Hot reload resets** — editing the config file restarts the sequence from the beginning
- **No looping** — transitions are one-way; the last entry without `duration` is the terminal state

### YAML equivalent

```yaml
routes:
  - method: GET
    match: /countries/{countryId}/visa-status
    enabled: true
    fallback: submitted
    transitions:
      - case: submitted
        duration: 30
      - case: under_review
        duration: 60
      - case: approved
    cases:
      submitted:
        status: 200
        json: '{"country": "morocco", "status": "submitted"}'
      under_review:
        status: 200
        json: '{"country": "morocco", "status": "under_review"}'
      approved:
        status: 200
        json: '{"country": "morocco", "status": "approved"}'
```

---

## Background transitions on POST

Transitions on a **POST route** schedule background file mutations after a resource is created. The file on disk is updated in the background, so all subsequent reads (GET by ID, GET list) reflect the new state automatically.

This is ideal for simulating city verification workflows, country registration pipelines, or any resource that transitions through states after creation.

### Example: city verification lifecycle

```toml
# POST — creates city, starts background transition
[[routes]]
method   = "POST"
match    = "/continents/{continentId}/cities"
fallback = "created"

  [[routes.transitions]]
  case     = "pending"
  duration = 15

  [[routes.transitions]]
  case     = "verified"

  [routes.cases.created]
  status   = 201
  file     = "cities/{path.continentId}/"
  persist  = true
  merge    = "append"
  key      = "cityId"
  defaults = "defaults/city.json"

  [routes.cases.verified]
  persist  = true
  merge    = "update"
  defaults = "defaults/city-verified.json"

# GET by ID — pure read, no transitions needed
[[routes]]
method   = "GET"
match    = "/continents/{continentId}/cities/{cityId}"
fallback = "success"

  [routes.cases.success]
  status = 200
  file   = "cities/{path.continentId}/{path.cityId}.json"

# GET list — directory aggregation, also sees the updated files
[[routes]]
method   = "GET"
match    = "/continents/{continentId}/cities"
fallback = "success"

  [routes.cases.success]
  status = 200
  file   = "cities/{path.continentId}/"
```

With `defaults/city.json`:
```json
{"status": "pending", "continent": "{path.continentId}", "createdAt": "{{now}}"}
```

And `defaults/city-verified.json`:
```json
{"status": "verified"}
```

### Timeline

The timer starts on **resource creation** (the POST request):

```
t = 0s   POST creates file  → {"status": "pending"}
t = 15s  background mutation → merges {"status": "verified"} into the file
```

### Flow

1. **POST** creates the resource → responds 201 with `"status": "pending"`
2. **Background goroutine** sleeps for 15 seconds
3. **After 15s**, apitwin merges `{"status": "verified"}` into the file on disk
4. **Any GET** (by ID or list) now returns `"status": "verified"`

### How it works

- The `fallback` case (`created`) handles the actual POST response
- Transition case names (`pending`, `verified`) are for the scheduler, not for request-time case selection — they don't need to match the `fallback`
- Transition cases with `persist = true` and `defaults` are scheduled as background file mutations
- Each POST spawns **independent background goroutines** — file mutations for different resources happen independently on disk
- **Shared timeline per route pattern** — the transition clock is shared across all resource IDs on the same route (see [below](#transition-state-and-multiple-resources))
- **Hot reload** cancels all pending background mutations
- **Server shutdown** waits for pending mutations to finish gracefully
- If the file is deleted before a transition fires, the mutation is skipped (logged as a warning)
- **DELETE resets transitions** — when a DELETE route with `merge = "delete"` removes a resource, the transition clock for that route pattern is reset so subsequent POSTs start fresh

### Multiple transition stages

Background transitions support multiple stages with cumulative durations:

```toml
[[routes.transitions]]
case     = "pending"
duration = 10               # first 10 seconds

[[routes.transitions]]
case     = "reviewing"
duration = 20               # next 20 seconds (fires at t=10s)

[[routes.transitions]]
case     = "verified"       # fires at t=30s

[routes.cases.reviewing]
persist  = true
merge    = "update"
defaults = "defaults/city-reviewing.json"

[routes.cases.verified]
persist  = true
merge    = "update"
defaults = "defaults/city-verified.json"
```

### Transition state and multiple resources

Transition state is tracked **per route pattern**, not per resource ID. This means `POST /continents/africa/cities` and `POST /continents/europe/cities` share the same transition clock.

In practice, this is handled automatically:

- **Background file mutations are per-file** — each POST spawns its own goroutines that mutate the correct file, regardless of the shared clock
- **Fallback on missing file** — if the transition clock has advanced to a terminal case (e.g. `verified` with `merge = "update"`) but the target file doesn't exist (because this is a new resource), apitwin automatically falls back to the `fallback` case (e.g. `created` with `merge = "append"`) and creates the file normally

This means you can create multiple resources at any time without worrying about the transition clock:

```
t = 0s    POST /cities (body: {name: "Casablanca"})  → 201 (created)
t = 15s   background mutation                         → file updated to "verified"
t = 20s   POST /cities (body: {name: "Berlin"})       → 201 (created, not 404)
          ↑ transition clock is past "pending", but apitwin detects the file
            doesn't exist and uses the fallback "created" case
```

Similarly, the **DELETE → re-create** cycle works correctly:

```
t = 0s    POST /cities (body: {name: "Casablanca"})  → 201 (created)
t = 15s   background mutation                         → file updated to "verified"
t = 20s   DELETE /cities/casablanca                    → 200 (deleted, clock resets)
t = 25s   POST /cities (body: {name: "Casablanca"})  → 201 (created fresh)
```

---

## `transitions` fields

| Field | Type | Required | Description |
|---|---|---|---|
| `case` | string | yes | Case key for this stage (request-time: served as the response; background: used for scheduling) |
| `duration` | int | no | How long this state lasts in seconds. Omit on the last entry for a terminal state |

---

## Examples

See [`examples/transitions/`](../../examples/transitions/) for complete working examples:

- **`visa-status.toml`** — request-time transitions (GET returns different responses over time)
- **`city-verification.toml`** — background transitions (POST creates resource, file mutates on disk after delay)

---

**See also:** [Cases](../configuration/cases.md) | [Conditions](conditions.md) | [gRPC Transitions](../grpc/config.md#transitions)
