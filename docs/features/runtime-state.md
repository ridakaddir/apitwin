# Runtime State Isolation

---

When a mock endpoint uses `persist = true`, apitwin mutates stub files on disk. Without isolation, those mutations land in the same committed `stubs/` directory your team tracks in git, dirtying `git status` after every session and risking accidental pushes of test data.

apitwin solves this by **mirroring the seed stub tree into a gitignored runtime directory** on startup. All reads and writes during a session go through the mirror, so the committed seed files are never touched.

---

## How it works

1. On startup, apitwin copies the entire config-dir tree (minus top-level config files and `.apitwin/` itself) into `.apitwin/state/` next to the config.
2. Every stub read and write is redirected to the runtime mirror.
3. On the next restart, the mirror is wiped and re-created from the seed, giving a clean slate.
4. `.apitwin/state/` is automatically added to the project's `.gitignore` by `--init` and `generate`.

```
my-project/
├── apitwin.toml         # committed config (seed)
├── stubs/
│   └── users/
│       ├── 1.json       # committed seed stub (never mutated at runtime)
│       └── 2.json
├── .apitwin/
│   └── state/           # gitignored runtime mirror
│       └── stubs/
│           └── users/
│               ├── 1.json       # runtime copy (mutations land here)
│               ├── 2.json
│               └── <uuid>.json  # new record from POST — only exists in mirror
└── .gitignore           # includes .apitwin/state/
```

---

## Modes

### Default (runtime mirror)

```sh
apitwin --config ./my-project
```

Creates `.apitwin/state/` next to the config. Mutations go there. Restart = fresh mirror from seed. This is the default — no flags needed.

### Ephemeral

```sh
apitwin --config ./my-project --ephemeral
```

The mirror lives in a system tempdir instead of next to the config. Nothing is written to your project directory at all. The tempdir is removed on shutdown. Useful for CI, demos, and one-shot tests.

### Legacy (no runtime dir)

```sh
apitwin --config ./my-project --no-runtime-dir
```

Disables the mirror entirely. Mutations write back to the seed stubs, exactly like versions prior to v0.2.0-beta.15. Use this for scripts or test harnesses that write directly to `stubs/` and assert against the same paths.

---

## Resetting state

A restart always wipes and re-mirrors, so restarting the server is the easiest way to get back to seed state.

If you want to reset without restarting:

```sh
apitwin reset
apitwin reset --config ./my-project
```

This deletes `.apitwin/state/` so the next run starts fresh.

---

## Record mode

When using `--record`, recorded stubs are **dual-written**: the canonical copy goes to the seed directory (so you can commit it) and a working copy goes to the runtime mirror (so the just-injected route serves it immediately during the current session).

---

## Gitignore

`--init`, `generate`, and the server startup all ensure `.apitwin/state/` is listed in the project's `.gitignore`. If the line is missing, the server logs a one-time warning on startup.

---

**See also:** [Directory-Based Stubs](directory-stubs.md) | [CLI Reference](../cli-reference.md)
