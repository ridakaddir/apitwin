# OpenAPI Generation

[Home](/) > OpenAPI

---

Generate a complete apitwin config directory from an OpenAPI 3 spec in one command. Works with local files and remote URLs.

```sh
apitwin generate --spec openapi.yaml --out ./mocks
apitwin --config ./mocks
```

Your mock server is running with routes for every operation in the spec — no editing required.

## Documentation

| Page | Description |
|---|---|
| [Generate Command](generate.md) | Full workflow, flags, output structure, and Task usage |
| [Stub Quality](stub-quality.md) | How stubs are populated and format hints for synthesis |
