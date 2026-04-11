# gRPC transitions + persistence QA harness

End-to-end test harness that exercises apitwin gRPC routes with transitions
and persistence across 12 failure-mode scenarios.

## Run

```bash
bash qa/grpc-transitions-persist/run.sh \
  --binary /tmp/apitwin-qa/apitwin \
  --iteration 1
```

Flags:

- `--binary PATH` — path to the apitwin binary (default `/tmp/apitwin-qa/apitwin`)
- `--iteration N` — required; used for report filenames
- `--scenario NAME` — run only a single scenario (e.g. `01-happy-path`)
- `--keep-stubs` — do not wipe `stubs/` before runs
- `--shared-server` — keep one server across all scenarios (faster but may
  cross-contaminate — default is to restart between scenarios)

## Outputs

- `report/iteration-N.json` — structured pass/fail report
- `report/iteration-N.jsonl` — raw per-scenario lines
- `report/iteration-N.log` — full server stdout+stderr + orchestrator notes

## Domain

Uses continent/country/city terminology exclusively, via a single
`GeographyService` proto with multiple Create* RPCs so each scenario can
test a distinct route configuration.
