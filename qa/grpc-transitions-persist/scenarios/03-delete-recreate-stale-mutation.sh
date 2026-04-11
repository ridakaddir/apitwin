#!/usr/bin/env bash
# 03-delete-recreate-stale-mutation
# Create FR, delete quickly, recreate — assert recreate's timeline is honoured,
# not polluted by the original goroutine nor by transition state leaking across.
set -euo pipefail

SCENARIO_ID="03-delete-recreate-stale-mutation"
scenario_start

EXPECTED="After delete+recreate, the recreate's own timeline governs status; at t~1.5s after recreate status=provisioning; at t~3s status=ready."

call_grpc "geography.GeographyService/CreateCountry" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "initial create failed"
  return 0 2>/dev/null || exit 0
}

sleep 0.5

call_grpc "geography.GeographyService/DeleteCountry" '{"code":"FR"}'
assert_grpc_ok "delete FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "delete after initial create failed"
  return 0 2>/dev/null || exit 0
}

# Recreate — "t0" for recreate's timeline.
RECREATE_T0=$(python3 -c "import time; print(time.time())")
call_grpc "geography.GeographyService/CreateCountry" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "recreate FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "recreate after delete failed"
  return 0 2>/dev/null || exit 0
}

# Sleep 1s so we're ~1s into the recreate window — still provisioning.
sleep 1

call_grpc "geography.GeographyService/GetCountry" '{"code":"FR"}'
assert_grpc_ok "get during window" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "get during recreate window failed"
  return 0 2>/dev/null || exit 0
}
mid_status="$(get_field "$GRPC_STDOUT" status)"

if [[ "$mid_status" != "provisioning" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "~1s after recreate, FR status is '$mid_status', expected 'provisioning'" \
    "Either the original goroutine stomped the file with 'ready' defaults at t=2s (scheduler.go:96 has no generation check per-resource), or delete's ResetMatch (handler.go:164) was not effective."
  return 0 2>/dev/null || exit 0
fi

# Wait until recreate's own timer should fire.
if ! wait_for_field_eq "geography.GeographyService/GetCountry" '{"code":"FR"}' status ready 4; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "After 4s recreate status still '$WAIT_LAST_VALUE'" \
    "Recreate goroutine did not fire (Schedule called from handler.go:157) — possibly because initial create's goroutine already completed and scheduler state does not re-arm."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "recreate's timeline honoured" ""
