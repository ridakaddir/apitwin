#!/usr/bin/env bash
# 15-client-update-vs-pending-mutation
# Late client update racing the scheduled ready transition. Client writes
# 'name' ~150ms before the scheduler fires at t=2s. Both the client field
# and the scheduler's 'status=ready' should be visible afterwards.
set -euo pipefail

SCENARIO_ID="15-client-update-vs-pending-mutation"
scenario_start

EXPECTED="After race window, name='French Republic' (client), status='ready' (scheduler), continent='Europe' (original) — no field dropped."

T0=$(python3 -c "import time; print(time.time())")

call_grpc "geography.GeographyService/CreateCountry" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "create failed"
  return 0 2>/dev/null || exit 0
}

# Sleep until just before the scheduled mutation fires (t=2s).
sleep_until_relative "$T0" 1.85

call_grpc "geography.GeographyService/UpdateCountry" \
  '{"code":"FR","name":"French Republic"}'
assert_grpc_ok "client update FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Client UpdateCountry failed during the race window — route /UpdateCountry broken."
  return 0 2>/dev/null || exit 0
}

# Wait past the scheduled goroutine (t=2s).
sleep_until_relative "$T0" 2.3

call_grpc "geography.GeographyService/GetCountry" '{"code":"FR"}'
assert_grpc_ok "get after race" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "get after race failed"
  return 0 2>/dev/null || exit 0
}

name_after="$(get_field "$GRPC_STDOUT" name)"
status_after="$(get_field "$GRPC_STDOUT" status)"
continent_after="$(get_field "$GRPC_STDOUT" continent)"

if [[ "$status_after" != "ready" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "status='$status_after' name='$name_after' continent='$continent_after' — scheduler did not apply ready" \
    "Scheduled ready transition did not land — scheduler.go:159 either errored (check log) or was cancelled. If the client Update path calls CancelFile on /UpdateCountry, that would explain it."
  return 0 2>/dev/null || exit 0
fi

if [[ "$name_after" != "French Republic" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "name='$name_after' status='$status_after' continent='$continent_after' — client update was clobbered by scheduler" \
    "Race lost: persist.Update (persist.go:65) has no file locking. The scheduler goroutine read the file BEFORE the client's UpdateCountry landed, merged defaults onto the stale view, and wrote AFTER the client — last-write-wins dropped 'name'. Fix: take a per-file mutex around read-modify-write in persist.Update, or re-read inside a lock."
  return 0 2>/dev/null || exit 0
fi

if [[ "$continent_after" != "Europe" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "continent='$continent_after' name='$name_after' status='$status_after' — original field dropped" \
    "Either client Update or scheduler transition overwrote 'continent' with empty — shallow merge in persist.Update (persist.go:89) should not delete unmentioned keys."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "name='French Republic' status='ready' continent='Europe' — race handled correctly" ""
