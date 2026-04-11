#!/usr/bin/env bash
# 13-client-update-during-provisioning
# Client calls UpdateCountry while the resource is still in "provisioning".
# Expect: the client's name update is preserved AND the background scheduler
# still lands the "ready" transition at t~=2s. Both changes must coexist.
set -euo pipefail

SCENARIO_ID="13-client-update-during-provisioning"
scenario_start

EXPECTED="Client UpdateCountry during provisioning sticks; background ready transition applies on top (name=French Republic, status=ready)."

T0=$(python3 -c "import time; print(time.time())")

call_grpc "geography.GeographyService/CreateCountry" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "CreateCountry failed outright — baseline append broken."
  return 0 2>/dev/null || exit 0
}

# Immediately update name while still in provisioning window.
call_grpc "geography.GeographyService/UpdateCountry" \
  '{"code":"FR","name":"French Republic"}'
assert_grpc_ok "update FR name" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "UpdateCountry returned error during provisioning — route /UpdateCountry (apitwin.toml:62) merge=update may be rejecting the write."
  return 0 2>/dev/null || exit 0
}

# Immediate Get: name should be updated, status should still be provisioning.
call_grpc "geography.GeographyService/GetCountry" '{"code":"FR"}'
assert_grpc_ok "get after update" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Get after update failed."
  return 0 2>/dev/null || exit 0
}

if ! assert_field_eq "$GRPC_STDOUT" "name" "French Republic" "name after update"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Client UpdateCountry did not persist 'name' — persist.Update shallow merge (persist.go:89) should overwrite the field."
  return 0 2>/dev/null || exit 0
fi

if ! assert_field_eq "$GRPC_STDOUT" "status" "provisioning" "status still provisioning"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Status changed before the 2s window expired — either the scheduler fired early or UpdateCountry clobbered 'status' with an empty field (defaults merging bug in grpc persist handler)."
  return 0 2>/dev/null || exit 0
fi

# Wait past the scheduled ready transition (t=2s).
sleep_until_relative "$T0" 2.6

call_grpc "geography.GeographyService/GetCountry" '{"code":"FR"}'
assert_grpc_ok "get after ready" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Get after ready transition failed — file may have been destroyed."
  return 0 2>/dev/null || exit 0
}

status_after="$(get_field "$GRPC_STDOUT" status)"
name_after="$(get_field "$GRPC_STDOUT" name)"

if [[ "$status_after" != "ready" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "status='$status_after' name='$name_after' at t>2.6s — expected status=ready" \
    "Background scheduler did not fire the ready transition (scheduler.go:159 persist.Update) — possibly because the client UpdateCountry ran through persist.Update and did not reset/cancel the pending goroutine, yet the goroutine still failed to run. Check iteration-3.log."
  return 0 2>/dev/null || exit 0
fi

if [[ "$name_after" != "French Republic" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "name='$name_after' status='$status_after' after ready transition — client update was clobbered" \
    "Scheduler's persist.Update (scheduler.go:159) is merging country-ready.json defaults onto a stale in-memory view and overwrote the client's name update. persist.Update (persist.go:65) does a read-modify-write with no locking; if the goroutine read the file BEFORE step 2's update landed and wrote AFTER, last-write-wins loses the 'name' change. Given step 2 runs synchronously and completes before the scheduler fires at t=2s, the scheduler's read should see the updated file — so this would indicate the scheduler snapshots the file at schedule time instead of read-at-fire."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "name='French Republic' status='ready' — client update and scheduled transition coexist" ""
