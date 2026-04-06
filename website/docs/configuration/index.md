# Configuration

[Home](/) > Configuration

---

apitwin supports `.toml`, `.yaml`/`.yml`, and `.json` config files — auto-detected by file extension.

## Single file

Point `--config` at a file:

```sh
apitwin --config apitwin.toml
```

### Minimal example

```toml
[[routes]]
method   = "GET"
match    = "/api/countries"
enabled  = true
fallback = "success"

  [routes.cases.success]
  status = 200
  file   = "stubs/countries.json"

  [routes.cases.empty]
  status = 200
  json   = '{"countries": []}'

  [routes.cases.error]
  status = 500
  json   = '{"message": "Internal Server Error"}'
  delay  = 2
```

---

## Directory config

Point `--config` at a folder and apitwin loads **all** config files in it, merging their routes in alphabetical order. Hot reload watches the whole directory — adding, editing, or removing any file takes effect immediately.

```sh
apitwin --config ./mocks
```

Split routes by domain for clarity:

```
mocks/
├── continents.toml    # /continents/*
├── countries.toml     # /countries/*
└── cities.toml        # /cities/*
```

Mix formats freely — TOML, YAML, and JSON can coexist in the same directory.

---

## Auto-detection

If `--config` is not set, apitwin:

1. Looks for `apitwin.toml` in the current directory
2. Falls back to loading all config files in `.` if none is found

---

## Config structure

A config file contains two top-level arrays:

| Key | Description |
|---|---|
| `[[routes]]` | HTTP route definitions |
| `[[grpc_routes]]` | gRPC route definitions |

Both arrays use the same case/condition/transition structure. They can coexist in the same file.

---

## Further reading

- [Routes](routes.md) — route fields, path matching patterns
- [Cases](cases.md) — case fields, response definitions, persistence
- [Config Formats](formats.md) — TOML, YAML, and JSON side-by-side
