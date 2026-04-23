#!/usr/bin/env bash
# 06-reset-runtime-wipes-stubs-and-meta:
# POST a runtime-only resource. Stop. Restart with --reset-runtime.
# GET returns 404. .apitwin/state/.apitwin-meta/ exists but transitions /
# schedule files are absent or empty. Sentinel is regenerated.
set -euo pipefail

SCENARIO_ID="06-reset-runtime-wipes-stubs-and-meta"
scenario_start

EXPECTED="--reset-runtime wipes runtime-only stubs + meta files, regenerates sentinel"

reset_stubs
if ! start_server; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "initial start failed" \
    "apitwin failed to start — see log"
  return 0 2>/dev/null || exit 0
fi

# Create runtime-only resource.
call_http POST /countries '{"code":"DE","name":"Germany","continent":"EU"}'
if ! assert_http_status 201 "POST DE"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "POST failed"
  return 0 2>/dev/null || exit 0
fi

# Also hit the transition route so FirstHit is persisted.
call_http GET /cities/BCN/status

# Capture old sentinel path + bytes for comparison.
SENTINEL="$RUNTIME_DIR/.apitwin-runtime-v1"
if [[ ! -f "$SENTINEL" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "sentinel missing before restart" \
    "OverlaySeed did not write the sentinel on initial start"
  return 0 2>/dev/null || exit 0
fi
OLD_INODE=$(ls -i "$SENTINEL" | awk '{print $1}')

stop_server

# Restart WITH --reset-runtime.
if ! restart_server --reset-runtime; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "restart with --reset-runtime failed" \
    "binary crashed under --reset-runtime"
  return 0 2>/dev/null || exit 0
fi

# GET DE should 404 — runtime-only resource was wiped.
call_http GET /countries/DE
if [[ "$HTTP_STATUS" == "200" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "GET /countries/DE still returned 200 after --reset-runtime" \
    "Runtime wipe did not remove runtime-only stubs"
  return 0 2>/dev/null || exit 0
fi

# Meta dir exists.
if [[ ! -d "$META_DIR" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    ".apitwin-meta dir missing after --reset-runtime" \
    "Server did not recreate the meta dir"
  return 0 2>/dev/null || exit 0
fi

# transitions.json / scheduled.json should be absent or empty.
TR_FILE="$META_DIR/transitions.json"
if [[ -f "$TR_FILE" ]]; then
  NON_EMPTY=$(jq -r '.entries | to_entries | map(.value.firstHit // {} | length) | add // 0' "$TR_FILE" 2>/dev/null || echo 0)
  if [[ "$NON_EMPTY" != "0" ]]; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    stop_server
    record_result "$SCENARIO_ID" fail "$EXPECTED" \
      "transitions.json has $NON_EMPTY firstHit entries after --reset-runtime" \
      "Meta wipe did not clear transitions"
    return 0 2>/dev/null || exit 0
  fi
fi
SCHED_FILE="$META_DIR/scheduled.json"
if [[ -f "$SCHED_FILE" ]]; then
  ITEMS=$(jq -r '.items | length' "$SCHED_FILE" 2>/dev/null || echo 0)
  if [[ "$ITEMS" != "0" ]]; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    stop_server
    record_result "$SCENARIO_ID" fail "$EXPECTED" \
      "scheduled.json has $ITEMS items after --reset-runtime" \
      "Meta wipe did not clear schedule"
    return 0 2>/dev/null || exit 0
  fi
fi

# Sentinel should be freshly regenerated (different inode from the first one).
if [[ ! -f "$SENTINEL" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "sentinel missing after --reset-runtime" \
    "Sentinel not regenerated after wipe"
  return 0 2>/dev/null || exit 0
fi
NEW_INODE=$(ls -i "$SENTINEL" | awk '{print $1}')
if [[ "$OLD_INODE" == "$NEW_INODE" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "sentinel inode unchanged ($OLD_INODE) — file was not recreated" \
    "--reset-runtime did not actually wipe the runtime dir (it just reused the old file)"
  return 0 2>/dev/null || exit 0
fi

# Validate sentinel content.
VERSION=$(jq -r '.version' "$SENTINEL" 2>/dev/null || echo '')
if [[ "$VERSION" != "1" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "sentinel version=$VERSION, expected 1" \
    "Sentinel format drifted or not JSON"
  return 0 2>/dev/null || exit 0
fi

stop_server
SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "--reset-runtime wiped stubs+meta, sentinel regenerated (version=1)" ""
