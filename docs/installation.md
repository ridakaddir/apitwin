# Installation

[Home](README.md)

---

## npm (recommended)

Best for frontend projects. Installs a platform-specific binary as a dev dependency:

```sh
npm install -D @ridakaddir/apitwin
```

Use it via `npx` or in `package.json` scripts:

```sh
npx apitwin --init
```

```json
{
  "scripts": {
    "mock": "apitwin --config ./mocks --target https://api.example.com"
  }
}
```

---

## Binary download

Download a pre-built binary from the [latest release](https://github.com/ridakaddir/apitwin/releases):

**macOS Apple Silicon:**

```sh
curl -L https://github.com/ridakaddir/apitwin/releases/latest/download/apitwin_darwin_arm64.tar.gz | tar xz
sudo mv apitwin /usr/local/bin/
```

**macOS Intel:**

```sh
curl -L https://github.com/ridakaddir/apitwin/releases/latest/download/apitwin_darwin_amd64.tar.gz | tar xz
sudo mv apitwin /usr/local/bin/
```

**Linux x86-64:**

```sh
curl -L https://github.com/ridakaddir/apitwin/releases/latest/download/apitwin_linux_amd64.tar.gz | tar xz
sudo mv apitwin /usr/local/bin/
```

---

## Go install

Requires Go 1.25+:

```sh
go install github.com/ridakaddir/apitwin@latest
```

---

## Build from source

```sh
git clone https://github.com/ridakaddir/apitwin.git
cd apitwin
task build          # requires Task (https://taskfile.dev)
# or
go build -o apitwin .
```

---

## Verify installation

```sh
apitwin --version
apitwin --help
```

---

**Next:** [Quick Start](quick-start.md)
