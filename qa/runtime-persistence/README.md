# Runtime-state persistence QA harness (REST)

End-to-end test harness that exercises the default runtime-dir mode of
apitwin: `.apitwin/state/` is non-destructively overlaid with seed on every
start, preserving runtime-only stubs and persisted transition / schedule
metadata across restarts. `--reset-runtime` is the escape hatch that wipes
everything back to seed-only.

Unlike `qa/grpc-transitions-persist/` (which uses `--no-runtime-dir` and
pre-seeds stubs directly), this suite relies on the runtime mirror exactly
as the user would in production.

Domain: continent / country / city (REST only, curl).

## Run

```bash
cd /path/to/smart-mock-api
go build -o /tmp/apitwin-qa .
bash qa/runtime-persistence/run.sh \
  --binary /tmp/apitwin-qa \
  --iteration 1
```

Flags:

- `--binary PATH` — path to the apitwin binary (default `/tmp/apitwin-qa`)
- `--iteration N` — required; used for report filenames
- `--scenario NAME` — run only a single scenario (e.g. `03-transition-firsthit-survives-restart`)

## Scenarios

1. **01-runtime-only-post-survives-restart** — POST /countries creates a
   runtime-only stub with no seed counterpart. After a clean restart, GET
   still returns it. Proves seed-overlay does not wipe runtime-only files.

2. **02-seed-wins-over-runtime-mutation** — Start with seed
   `stubs/continents/EU.json`. PATCH to mutate. Stop. Edit the on-disk seed
   template. Restart. GET returns the updated seed content. Proves seed
   wins on overlap (so seed updates propagate, runtime mutations are
   transient).

3. **03-transition-firsthit-survives-restart** — GET route with
   `[initial 3s, ready]`. First hit at t=0 serves initial. Sleep 4s. Stop.
   Restart. Next hit serves ready (because FirstHit timestamp is persisted
   to `.apitwin-meta/transitions.json` and elapsed > 3s).

4. **04-past-due-pending-mutation-applies-on-boot** — POST with 5s deferred
   mutation. Stop within 1s, wait 10s, restart. GET returns transitioned
   payload immediately (applied synchronously on boot via `Rearm`).
   `scheduled.json` is empty after.

5. **05-future-pending-mutation-rearms** — POST with 10s deferred mutation.
   Stop within 1s, restart immediately. `scheduled.json` still has the
   pending item. Wait up to 15s — file flips to `ready`. Proves future-due
   items are re-armed with the remaining delay.

6. **06-reset-runtime-wipes-stubs-and-meta** — POST a runtime-only
   resource. Stop. Restart with `--reset-runtime`. GET 404s. `transitions.json`
   / `scheduled.json` are empty. Sentinel file is freshly regenerated
   (different inode).

7. **07-sentinel-presence-and-version** — After normal start,
   `.apitwin/state/.apitwin-runtime-v1` exists and contains valid JSON with
   `{"version": 1, "createdAt": <ISO-8601>}`.

## Outputs

- `report/iteration-N.json` — structured pass/fail report (same shape as
  the sibling gRPC suite).
- `report/iteration-N.jsonl` — raw per-scenario lines.
- `report/iteration-N.log` — full server stdout+stderr + orchestrator notes.

## Design notes

- Ports are picked dynamically (`pick_port`) so parallel suites don't clash.
- The harness never calls `reset_stubs` between restarts inside a scenario.
  Use `restart_server` — it only touches the PID. `reset_stubs` is only
  invoked at the top of each scenario.
- `seed-template/` holds the ground-truth seed `stubs/` contents. On each
  scenario start, `reset_stubs` wipes both `.apitwin/state/` and `stubs/`,
  then copies `seed-template/stubs/` → `stubs/`. This guarantees seed
  content is deterministic while leaving the runtime dir free to be
  mutated/restarted within the scenario.
- Timing: where possible we use 3–10s scales. Tolerances are generous
  (waits up to 15s) to absorb CI jitter.
