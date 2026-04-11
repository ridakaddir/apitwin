# OpenAPI Generate Command

---

## Usage

```sh
# From a local file
apitwin generate --spec openapi.yaml --out ./mocks

# From a remote URL
apitwin generate --spec https://petstore3.swagger.io/api/v3/openapi.json --out ./mocks

# YAML format, single file instead of one per tag
apitwin generate --spec openapi.yaml --format yaml --split=false
```

Then serve immediately:

```sh
apitwin --config ./mocks
```

---

## What is generated

For each path + operation in the spec:

- **One config file per tag** (e.g. `countries.toml`, `cities.toml`) containing one route per operation
- **One stub JSON file per response status code** in `stubs/`
- OpenAPI path parameters (`{id}`) are converted to apitwin wildcards (`*`)
- The first 2xx response is set as the route `fallback`

---

## Example output

For the Petstore spec:

```
mocks/
├── pet.toml        # 13 routes
├── store.toml      #  3 routes
├── user.toml       #  3 routes
└── stubs/
    ├── get_pet_petId_200.json
    ├── get_pet_findByStatus_200.json
    ├── post_pet_200.json
    └── ... (42 more)
```

---

## Generated config example

**`pet.toml`:**

```toml
# Generated from openapi.yaml
# Tag: pet

# Returns pets based on status
[[routes]]
method   = "GET"
match    = "/pet/findByStatus"
enabled  = true
fallback = "success"

  [routes.cases.success]
  status = 200
  file   = "stubs/get_pet_findByStatus_200.json"

  [routes.cases.bad_request]
  status = 400
  file   = "stubs/get_pet_findByStatus_400.json"
```

---

## Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--spec <file\|url>` | `-s` | — | OpenAPI spec file path or URL |
| `--out <dir>` | `-o` | `mocks` | Output directory for config and stubs |
| `--format <fmt>` | `-f` | `toml` | Config format: `toml`, `yaml`, `json` |
| `--split` | | `true` | One file per tag; `--split=false` for a single file |

---

## Using Task

```sh
task generate SPEC=openapi.yaml
task generate SPEC=https://petstore3.swagger.io/api/v3/openapi.json
task generate SPEC=openapi.yaml OUT=./petstore
```

---

**See also:** [Stub Quality](stub-quality.md) | [Quick Start](../quick-start.md) | [CLI Reference](../cli-reference.md)
