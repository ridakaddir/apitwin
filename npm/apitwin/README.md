# apitwin

A fast, zero-dependency-on-your-app CLI tool to **mock, stub, and proxy HTTP and gRPC APIs** — written in Go.

Point your frontend at `apitwin` instead of the real API. Mock only the endpoints you're building. Forward everything else to the real backend. Switch between response scenarios by editing a config file — changes apply instantly.

## Install

```sh
npm install -D @ridakaddir/apitwin
```

Or use it directly with `npx`:

```sh
npx @ridakaddir/apitwin --init
```

Once installed, the `apitwin` command is available in your project:

```sh
npx apitwin --init
```

## Quick Start

### 1. Scaffold a starter config

```sh
npx apitwin --init
```

This creates a `apitwin.toml` config file and a `stubs/` directory with an example JSON response.

### 2. Start the mock server

```sh
npx apitwin
```

The server starts at `http://localhost:4000` by default.

### 3. Point your frontend at it

Update your API base URL to `http://localhost:4000` during development:

```js
// .env.development
VITE_API_URL=http://localhost:4000

// or in your fetch calls
const response = await fetch("http://localhost:4000/api/users");
```

### 4. Add to your npm scripts

```json
{
  "scripts": {
    "mock": "apitwin --config ./mocks --target https://api.example.com",
    "dev": "concurrently 'npm run mock' 'vite'"
  }
}
```

## Usage

```sh
# Start with default config (apitwin.toml in current directory)
npx apitwin

# Start with a target upstream API
npx apitwin --target https://api.example.com

# Start on a custom port
npx apitwin --port 3001

# Use a config directory (loads and merges all config files)
npx apitwin --config ./mocks

# Strip an API prefix before matching routes
npx apitwin --api-prefix /api

# Record responses from a real API
npx apitwin --target https://api.example.com --record

# Generate config from an OpenAPI spec
npx apitwin generate --spec ./openapi.yaml

# Generate config from a .proto file
npx apitwin generate --proto ./service.proto
```

## Features

- **Route-based mocking** — define routes with named response cases
- **Condition routing** — route based on request body, query params, headers, or path parameters
- **Dynamic file resolution** — serve different stubs based on request data
- **Reverse proxy fallthrough** — unmatched routes forward to the real API
- **Hot reload** — edit config files and changes apply instantly
- **Record mode** — proxy a real API and save responses as stub files
- **Response templating** — `{{uuid}}`, `{{now}}`, `{{timestamp}}` in responses
- **OpenAPI generation** — generate apitwin config from any OpenAPI 3 spec
- **gRPC support** — mock and proxy gRPC services from `.proto` files
- **Multi-format config** — TOML, YAML, or JSON

## Config Example

```toml
[[routes]]
method = "GET"
path   = "/api/users"
fallback = "success"

  [routes.cases.success]
  status = 200
  file   = "stubs/users.json"

  [routes.cases.empty]
  status = 200
  body   = "[]"

  [routes.cases.error]
  status = 500
  body   = '{"error": "Internal server error"}'
```

## Supported Platforms

| OS      | Architecture | Package               |
| ------- | ------------ | --------------------- |
| macOS   | ARM64        | `@ridakaddir/apitwin-darwin-arm64` |
| macOS   | x64          | `@ridakaddir/apitwin-darwin-x64`   |
| Linux   | x64          | `@ridakaddir/apitwin-linux-x64`    |
| Linux   | ARM64        | `@ridakaddir/apitwin-linux-arm64`  |
| Windows | x64          | `@ridakaddir/apitwin-win32-x64`    |
| Windows | ARM64        | `@ridakaddir/apitwin-win32-arm64`  |

The correct binary for your platform is installed automatically via `optionalDependencies`.

## Documentation

For complete documentation, examples, and configuration reference, visit the [main repository](https://github.com/ridakaddir/apitwin).

## License

MIT
