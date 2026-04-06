# Record Mode

[Home](/) > [Features](index) > Record Mode

---

Record mode proxies all requests to a real API, saves each response as a stub file, and immediately starts serving the stub on subsequent requests. The recorded latency is saved as the `delay` field so stubs replay at realistic speed.

## Usage

```sh
apitwin --config ./mocks \
      --target https://restcountries.com/v3.1 \
      --api-prefix /api \
      --record
```

## How it works

Each new path is recorded once:

```
Request 1  →  via=proxy   (real network, e.g. 73ms)
              → stubs/get_countries_morocco.json saved
              → route appended to mocks/recorded.toml
Request 2  →  via=stub    (local file, <1ms)
```

- The **first** request to a path is proxied to the real API
- The response body is saved as a stub file in `stubs/`
- A route entry is appended to `recorded.toml` with the recorded `delay`
- **Subsequent** requests to the same path are served from the stub

## Serve offline

After recording, serve without a network connection:

```sh
apitwin --config ./mocks --api-prefix /api
```

No `--target` and no `--record` — everything is served from the recorded stubs.

---

**See also:** [CLI Reference](../cli-reference.md) | [API Prefix](api-prefix.md) | [Hot Reload](hot-reload.md)
