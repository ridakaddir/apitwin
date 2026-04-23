#!/usr/bin/env bash
# 04-past-due-pending-mutation-applies-on-boot:
# POST /towns-short (5s "ready" delay). Stop within 2s. Wait 10s so fireAt is
# past. Restart. GET /towns-short/{code} returns transitioned payload
# immediately (applied synchronously on boot). scheduled.json is empty after.
set -euo pipefail

SCENARIO_ID="04-past-due-pending-mutation-applies-on-boot"
scenario_start

EXPECTED="Past-due pending mutation applies synchronously on boot"

reset_stubs
if ! start_server; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "initial start failed" \
    "apitwin failed to start — see log"
  return 0 2>/dev/null || exit 0
fi

# Create town with 5s delay.
call_http POST /towns-short '{"code":"NYC","name":"New York"}'
if ! assert_http_status 201 "POST town NYC"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "POST failed"
  return 0 2>/dev/null || exit 0
fi

# Verify initial status.
if ! assert_field_eq "$HTTP_BODY" "status" "provisioning" "immediate status"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "POST defaults payload did not apply"
  return 0 2>/dev/null || exit 0
fi

# Stop quickly (well within 5s).
sleep 1
stop_server

# Verify scheduled.json has the pending item.
SCHED_FILE="$META_DIR/scheduled.json"
if [[ ! -f "$SCHED_FILE" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "scheduled.json missing at $SCHED_FILE after stop" \
    "pending mutation was not persisted — SetPersister wiring broken"
  return 0 2>/dev/null || exit 0
fi
PENDING_COUNT=$(jq -r '.items | length' "$SCHED_FILE" 2>/dev/null || echo 0)
if [[ "$PENDING_COUNT" != "1" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "scheduled.json has $PENDING_COUNT items, expected 1" \
    "pending mutation not persisted correctly"
  return 0 2>/dev/null || exit 0
fi

# Wait past fireAt.
sleep 10

# Restart; past-due item should fire synchronously before the server accepts
# traffic, so the very first GET should already see ready.
if ! restart_server; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "restart failed" \
    "apitwin failed to restart — see log"
  return 0 2>/dev/null || exit 0
fi

call_http GET /towns-short/NYC
if ! assert_http_status 200 "GET NYC post-restart"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "GET failed post-restart"
  return 0 2>/dev/null || exit 0
fi

if ! assert_field_eq "$HTTP_BODY" "status" "ready" "ready status"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Past-due mutation did not apply synchronously on boot — Rearm didn't fire it"
  return 0 2>/dev/null || exit 0
fi

# scheduled.json should now be empty.
POST_COUNT=$(jq -r '.items | length' "$SCHED_FILE" 2>/dev/null || echo 0)
if [[ "$POST_COUNT" != "0" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "scheduled.json still has $POST_COUNT items after boot-sync" \
    "Rearm did not remove past-due items after firing them"
  return 0 2>/dev/null || exit 0
fi

stop_server
SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "Past-due mutation fired on boot; scheduled.json cleared" ""
