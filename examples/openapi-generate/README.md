# openapi-generate example

Shows how to generate a complete apitwin config from an OpenAPI spec in one command.
Uses the [Swagger Petstore v3](https://petstore3.swagger.io) as the source spec.

---

## 1. Generate

```sh
apitwin generate \
  --spec https://petstore3.swagger.io/api/v3/openapi.json \
  --out examples/openapi-generate/mocks
```

Or with Task:

```sh
task generate \
  SPEC=https://petstore3.swagger.io/api/v3/openapi.json \
  OUT=examples/openapi-generate/mocks
```

**What gets generated:**

```
examples/openapi-generate/mocks/
├── pet.toml        # 13 routes — /pet, /pet/findByStatus, /pet/{petId} ...
├── store.toml      #  3 routes — /store/inventory, /store/order
├── user.toml       #  3 routes — /user, /user/login, /user/{username}
└── stubs/          # one stub JSON file per response status code
    ├── get_pet_findByStatus_200.json
    ├── get_pet_findByStatus_400.json
    ├── get_pet_petId_200.json
    ├── post_pet_200.json
    └── ... (42 more)
```

---

## 2. Serve

```sh
apitwin --config examples/openapi-generate/mocks
```

All 19 routes are immediately available at `http://localhost:4000`.

---

## 3. Try it

```sh
# List pets by status
http ':4000/pet/findByStatus?status=available'

# Get a pet by id
http :4000/pet/1

# Create a pet
http POST :4000/pet name=Rex photoUrls:='["https://example.com/rex.jpg"]' status=available

# Get store inventory
http :4000/store/inventory

# Get an order
http :4000/store/order/1

# Get a user
http :4000/user/johndoe
```

---

## 4. Switch cases (hot reload)

The generated config has one case per response status code. Change the `fallback`
in any file to simulate a different response — no restart needed.

**Example:** make `GET /pet/{petId}` return 404:

```toml
# In mocks/pet.toml, find the GET /pet/* route and change:
fallback = "not_found"   # was "success"
```

```sh
http :4000/pet/1         # now returns 404
```

Change it back to `"success"` to restore the 200 response.

---

## 5. Generate in a different format

```sh
# YAML format
apitwin generate \
  --spec https://petstore3.swagger.io/api/v3/openapi.json \
  --out examples/openapi-generate/mocks-yaml \
  --format yaml

# Single file instead of one per tag
apitwin generate \
  --spec https://petstore3.swagger.io/api/v3/openapi.json \
  --out examples/openapi-generate/mocks-single \
  --split=false
```

---

## 6. Clean up

The generated `mocks/` directory is gitignored — it is safe to delete and regenerate at any time.

```sh
rm -rf examples/openapi-generate/mocks
rm -rf examples/openapi-generate/mocks-yaml
rm -rf examples/openapi-generate/mocks-single
```
