#!/usr/bin/env bash
# 05-source-field-transition
# CreateCountryWithSource uses source="country" so the persisted file must
# contain only the country sub-object (no meta leak). After transition, the
# update case's file path uses {body.country.code}, so the scheduled goroutine
# must still resolve correctly.
set -euo pipefail

SCENARIO_ID="05-source-field-transition"
scenario_start

EXPECTED="CreateCountryWithSource persists only the country sub-object; status transitions provisioning→ready without leaking meta fields."

call_grpc "geography.GeographyService/CreateCountryWithSource" \
  '{"meta":{"requestId":"req-0001"},"country":{"code":"FR","name":"France","continent":"Europe"}}'
assert_grpc_ok "create FR (source)" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "CreateCountryWithSource failed"
  return 0 2>/dev/null || exit 0
}

# Inspect raw stub file for meta leak.
stub_file="$STUBS_DIR/countries_src/FR.json"
if [[ ! -f "$stub_file" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub file $stub_file missing after create" \
    "source-field extraction might break append path resolution in persist.go applyGRPCPersist."
  return 0 2>/dev/null || exit 0
fi

if jq -e 'has("meta") or has("requestId")' "$stub_file" >/dev/null 2>&1; then
  meta_contents="$(cat "$stub_file")"
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub file contains meta: $meta_contents" \
    "Source extraction in persist.go should strip meta — bug in ExtractSourceField or its application."
  return 0 2>/dev/null || exit 0
fi

if ! jq -e '.status == "provisioning"' "$stub_file" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub file status not provisioning immediately: $(cat "$stub_file")" \
    "Defaults file not applied on append with source+wrap."
  return 0 2>/dev/null || exit 0
fi

# Wait 2.5s, then inspect stub for ready status.
sleep 2.5

if ! jq -e '.status == "ready"' "$stub_file" >/dev/null 2>&1; then
  actual="$(cat "$stub_file")"
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "after 2.5s stub status not ready: $actual" \
    "Scheduled goroutine likely failed to resolve file path template {body.country.code} because the scheduler uses the path that was already resolved at append time — it may not need templating again, but this confirms path propagation is wrong, or scheduler.go:73 passed a mis-resolved path."
  return 0 2>/dev/null || exit 0
fi

if jq -e 'has("meta") or has("requestId")' "$stub_file" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "meta leaked into stub after transition: $(cat "$stub_file")" \
    "Deferred update path in scheduler.go schedule() does not re-strip meta."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "source-field transition clean" ""
