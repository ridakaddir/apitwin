# Examples

Each example is a **directory** of config files. Run any of them with:

```sh
apitwin --config examples/<name>
```

apitwin loads all `.toml`, `.yaml`, `.yml`, and `.json` files in the directory and merges their routes. Files are loaded in alphabetical order — routes defined earlier take priority.

---

## Structure

```
examples/
├── basic/                    # Static stubs, named cases, hot-reload
│   ├── products.toml         # GET /api/products
│   ├── product-detail.toml   # GET /api/products/:id
│   └── stubs/
│
├── conditions/               # Condition-based routing
│   ├── checkout.toml         # POST /api/cart/checkout — body/query/header conditions
│   ├── orders.toml           # POST /api/orders — nested dot-notation body fields
│   └── stubs/
│
├── cross-refs/               # Cross-endpoint references with {{ref:...}} syntax
│   ├── apitwin.toml            # Routes with {{ref:...}} tokens and dynamic refs in defaults
│   └── stubs/
│       ├── models/           # Model data files
│       ├── endpoints/        # Endpoint files referencing models
│       ├── environments/     # Environment-specific models for dynamic refs
│       ├── tenants/          # Tenant-specific configs for dynamic refs
│       ├── defaults/         # Defaults files with dynamic placeholders
│       └── templates/        # Go templates for data transformation
│
├── directory-stubs/          # Directory-based CRUD (one file per resource)
│   ├── apitwin.toml            # Full CRUD with directory aggregation + auto-ID
│   └── stubs/users/          # One JSON file per user
│
├── dynamic-files/            # {source.field} placeholders in file paths
│   ├── apitwin.toml            # GET /api/orders + POST /api/profile
│   └── stubs/
│

│
├── transitions/              # Time-based response transitions
│   └── orders.toml           # GET /orders/* — shipped → out_for_delivery → delivered
│
├── record-mode/              # Proxy + auto-record workflow
│   └── apitwin.toml
│
├── openapi-generate/         # Generate config from an OpenAPI spec
│   └── README.md             # Instructions (generated files not committed)
│
├── grpc-mock/                # gRPC — basic unary mock (UserService)
│   ├── users.proto           # Proto definition
│   ├── apitwin.toml            # [[grpc_routes]] with named cases
│   └── stubs/
│
├── grpc-conditions/          # gRPC — condition routing on request body fields
│   ├── orders.proto
│   ├── apitwin.toml
│   └── stubs/
│
├── grpc-proxy/               # gRPC — selective mock + transparent proxy fallthrough
│   ├── products.proto
│   ├── apitwin.toml
│   └── stubs/
│
├── grpc-directory-persist/   # gRPC — directory-based CRUD
│   ├── items.proto
│   ├── apitwin.toml
│   └── stubs/
│
├── grpc-wrap/                # gRPC — response wrapping (single file + directory)
│   ├── countries.proto
│   ├── apitwin.toml
│   └── stubs/
│
└── wrap-test/                # HTTP — response wrapping with full CRUD
    ├── config.toml
    └── stubs/
```

---

## basic

Serves product data from stub files. Shows hot-reload — change `fallback` in any file and the next request reflects it immediately, no restart.

```sh
apitwin --config examples/basic
```

```sh
http :4000/api/products           # list — via stubs/products.json
http :4000/api/products/1         # detail — via stubs/product-detail.json
```

Switch to empty list: edit `products.toml` and change `fallback = "empty"`.  
Switch to error: change `fallback = "error"`.

---

## conditions

The same endpoint returns different responses depending on who is calling.

```sh
apitwin --config examples/conditions
```

**Checkout** (`checkout.toml`) — body, query, and header conditions:

```sh
http POST :4000/api/cart/checkout user_type=vip       # 200 discounted
http POST :4000/api/cart/checkout user_type=banned    # 403 forbidden
http POST ':4000/api/cart/checkout?version=v2'        # 200 v2 shape
http POST :4000/api/cart/checkout X-User-Role:admin   # 200 full details
http POST :4000/api/cart/checkout user_type=standard  # 200 fallback
```

**Orders** (`orders.toml`) — nested body fields with dot-notation:

```sh
http POST :4000/api/orders payment:='{"method":"crypto"}' items:='[{"id":"1"}]'  # 202
http POST :4000/api/orders payment:='{"method":"card"}' items:='[{"id":"1"}]'    # 201
http POST :4000/api/orders payment:='{"method":"card"}'                           # 422 missing items
```

---

## directory-stubs

Directory-based CRUD where each user is stored as a separate JSON file. Demonstrates auto-ID generation, directory aggregation, individual file operations, and **persist defaults** (enriching created resources with server-generated fields like `{{uuid}}` and `{{now}}`).

```sh
apitwin --config examples/directory-stubs
```

```sh
# List users (directory aggregation)
http :4000/users                                           # returns: [{"userId":"1",...}, {"userId":"2",...}]

# Get specific user (single file read)
http :4000/users/1                                         # returns: {"userId":"1","name":"Alice Johnson",...}

# Create user with explicit ID
http POST :4000/users userId=4 name="Diana Wilson" email="diana@example.com" role=user

# Create user with auto-generated ID and defaults (userId, role, active, createdAt filled in)
http POST :4000/users name="Eve Brown" email="eve@example.com"

# Update user (shallow merge into file)
http PATCH :4000/users/1 email="alice.johnson@newdomain.com" active:=false

# Delete user (remove file)
http DELETE :4000/users/3
```

**Benefits:** Each user is one file, version-control friendly, unlimited scalability, no array size limits.

Reset: `git checkout examples/directory-stubs/stubs/users/`

---

## cross-refs

Cross-endpoint references with the `{{ref:...}}` syntax. Endpoints can reference data from other stub files with optional filtering and template transformation for field renaming.

```sh
apitwin --config examples/cross-refs
```

```sh
# Get all models (basic directory reference)
http :4000/models
# Returns: [{"id":"model-1","modelName":"GPT-4",...}, ...]

# Get production endpoint (filter + template transformation)
http :4000/endpoints/prod
# Returns: {"deployedModels": [{"modelId":"model-1","name":"GPT-4","modelVersion":"1.0"}]}

# Get development endpoint (shows multiple reference types)
http :4000/endpoints/dev
# Returns: {"allModels":[...], "activeModelsOnly":[...]}

# Get staging endpoint (single file + provider filtering)  
http :4000/endpoints/staging
# Returns: {"primaryModel":{...}, "backupModels":[...]}

# Create new endpoint (inline JSON with references)
http POST :4000/endpoints
# Returns: {"availableModels": [active models only]}

# Create environment-specific endpoint with dynamic defaults
http POST :4000/api/acme/environments/prod/endpoints \
  -H "Content-Type: application/json" \
  -H "X-User-Id: alice" \
  -d '{"endpointId": "ep-123", "region": "us-west-2", "version": "v2.1"}'
# Creates endpoint with prod models and ACME tenant config

http POST :4000/api/techcorp/environments/staging/endpoints \
  -H "Content-Type: application/json" \
  -H "X-User-Id: bob" \
  -d '{"endpointId": "ep-456", "region": "eu-west-1", "version": "v1.5"}'
# Creates endpoint with staging models and TechCorp tenant config
```

**Key features demonstrated:**

- **Directory references**: `{{ref:stubs/models/}}` → all models
- **Single file references**: `{{ref:stubs/models/1.json}}` → specific model
- **Filtering**: `{{ref:stubs/models/?filter=status:active}}` → only active models
- **Provider filtering**: `{{ref:stubs/models/?filter=provider:Anthropic}}` → Anthropic models only
- **Template transformation**: Field renaming from `{id, modelName, version}` to `{modelId, name, modelVersion}`
- **Combined filter + template**: Active models with transformed field names
- **Inline JSON support**: References work in config `json = '...'` blocks
- **Dynamic refs in defaults**: `{{ref:environments/{path.env}/models/}}` with request data placeholders
- **Multi-tenant support**: Different configs per tenant using `{path.tenantId}` placeholders
- **Environment-specific data**: Load different models per environment using `{path.env}` placeholders

**Benefits**: Build interconnected APIs where endpoints dynamically reference each other's data. Changes to model data automatically appear in all referencing endpoints. Support multi-tenant, multi-environment scenarios with context-aware defaults.

---

## dynamic-files

File paths contain `{source.field}` placeholders resolved from the request at runtime.

```sh
apitwin --config examples/dynamic-files
```

```sh
# Resolves to stubs/orders-user-alice-orders.json
http ':4000/api/orders?username=alice'

# Resolves to stubs/orders-user-bob-orders.json
http ':4000/api/orders?username=bob'

# File not found → falls through to fallback (empty orders)
http ':4000/api/orders?username=unknown'

# Resolves to stubs/profile-id-1-profile.json (from body.id)
http POST :4000/api/profile id=1

# Add a new user without changing the config:
cp examples/dynamic-files/stubs/orders-user-alice-orders.json \
   examples/dynamic-files/stubs/orders-user-charlie-orders.json
http ':4000/api/orders?username=charlie'
```

---

## transitions

Two modes for simulating state changes over time.

```sh
apitwin --config examples/transitions
```

### Request-time transitions (orders)

Each GET evaluates elapsed time since the first request and serves the matching case. The response changes as time passes.

```sh
http :4000/orders/o123    # t=0s  → shipped
# wait 30 seconds
http :4000/orders/o123    # t=30s → out_for_delivery  
# wait 60 more seconds
http :4000/orders/o123    # t=90s → delivered (stays here)
```

**Watch it change automatically:**

```sh
watch -n 5 'http :4000/orders/o123'
```

### Background transitions (deployments)

POST creates a resource, and apitwin **mutates the file on disk** in the background after a delay. All reads (GET by ID, list) see the updated state.

```sh
# 1. Create a deployment
http POST :4000/deployments/ep-demo \
  deploymentId=dep-001 name="my-app"

# 2. GET immediately → "Deploying"
http :4000/deployments/ep-demo/dep-001

# 3. Wait 15+ seconds, GET again → "Ready"
sleep 16 && http :4000/deployments/ep-demo/dep-001

# 4. List all — also shows "Ready"
http :4000/deployments/ep-demo
```

---

## record-mode

Proxy a real API and automatically record responses as stub files. Each new path is recorded once — subsequent requests to the same path are served from the stub (`via=stub`).

```sh
apitwin --config examples/record-mode \
      --target https://jsonplaceholder.typicode.com \
      --api-prefix /api \
      --record
```

```sh
http :4000/api/posts
http :4000/api/posts/1
http :4000/api/users
http :4000/api/users/1
```

After recording, serve fully offline (no `--target`, no `--record`):

```sh
apitwin --config examples/record-mode --api-prefix /api
```

---

## openapi-generate

Generate a complete apitwin config from the [Swagger Petstore v3](https://petstore3.swagger.io) spec — no config writing required.

```sh
apitwin generate \
  --spec https://petstore3.swagger.io/api/v3/openapi.json \
  --out examples/openapi-generate/mocks
```

Then serve immediately:

```sh
apitwin --config examples/openapi-generate/mocks
```

```sh
http ':4000/pet/findByStatus?status=available'  # 200 — list of pets
http :4000/pet/1                                 # 200 — single pet
http POST :4000/pet name=Rex photoUrls:='[]'     # 200 — create pet
http :4000/store/inventory                       # 200 — inventory map
http :4000/user/johndoe                          # 200 — user profile
```

The generated `mocks/` directory is gitignored — regenerate it any time. See [`openapi-generate/README.md`](openapi-generate/README.md) for the full workflow including format options and hot-reload tips.

---

## grpc-mock

Basic gRPC mocking with named cases, error codes, and template tokens. No upstream server needed. Requires `--grpc-proto` to activate the gRPC server.

> **Prerequisites:** install [grpcurl](https://github.com/fullstorydev/grpcurl) to send gRPC calls from the terminal.

```sh
apitwin --config examples/grpc-mock \
      --grpc-proto examples/grpc-mock/users.proto
```

**GetUser:**

```sh
# Success (stub file)
grpcurl -plaintext -d '{"user_id":"1"}' \
  localhost:50051 users.UserService/GetUser

# Switch to not_found: edit apitwin.toml and change fallback = "not_found"
grpcurl -plaintext -d '{"user_id":"999"}' \
  localhost:50051 users.UserService/GetUser

# Switch to error with delay: change fallback = "error"
grpcurl -plaintext -d '{"user_id":"1"}' \
  localhost:50051 users.UserService/GetUser
```

**ListUsers:**

```sh
grpcurl -plaintext -d '{"page":1,"page_size":10}' \
  localhost:50051 users.UserService/ListUsers

# Empty list: change fallback = "empty" in apitwin.toml
```

**CreateUser (template tokens — {{uuid}} rendered on every call):**

```sh
grpcurl -plaintext \
  -d '{"name":"Diana","email":"diana@example.com","role":"member"}' \
  localhost:50051 users.UserService/CreateUser

# Conflict: change fallback = "already_exists"
```

**Inspect registered services (gRPC reflection is always on):**

```sh
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext localhost:50051 describe users.UserService
```

---

## grpc-conditions

Condition-based routing on decoded protobuf request body fields. Same `source / field / op / value` config as REST, applied to the protojson representation of the incoming message.

```sh
apitwin --config examples/grpc-conditions \
      --grpc-proto examples/grpc-conditions/orders.proto
```

**GetOrder — route by `region` field:**

```sh
# EU region → localised stub
grpcurl -plaintext -d '{"order_id":"o999","region":"eu"}' \
  localhost:50051 orders.OrderService/GetOrder

# US region → localised stub
grpcurl -plaintext -d '{"order_id":"o999","region":"us"}' \
  localhost:50051 orders.OrderService/GetOrder

# No region → default fallback
grpcurl -plaintext -d '{"order_id":"o999"}' \
  localhost:50051 orders.OrderService/GetOrder
```

**PlaceOrder — route by `payment_type`:**

```sh
# Card payment → accepted (code 0 OK)
grpcurl -plaintext \
  -d '{"user_id":"u1","payment_type":"card","amount":99}' \
  localhost:50051 orders.OrderService/PlaceOrder

# Crypto payment → pending review + 1s delay
grpcurl -plaintext \
  -d '{"user_id":"u1","payment_type":"crypto","amount":500}' \
  localhost:50051 orders.OrderService/PlaceOrder
```

**CancelOrder — route by `contains` operator on reason string:**

```sh
# Contains "duplicate" → instant cancel
grpcurl -plaintext \
  -d '{"order_id":"o999","reason":"duplicate order placed"}' \
  localhost:50051 orders.OrderService/CancelOrder

# Missing reason → INVALID_ARGUMENT (code 3)
grpcurl -plaintext \
  -d '{"order_id":"o999"}' \
  localhost:50051 orders.OrderService/CancelOrder

# Change fallback = "too_late" to simulate already-shipped scenario (code 9)
grpcurl -plaintext \
  -d '{"order_id":"o999","reason":"changed mind"}' \
  localhost:50051 orders.OrderService/CancelOrder
```

---

## grpc-proxy

Demonstrates selective mocking with transparent proxy fallthrough:

- **GetProduct** — always served from a local stub
- **ListProducts** — stubbed for `category=electronics`; any other category falls through to `--grpc-target`
- **UpdateProduct** — not mocked at all; always forwarded to `--grpc-target` (or `UNIMPLEMENTED` if no target)

**Mock-only mode (no upstream):**

```sh
apitwin --config examples/grpc-proxy \
      --grpc-proto examples/grpc-proxy/products.proto

# Stubbed — returns local file
grpcurl -plaintext -d '{"product_id":"prod_001"}' \
  localhost:50051 products.ProductService/GetProduct

# Stubbed (category=electronics matches condition)
grpcurl -plaintext -d '{"category":"electronics","limit":5}' \
  localhost:50051 products.ProductService/ListProducts

# No mock → UNIMPLEMENTED (no target configured)
grpcurl -plaintext -d '{"category":"clothing","limit":5}' \
  localhost:50051 products.ProductService/ListProducts
```

**With upstream proxy:**

```sh
apitwin --config examples/grpc-proxy \
      --grpc-proto examples/grpc-proxy/products.proto \
      --grpc-target localhost:9090

# Forwarded to real upstream (clothing category — not mocked)
grpcurl -plaintext -d '{"category":"clothing","limit":5}' \
  localhost:50051 products.ProductService/ListProducts

# Forwarded — no mock for UpdateProduct
grpcurl -plaintext \
  -d '{"product_id":"prod_001","price":29.99,"stock":100}' \
  localhost:50051 products.ProductService/UpdateProduct
```

**Inspect services:**

```sh
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext localhost:50051 describe products.ProductService
```

---

## grpc-directory-persist

gRPC directory-based CRUD where each item is stored as a separate JSON file. Demonstrates protobuf ↔ JSON conversion, directory aggregation, and auto-ID generation.

```sh
apitwin --config examples/grpc-directory-persist \
      --grpc-proto examples/grpc-directory-persist/items.proto
```

```sh
# List all items (directory aggregation)
grpcurl -plaintext -d '{}' localhost:50051 items.ItemService/ListItems

# Get specific item (single file read)  
grpcurl -plaintext -d '{"itemId":"item-1"}' localhost:50051 items.ItemService/GetItem

# Create item with explicit ID (creates item-3.json)
grpcurl -plaintext \
  -d '{"itemId":"item-3","name":"Laptop","description":"High-performance laptop","price":1299.99,"available":true}' \
  localhost:50051 items.ItemService/CreateItem

# Create item with auto-generated ID (UUID injected)
grpcurl -plaintext \
  -d '{"name":"Tablet","description":"Lightweight tablet","price":299.99,"available":true}' \
  localhost:50051 items.ItemService/CreateItem

# Update item (shallow merge into file)
grpcurl -plaintext \
  -d '{"itemId":"item-1","price":179.99,"available":false}' \
  localhost:50051 items.ItemService/UpdateItem

# Delete item (remove file)
grpcurl -plaintext \
  -d '{"itemId":"item-2"}' \
  localhost:50051 items.ItemService/DeleteItem

# Delete non-existent item → NOT_FOUND (gRPC code 5)
grpcurl -plaintext \
  -d '{"itemId":"item-999"}' \
  localhost:50051 items.ItemService/DeleteItem
```

Reset: `git checkout examples/grpc-directory-persist/stubs/items/`

**Features:** Each item is one file, protobuf field mapping (`item_id` ↔ `itemId`), unlimited scalability.

---

## grpc-wrap

Response wrapping for gRPC. Stub files store flat objects (e.g. `{"code": "morocco", "name": "Morocco", "continent": "africa"}`), but the `wrap` field wraps them into the proto message shape (e.g. `{"country": {"code": "morocco", ...}}`).

```sh
apitwin --config examples/grpc-wrap \
      --grpc-proto examples/grpc-wrap/countries.proto
```

**GetCountry (single-file wrap):**

```sh
# File contains {"code":"morocco","name":"Morocco","continent":"africa"}
# Response: {"country": {"code":"morocco","name":"Morocco","continent":"africa"}}
grpcurl -plaintext -d '{"country_code":"morocco"}' \
  localhost:50051 geo.CountryService/GetCountry
```

**ListCountries (directory wrap):**

```sh
# Aggregates all files in stubs/countries/ into {"countries": [...]}
grpcurl -plaintext -d '{}' \
  localhost:50051 geo.CountryService/ListCountries
```

**CreateCountry:**

```sh
grpcurl -plaintext \
  -d '{"code":"canada","name":"Canada","continent":"north-america"}' \
  localhost:50051 geo.CountryService/CreateCountry
```

**UpdateCountry:**

```sh
grpcurl -plaintext \
  -d '{"country_code":"morocco","name":"Kingdom of Morocco"}' \
  localhost:50051 geo.CountryService/UpdateCountry
```

**DeleteCountry:**

```sh
grpcurl -plaintext -d '{"country_code":"canada"}' \
  localhost:50051 geo.CountryService/DeleteCountry
```

Reset: `git checkout examples/grpc-wrap/stubs/countries/`

**Key concept:** The `wrap` field works for both single files and directories. Files on disk stay flat — wrapping is applied only in the response.

---

## wrap-test

HTTP response wrapping with full CRUD. Same concept as `grpc-wrap` but for HTTP routes.

```sh
apitwin --config examples/wrap-test
```

```sh
# GET single — returns {"country": {"code":"morocco","name":"Morocco","continent":"africa"}}
http :4000/countries/morocco

# LIST — returns {"countries": [{...}]}
http :4000/countries

# CREATE — returns {"country": {"code":"canada",...}}
http POST :4000/countries code=canada name=Canada continent=north-america

# UPDATE — returns {"country": {"code":"morocco",...}}
http PATCH :4000/countries/morocco name="Kingdom of Morocco"

# DELETE
http DELETE :4000/countries/canada
```

Reset: `git checkout examples/wrap-test/stubs/countries/`

---

## Directory mode tips

**Split by domain** — each file owns one resource:

```
mocks/
├── auth.toml       # /auth/*
├── users.toml      # /users/*
├── products.toml   # /products/*
└── orders.toml     # /orders/*
```

**Mix formats** — TOML, YAML, and JSON can coexist in the same directory:

```
mocks/
├── auth.toml
├── users.yaml
└── legacy.json
```

**Override order** — files are loaded alphabetically. To force a specific priority, prefix with a number:

```
mocks/
├── 01-base.toml
└── 02-overrides.toml
```
