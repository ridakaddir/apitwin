#!/usr/bin/env bash
# 05-future-pending-mutation-rearms:
# POST /towns-long (10s delay). Stop within 2s. Restart immediately. Wait
# another 10s. File shows transitioned payload. scheduled.json was non-empty
# just after restart.
set -euo pipefail

SCENARIO_ID="05-future-pending-mutation-rearms"
scenario_start

EXPECTED="Future-due pending mutation re-arms on restart and fires with remaining delay"

reset_stubs
if ! start_server; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "initial start failed" \
    "apitwin failed to start — see log"
  return 0 2>/dev/null || exit 0
fi

call_http POST /towns-long '{"code":"TKY","name":"Tokyo"}'
if ! assert_http_status 201 "POST town TKY"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "POST failed"
  return 0 2>/dev/null || exit 0
fi

# Stop within 2s of POST (well before 10s fireAt).
sleep 1
stop_server

# Restart immediately.
if ! restart_server; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "restart failed" \
    "apitwin failed to restart — see log"
  return 0 2>/dev/null || exit 0
fi

# Just after restart, scheduled.json should still contain the pending item
# (because the fire time is ~9s in the future).
SCHED_FILE="$META_DIR/scheduled.json"
if [[ ! -f "$SCHED_FILE" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "scheduled.json missing after restart" \
    "pending mutation not persisted"
  return 0 2>/dev/null || exit 0
fi
JUST_AFTER_COUNT=$(jq -r '.items | length' "$SCHED_FILE" 2>/dev/null || echo 0)
if [[ "$JUST_AFTER_COUNT" == "0" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "scheduled.json unexpectedly empty just after restart" \
    "Future-due item was dropped instead of re-armed"
  return 0 2>/dev/null || exit 0
fi

# File currently has provisioning status.
if [[ ! -f "$RUNTIME_DIR/stubs/towns_long/TKY.json" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "TKY runtime file missing after restart" \
    "Runtime-only stub was wiped — OverlaySeed destroyed it"
  return 0 2>/dev/null || exit 0
fi
CUR_STATUS=$(jq -r '.status // empty' "$RUNTIME_DIR/stubs/towns_long/TKY.json" 2>/dev/null || echo '')
if [[ "$CUR_STATUS" != "provisioning" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "TKY status is already '$CUR_STATUS' just after restart" \
    "Mutation fired prematurely — fireAt not honoured after re-arm"
  return 0 2>/dev/null || exit 0
fi

# Wait for file to flip to ready, up to 15s (fireAt ~9s away + tolerance).
if ! wait_for_file_field_eq "$RUNTIME_DIR/stubs/towns_long/TKY.json" status ready 15; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "status never flipped to ready (last=$WAIT_LAST_VALUE)" \
    "Re-armed mutation never fired — scheduler.Rearm broken for future items"
  return 0 2>/dev/null || exit 0
fi

# scheduled.json should be empty now.
POST_COUNT=$(jq -r '.items | length' "$SCHED_FILE" 2>/dev/null || echo 0)
if [[ "$POST_COUNT" != "0" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "scheduled.json still has $POST_COUNT items after firing" \
    "Persister did not remove item on fire"
  return 0 2>/dev/null || exit 0
fi

# GET confirms ready served.
call_http GET /towns-long/TKY
if ! assert_http_status 200 "GET TKY after fire"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "GET failed"
  return 0 2>/dev/null || exit 0
fi
if ! assert_field_eq "$HTTP_BODY" "status" "ready" "ready via GET"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "File is ready on disk but GET says otherwise — reader stale"
  return 0 2>/dev/null || exit 0
fi

stop_server
SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "Pending item re-armed and fired with remaining delay" ""
