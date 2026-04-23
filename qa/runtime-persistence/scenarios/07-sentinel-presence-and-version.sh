#!/usr/bin/env bash
# 07-sentinel-presence-and-version:
# After a normal start, verify .apitwin/state/.apitwin-runtime-v1 exists and
# contains valid JSON with {"version": 1, "createdAt": ...}.
set -euo pipefail

SCENARIO_ID="07-sentinel-presence-and-version"
scenario_start

EXPECTED="Sentinel file exists post-start with version=1 and valid createdAt"

reset_stubs
if ! start_server; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "initial start failed" \
    "apitwin failed to start — see log"
  return 0 2>/dev/null || exit 0
fi

SENTINEL="$RUNTIME_DIR/.apitwin-runtime-v1"
if [[ ! -f "$SENTINEL" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "sentinel file missing at $SENTINEL" \
    "OverlaySeed did not write the sentinel on first-ever init"
  return 0 2>/dev/null || exit 0
fi

# Must be valid JSON.
if ! jq -e . "$SENTINEL" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  BODY=$(head -c 400 "$SENTINEL")
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "sentinel is not valid JSON: $BODY" \
    "writeSentinel produced invalid JSON"
  return 0 2>/dev/null || exit 0
fi

VERSION=$(jq -r '.version' "$SENTINEL")
if ! assert_eq "$VERSION" "1" "sentinel version"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Sentinel format version mismatch"
  return 0 2>/dev/null || exit 0
fi

# createdAt must be parseable ISO-8601.
CREATED=$(jq -r '.createdAt' "$SENTINEL")
if [[ -z "$CREATED" || "$CREATED" == "null" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "createdAt missing/null in sentinel" \
    "writeSentinel did not encode createdAt"
  return 0 2>/dev/null || exit 0
fi
if ! python3 -c "import sys, datetime; datetime.datetime.fromisoformat(sys.argv[1].replace('Z','+00:00'))" "$CREATED" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  stop_server
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "createdAt='$CREATED' is not parseable ISO-8601" \
    "writeSentinel emitted an unparseable timestamp"
  return 0 2>/dev/null || exit 0
fi

stop_server
SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "Sentinel present with version=1 and valid createdAt=$CREATED" ""
