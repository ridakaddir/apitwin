# Runtime State Isolation

---

When a mock endpoint uses `persist = true`, apitwin mutates stub files on disk. Without isolation, those mutations land in the same committed `stubs/` directory your team tracks in git, dirtying `git status` after every session and risking accidental pushes of test data.

apitwin solves this by **mirroring the seed stub tree into a gitignored runtime directory** on startup. All reads and writes during a session go through the mirror, so the committed seed files are never touched.

---

## How it works

1. On startup, apitwin copies the seed tree (minus top-level config files and `.apitwin/` itself) on top of `.apitwin/state/` next to the config.
2. Every stub read and write is redirected to the runtime mirror.
3. **The mirror persists across restarts.** On a subsequent start, seed files are re-overlaid (so seed edits flow through), but runtime-only files (e.g. POST-created stubs) are preserved. To force a clean slate, pass `--reset-runtime` or run `apitwin reset` before starting.
4. **Transition state and pending scheduled mutations also persist** in `.apitwin/state/.apitwin-meta/`. Transition `FirstHit` timestamps survive restarts, and any deferred mutations scheduled by a transition timeline are re-armed on the next boot — past-due items fire synchronously.
5. `.apitwin/state/` is automatically added to the project's `.gitignore` by `--init` and `generate`.

```
my-project/
├── apitwin.toml         # committed config (seed)
├── stubs/
│   └── users/
│       ├── 1.json       # committed seed stub (never mutated at runtime)
│       └── 2.json
├── .apitwin/
│   └── state/                    # gitignored runtime mirror
│       ├── .apitwin-runtime-v1   # version sentinel (created on first init)
│       ├── .apitwin-meta/        # persisted transition + schedule state
│       │   ├── transitions.json  # FirstHit timestamps per scope
│       │   └── scheduled.json    # pending deferred mutations
│       └── stubs/
│           └── users/
│               ├── 1.json        # runtime copy (mutations land here)
│               ├── 2.json
│               └── <uuid>.json   # POST-created — runtime-only, survives restarts
└── .gitignore           # includes .apitwin/state/
```

### Seed-vs-runtime conflict resolution

When the same path exists in both seed and runtime:

| Scenario | Outcome |
|---|---|
| File exists only in seed | Mirrored to runtime on every start |
| File exists only in runtime (e.g. POST creation) | Preserved on every start |
| File exists in both, seed edited since last start | **Seed wins** — runtime copy is overwritten |
| File deleted from seed but still in runtime | Runtime copy is preserved (use `--reset-runtime` to clean up) |

---

## Modes

### Default (persistent runtime mirror)

```sh
apitwin --config ./my-project
```

Creates `.apitwin/state/` next to the config on first start. Mutations go there. **Restart preserves runtime state** — POST-created stubs, transition timestamps, and pending deferred mutations all survive. Seed files always win on overlap, so editing a seed stub and restarting does flow the new content through to runtime. This is the default — no flags needed.

### Force-reset on startup

```sh
apitwin --config ./my-project --reset-runtime
```

Wipes `.apitwin/state/` (including the `.apitwin-meta/` transition + schedule state) and re-mirrors from seed before the server starts. Equivalent to running `apitwin reset` and then starting normally.

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

A normal restart preserves runtime state. To get back to a clean seed-only state, choose one:

```sh
# Inline at startup — wipe + re-mirror in one shot
apitwin --reset-runtime --config ./my-project

# Subcommand — wipe while the server is stopped
apitwin reset --config ./my-project
```

Both delete `.apitwin/state/` (including `.apitwin-meta/`) so the next run starts from seed only, with no runtime-only stubs and no persisted transition state.

---

## Record mode

When using `--record`, recorded stubs are **dual-written**: the canonical copy goes to the seed directory (so you can commit it) and a working copy goes to the runtime mirror (so the just-injected route serves it immediately during the current session).

---

## Gitignore

`--init`, `generate`, and the server startup all ensure `.apitwin/state/` is listed in the project's `.gitignore`. If the line is missing, the server logs a one-time warning on startup.

---

**See also:** [Directory-Based Stubs](directory-stubs.md) | [CLI Reference](../cli-reference.md)
