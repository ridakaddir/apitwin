#!/usr/bin/env bash
# 03-transition-firsthit-survives-restart:
# GET /cities/{code}/status has transitions [initial 3s, ready]. Hit it once
# at t=0 → "initial". Sleep > 3s. Stop, restart. Hit again → "ready" because
# FirstHit timestamp was persisted and elapsed time since it is now > 3s.
# Without persistence, FirstHit would reset and we'd see "initial" again.
set -euo pipefail

SCENARIO_ID="03-transition-firsthit-survives-restart"
scenario_start

EXPECTED="Transition FirstHit timestamp survives restart; t>3s post-restart returns 'ready'"

reset_stubs
if ! start_server; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "initial start failed" \
    "apitwin failed to start — see log"
  return 0 2>/dev/null || exit 0
fi

# First hit at t=0 — should be initial.
T0=$(now_s)
call_http GET /cities/LON/status
if ! assert_http_status 200 "GET /cities/LON/status t=0"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "first-hit request failed"
  return 0 2>/dev/null || exit 0
fi
if ! assert_field_eq "$HTTP_BODY" "status" "initial" "initial status"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "First hit should have returned 'initial' (within transition window)"
  return 0 2>/dev/null || exit 0
fi

# Sleep 4s so FirstHit+3s has passed.
sleep 4

# Stop server — FirstHit should be persisted to .apitwin-meta/transitions.json.
stop_server

TR_FILE="$META_DIR/transitions.json"
if [[ ! -f "$TR_FILE" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "transitions.json missing at $TR_FILE after stop" \
    "FirstHit was not flushed to disk — persistence broken for REST transitions"
  return 0 2>/dev/null || exit 0
fi

# Restart.
if ! restart_server; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "restart failed" \
    "apitwin failed to restart — see log"
  return 0 2>/dev/null || exit 0
fi

# Next hit — since elapsed > 3s, should serve "ready".
call_http GET /cities/LON/status
if ! assert_http_status 200 "GET /cities/LON/status post-restart"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "post-restart request failed"
  return 0 2>/dev/null || exit 0
fi

if ! assert_field_eq "$HTTP_BODY" "status" "ready" "post-restart status"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "FirstHit was not preserved across restart — transition reset to 'initial' on reboot"
  return 0 2>/dev/null || exit 0
fi

stop_server
SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "Post-restart hit at t>3s returned 'ready' as expected" ""
