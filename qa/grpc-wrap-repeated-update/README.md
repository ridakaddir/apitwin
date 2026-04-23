# QA harness — gRPC `wrap` + repeated client updates

Targets the claim that "when a gRPC route has `wrap` configured and the client
issues multiple successive updates, the second or third update fails."

## Layout

- `apitwin.toml` — wrapped Create/Get/Update lane plus an unwrapped control lane.
- `geography.proto` — `Country`, `CountryEnvelope`, and Create/Get/Update RPCs
  in both wrapped and unwrapped shapes. Also `UpdateCountryFlatToWrapped` to
  cover the "flat request, wrapped response" misconfiguration.
- `defaults/country-provisioning.json` — single-field default applied on
  create (sets `status="provisioning"`).
- `lib/` — `grpc.sh`, `wait.sh`, `assert.sh` copied from
  `qa/grpc-transitions-persist/lib/`.
- `scenarios/` — one shell script per case; each appends a JSONL row to the
  iteration results file via `record_result`.
- `report/` — per-iteration `.log`, `.jsonl`, and `.json` outputs.

## Scenarios

- `01-two-updates-wrapped` — Create then two wrapped updates.
- `02-three-updates-wrapped` — Create then three wrapped updates.
- `03-five-updates-wrapped` — Create then five wrapped updates.
- `04-unwrapped-baseline` — Control: five unwrapped updates.
- `05-rapid-fire-wrapped` — Ten back-to-back wrapped updates.
- `06-flat-request-to-wrapped-response` — Update request is flat
  (`UpdateCountryRequest`), the route still has `wrap="country"`, response is
  wrapped.
- `07-autoderive-repeated-updates` — Alternating `name`/`continent` fields
  across three updates on the flat→wrapped route.

## Running

```sh
cd /Users/ridakaddir/code/smart-mock-api && go build -o /tmp/apitwin-qa .
bash qa/grpc-wrap-repeated-update/run.sh --binary /tmp/apitwin-qa --iteration 1
```

Single-scenario filter:

```sh
bash qa/grpc-wrap-repeated-update/run.sh --binary /tmp/apitwin-qa --iteration 99 \
  --scenario 02-three-updates-wrapped
```

Each scenario gets its own fresh server + wiped `stubs/` so runs are isolated.
