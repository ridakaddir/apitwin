#!/usr/bin/env bash
# 02-seed-wins-over-runtime-mutation:
# Seed ships stubs/continents/EU.json with {"code":"EU","name":"Europe"}.
# PATCH runtime to change name to "Mutated". Stop. Edit seed on disk to
# "Europa". Restart. GET returns "Europa" (seed-overlay overwrites runtime
# copy for paths that exist in seed).
set -euo pipefail

SCENARIO_ID="02-seed-wins-over-runtime-mutation"
scenario_start

EXPECTED="Seed file on disk wins over the pre-restart runtime mutation when restarted"

reset_stubs
if ! start_server; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "initial start failed" \
    "apitwin failed to start — see log"
  return 0 2>/dev/null || exit 0
fi

# Confirm seed EU is served first.
call_http GET /continents/EU
if ! assert_http_status 200 "GET EU initial"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Initial seed read failed — seed not mirrored into runtime dir"
  return 0 2>/dev/null || exit 0
fi
if ! assert_field_eq "$HTTP_BODY" "name" "Europe" "seed name"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Seed content mismatch — template not copied correctly"
  return 0 2>/dev/null || exit 0
fi

# PATCH EU to rename it.
call_http PATCH /continents/EU '{"name":"Mutated"}'
if ! assert_http_status 200 "PATCH EU mutation"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "PATCH did not return 200"
  return 0 2>/dev/null || exit 0
fi

# Verify runtime file reflects the mutation.
if ! wait_for_file_field_eq "$RUNTIME_DIR/stubs/continents/EU.json" name "Mutated" 2; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "runtime EU.json never reflected 'Mutated' (last=$WAIT_LAST_VALUE)" \
    "PATCH persist did not land in runtime dir"
  return 0 2>/dev/null || exit 0
fi

# Stop server, edit seed on disk (simulates git pull updating the template).
stop_server
printf '%s\n' '{"code":"EU","name":"Europa"}' > "$SEED_STUBS_DIR/continents/EU.json"

# Restart. Seed overlay should overwrite the runtime copy.
if ! restart_server; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "restart failed" \
    "apitwin failed to restart — see log"
  return 0 2>/dev/null || exit 0
fi

call_http GET /continents/EU
if ! assert_http_status 200 "GET EU post-restart"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "GET failed after restart"
  return 0 2>/dev/null || exit 0
fi

if ! assert_field_eq "$HTTP_BODY" "name" "Europa" "seed wins"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Runtime mutation was not overwritten by updated seed — OverlaySeed is not seed-wins"
  return 0 2>/dev/null || exit 0
fi

stop_server
SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "Seed 'Europa' overwrote runtime 'Mutated' on restart" ""
