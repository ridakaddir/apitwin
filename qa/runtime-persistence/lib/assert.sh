#!/usr/bin/env bash
# assert.sh — assertion helpers + result recording (shared shape with
# qa/grpc-transitions-persist/lib/assert.sh so both suites' iteration-*.json
# reports assemble the same way).
set -euo pipefail

# record_result ID STATUS EXPECTED ACTUAL HYPOTHESIS
record_result() {
  local id="$1" status="$2" expected="$3" actual="$4" hypothesis="${5:-}"
  local dur="${SCENARIO_DURATION_MS:-0}"
  jq -nc \
    --arg id "$id" \
    --arg status "$status" \
    --arg expected "$expected" \
    --arg actual "$actual" \
    --arg hypothesis "$hypothesis" \
    --argjson duration_ms "$dur" \
    '{id:$id,status:$status,duration_ms:$duration_ms,expected:$expected,actual:$actual,hypothesis:$hypothesis}' \
    >> "$RESULTS_FILE"
  local status_upper
  status_upper=$(printf '%s' "$status" | tr '[:lower:]' '[:upper:]')
  echo "[$status_upper] $id  (${dur}ms)"
  if [[ "$status" == "fail" ]]; then
    echo "  expected:   $expected"
    echo "  actual:     $actual"
    echo "  hypothesis: $hypothesis"
  fi
}

assert_eq() {
  local actual="$1" expected="$2" label="${3:-assert_eq}"
  if [[ "$actual" != "$expected" ]]; then
    ASSERT_LAST_ERR="$label: expected='$expected' actual='$actual'"
    return 1
  fi
  return 0
}

assert_field_eq() {
  local json="$1" path="$2" expected="$3" label="${4:-assert_field_eq}"
  local actual
  actual="$(get_field "$json" "$path")"
  if [[ "$actual" != "$expected" ]]; then
    ASSERT_LAST_ERR="$label: field=$path expected='$expected' actual='$actual'"
    return 1
  fi
  return 0
}

assert_http_status() {
  local expected="$1" label="${2:-assert_http_status}"
  if [[ "$HTTP_STATUS" != "$expected" ]]; then
    ASSERT_LAST_ERR="$label: expected=$expected got=$HTTP_STATUS body=$(printf '%s' "$HTTP_BODY" | head -c 200)"
    return 1
  fi
  return 0
}

scenario_start() {
  SCENARIO_T0_S=$(python3 -c "import time; print(time.time())")
  ASSERT_LAST_ERR=""
}

scenario_elapsed_ms() {
  python3 -c "import time; print(int((time.time() - $SCENARIO_T0_S) * 1000))"
}
