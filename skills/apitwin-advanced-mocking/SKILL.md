---
name: apitwin-advanced-mocking
description: Apitwin advanced mocking for HTTP and gRPC APIs. Use when implementing dynamic responses, stateful workflows, CRUD persistence, or time-based transitions. Covers template tokens, directory-based mutations, and complex routing conditions.
---

# Apitwin Advanced Mocking

Mock HTTP and gRPC APIs with dynamic responses, stateful transitions, and persistent CRUD operations.

## When to Apply

- Creating time-based workflow simulations (visa processing, order status)
- Implementing stateful CRUD operations with file persistence
- Building dynamic responses with template tokens and conditions
- Testing complex API scenarios with gRPC and HTTP together

## Critical Rules

**Use `apitwin serve` not `apitwin start`**: The correct command for starting servers

```bash
# WRONG
apitwin start --config ./mocks

# RIGHT
apitwin serve --config ./mocks
```

**gRPC requires proto file**: Must specify proto file for gRPC server

```bash
# WRONG - gRPC won't work
apitwin --config ./mocks

# RIGHT
apitwin --config ./mocks --grpc-proto service.proto
```

**Request-time vs Background transitions**: Different behavior patterns

```toml
# WRONG - GET routes can't have background transitions
[[routes]]
method = "GET"
match = "/orders/{id}"
# transitions here modify files on disk

# RIGHT - GET uses request-time (read-only state progression)
[[routes]]
method = "GET"
match = "/visa-status/{id}"
fallback = "submitted"
[[routes.transitions]]
case = "submitted"
duration = 30
```

## Key Patterns

### Stateful Workflow Simulation

```toml
[[routes]]
method = "GET"
match = "/visa-status/{id}"
enabled = true
fallback = "submitted"

[[routes.transitions]]
case = "submitted"
duration = 30

[[routes.transitions]]
case = "under_review"
duration = 60

[[routes.transitions]]
case = "approved"
# no duration = terminal state

[routes.cases.submitted]
status = 200
json = '{"status": "submitted", "progress": 10}'

[routes.cases.under_review]
status = 200
json = '{"status": "under_review", "progress": 60}'

[routes.cases.approved]
status = 200
json = '{"status": "approved", "progress": 100}'
```

### Directory-Based CRUD with Auto-Generation

```toml
# Create with auto-ID and defaults
[[routes]]
method = "POST"
match = "/users"
fallback = "created"

[routes.cases.created]
status = 201
file = "stubs/users/"
persist = true
merge = "append"
key = "userId"                    # auto-generated if missing
defaults = "defaults/user.json"   # server fields like {{uuid}}, {{now}}

# List all (directory aggregation)
[[routes]]
method = "GET"
match = "/users"
fallback = "list"

[routes.cases.list]
file = "stubs/users/"            # returns array of all .json files

# Update (shallow merge)
[[routes]]
method = "PATCH"
match = "/users/{userId}"
fallback = "updated"

[routes.cases.updated]
file = "stubs/users/{path.userId}.json"
persist = true
merge = "update"
```

### Dynamic Response Conditions

```toml
[[routes]]
method = "POST"
match = "/payments"
fallback = "default"

# Body field matching
[[routes.conditions]]
source = "body"
field = "payment_type"
op = "eq"
value = "crypto"
case = "crypto_pending"

# Header-based routing
[[routes.conditions]]
source = "header"
field = "X-Region"
op = "eq"
value = "EU"
case = "gdpr_response"

# Query parameter routing
[[routes.conditions]]
source = "query"
field = "format"
op = "contains"
value = "brief"
case = "minimal_response"

[routes.cases.crypto_pending]
status = 202
json = '{"status": "pending", "review_time": "24h"}'
delay = 2

[routes.cases.gdpr_response]
status = 200
json = '{"data": "{{uuid}}", "privacy_notice": "EU compliant"}'
```

### gRPC with Directory Persistence

```toml
# Create item
[[grpc_routes]]
match = "/items.ItemService/CreateItem"
fallback = "created"

[grpc_routes.cases.created]
status = 0
file = "stubs/items/"
persist = true
merge = "append"
key = "itemId"
defaults = "defaults/item.json"

# List items
[[grpc_routes]]
match = "/items.ItemService/ListItems"
fallback = "list"

[grpc_routes.cases.list]
file = "stubs/items/"

# Get specific item with conditions
[[grpc_routes]]
match = "/items.ItemService/GetItem"
fallback = "ok"

[[grpc_routes.conditions]]
source = "body"
field = "itemId"
op = "eq"
value = "not-found"
case = "not_found"

[grpc_routes.cases.ok]
status = 0
file = "stubs/items/{body.itemId}.json"

[grpc_routes.cases.not_found]
status = 5                       # gRPC NOT_FOUND
json = '{"message": "item not found"}'
```

### Template Tokens for Dynamic Data

```json
// defaults/user.json
{
  "userId": "{{uuid}}",
  "role": "user",
  "active": true,
  "createdAt": "{{now}}",
  "timestamp": "{{timestamp}}",
  "continent": "{.continent}",         // from request body
  "region": "{header.X-Region}",       // from headers
  "referrer": "{query.ref}",           // from query params
  "related": "{{ref:countries/{.continent}/}}"  // cross-reference
}
```

### Background Transitions (POST Routes)

```toml
# POST creates and starts background mutation
[[routes]]
method = "POST"
match = "/cities"
fallback = "created"

[[routes.transitions]]
case = "pending"
duration = 15

[[routes.transitions]]
case = "verified"
# terminal state

[routes.cases.created]
status = 201
file = "stubs/cities/"
persist = true
merge = "append"
key = "cityId"
defaults = "defaults/city.json"

[routes.cases.verified]
persist = true
merge = "update"
defaults = "defaults/city-verified.json"   # updates existing file
```

## Common Mistakes

- **Using wrong merge type**: `append` for creation, `update` for modification, `delete` for removal
- **Missing gRPC status codes**: Use `0` (OK), `5` (NOT_FOUND), `13` (INTERNAL), not HTTP codes
- **File vs JSON conflict**: Can't use both `file` and `json` in same case
- **Directory path format**: Use trailing slash for directories: `"stubs/users/"` not `"stubs/users"`
- **Template token syntax**: Use double braces `{{uuid}}` not single `{uuid}`