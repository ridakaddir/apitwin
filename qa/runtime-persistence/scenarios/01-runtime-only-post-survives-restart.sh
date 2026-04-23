#!/usr/bin/env bash
# 01-runtime-only-post-survives-restart:
# POST /countries creates a runtime-only file with no seed counterpart.
# Restart (preserving .apitwin/state/). GET /countries/FR returns 200 with
# the original payload.
set -euo pipefail

SCENARIO_ID="01-runtime-only-post-survives-restart"
scenario_start

EXPECTED="Runtime-only country FR persisted across server restart"

reset_stubs
if ! start_server; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "initial start failed" \
    "apitwin failed to start — see log"
  return 0 2>/dev/null || exit 0
fi

# Create FR via POST.
call_http POST /countries '{"code":"FR","name":"France","continent":"EU"}'
if ! assert_http_status 201 "POST FR"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "POST /countries did not return 201"
  return 0 2>/dev/null || exit 0
fi

# Confirm file exists under runtime dir, not seed dir.
if [[ ! -f "$RUNTIME_DIR/stubs/countries/FR.json" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "runtime file $RUNTIME_DIR/stubs/countries/FR.json missing after POST" \
    "POST did not write to runtime dir — mirror redirect broken"
  return 0 2>/dev/null || exit 0
fi
if [[ -f "$SEED_STUBS_DIR/countries/FR.json" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "seed dir was dirtied by POST" \
    "Runtime-dir mode is supposed to isolate seed; POST wrote back to seed"
  return 0 2>/dev/null || exit 0
fi

# Restart (preserves .apitwin/state/).
if ! restart_server; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "restart failed" \
    "apitwin failed to restart after POST — see log"
  return 0 2>/dev/null || exit 0
fi

call_http GET /countries/FR
if ! assert_http_status 200 "GET FR post-restart"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Runtime-only stub was wiped on restart — OverlaySeed is destructive instead of non-destructive"
  return 0 2>/dev/null || exit 0
fi

if ! assert_field_eq "$HTTP_BODY" "code" "FR" "code"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Post-restart payload mismatch — file survived but contents differ"
  return 0 2>/dev/null || exit 0
fi
if ! assert_field_eq "$HTTP_BODY" "name" "France" "name"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Post-restart payload mismatch — name field was corrupted"
  return 0 2>/dev/null || exit 0
fi

stop_server
SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "FR served with same payload after restart" ""
