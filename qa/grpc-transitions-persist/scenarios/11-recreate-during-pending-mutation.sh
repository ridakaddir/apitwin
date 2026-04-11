#!/usr/bin/env bash
# 11-recreate-during-pending-mutation
# Tight variant of 03. Create at t=0, delete at t~0.5s, recreate at t~0.55s.
# Original goroutine was scheduled for t=2s; recreate's goroutine for t~2.55s.
# At t=2.1s, the stale goroutine from the original create will still fire on
# the recreated file and stamp it "ready" too early — the recreate's window
# (~2s from t=0.55s) is not yet over.
set -euo pipefail

SCENARIO_ID="11-recreate-during-pending-mutation"
scenario_start

EXPECTED="At t~2.1s post initial-create, the recreated FR (created at ~0.55s) is still 'provisioning'. Stale original goroutine must not stomp."

T0=$(python3 -c "import time; print(time.time())")

call_grpc "geography.GeographyService/CreateCountry" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create 1" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "create failed"
  return 0 2>/dev/null || exit 0
}

sleep_until_relative "$T0" 0.5

call_grpc "geography.GeographyService/DeleteCountry" '{"code":"FR"}'
assert_grpc_ok "delete" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "delete failed"
  return 0 2>/dev/null || exit 0
}

# Recreate nearly immediately.
call_grpc "geography.GeographyService/CreateCountry" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "recreate" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "recreate failed"
  return 0 2>/dev/null || exit 0
}

# Sleep until t=2.1s since T0 — original goroutine should have fired by now
# if it wasn't cancelled on delete.
sleep_until_relative "$T0" 2.1

call_grpc "geography.GeographyService/GetCountry" '{"code":"FR"}'
assert_grpc_ok "get at 2.1s" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "get failed at t=2.1s"
  return 0 2>/dev/null || exit 0
}

status_at_2_1="$(get_field "$GRPC_STDOUT" status)"

if [[ "$status_at_2_1" == "ready" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "at t=2.1s status='ready' — the stale original goroutine stomped the recreated file before its own window expired" \
    "The scheduler spawns goroutines per Schedule() call without linking them to the file's lifecycle. On delete (handler.go:163) the transition state is reset but scheduler.go has no Cancel API keyed on filePath — stale goroutines keep firing. Fix: track in-flight scheduled mutations by filePath and cancel on delete."
  return 0 2>/dev/null || exit 0
fi

if [[ "$status_at_2_1" != "provisioning" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "at t=2.1s status='$status_at_2_1' — unexpected value" \
    "file ended in unexpected state"
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "recreate's timeline honoured, no stale stomp" ""
