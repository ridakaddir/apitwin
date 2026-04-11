#!/usr/bin/env bash
# 07-transition-case-missing-defaults
# The transitions.ready case has persist=true, merge=update, but NO defaults.
# Per scheduler.go:67 (`if c.Persist && c.Defaults != ""`), schedule() is skipped.
# Result: status stays "provisioning" forever (no ready transition).
# Acceptable: document as known behaviour. Fail only if behaviour is worse
# (e.g. server crashes, scheduler errors, file is corrupted).
set -euo pipefail

SCENARIO_ID="07-transition-case-missing-defaults"
scenario_start

EXPECTED="Transition case without defaults: either (a) scheduler skips silently and status stays provisioning, OR (b) status transitions. Must not crash or corrupt."

call_grpc "geography.GeographyService/CreateCountryMissingDefaults" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR (no defaults)" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "create failed outright"
  return 0 2>/dev/null || exit 0
}

call_grpc "geography.GeographyService/GetCountryMissingDefaults" '{"code":"FR"}'
assert_grpc_ok "get FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "immediate get failed"
  return 0 2>/dev/null || exit 0
}
if ! assert_field_eq "$GRPC_STDOUT" "status" "provisioning" "initial status"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "initial defaults not applied"
  return 0 2>/dev/null || exit 0
fi

# Wait 3s — scheduler is expected to skip (c.Defaults == "" in scheduler.go:67).
sleep 3

call_grpc "geography.GeographyService/GetCountryMissingDefaults" '{"code":"FR"}'
assert_grpc_ok "get FR after wait" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "get after wait failed"
  return 0 2>/dev/null || exit 0
}

final_status="$(get_field "$GRPC_STDOUT" status)"
SCENARIO_DURATION_MS=$(scenario_elapsed_ms)

if [[ "$final_status" == "ready" || "$final_status" == "provisioning" ]]; then
  record_result "$SCENARIO_ID" pass "$EXPECTED" \
    "final status='$final_status' — scheduler handled missing-defaults case gracefully" \
    "Per scheduler.go:67 the goroutine is skipped when defaults is empty; status stays provisioning (documented)."
else
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "unexpected status '$final_status' — neither provisioning nor ready" \
    "Something else mutated the file — possibly an error path in applyGRPCPersist left a partial file."
fi
