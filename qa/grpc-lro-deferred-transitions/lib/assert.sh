#!/usr/bin/env bash
# assert.sh — assertion helpers + result recording.
set -euo pipefail

# record_result ID STATUS EXPECTED ACTUAL HYPOTHESIS
# Appends a JSON line to $RESULTS_FILE.
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

# assert_eq ACTUAL EXPECTED LABEL
assert_eq() {
  local actual="$1" expected="$2" label="${3:-assert_eq}"
  if [[ "$actual" != "$expected" ]]; then
    ASSERT_LAST_ERR="$label: expected='$expected' actual='$actual'"
    return 1
  fi
  return 0
}

# assert_field_eq JSON DOTPATH EXPECTED LABEL
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

# assert_grpc_ok LABEL
assert_grpc_ok() {
  local label="${1:-assert_grpc_ok}"
  if [[ $GRPC_RC -ne 0 ]]; then
    ASSERT_LAST_ERR="$label: rc=$GRPC_RC stderr=$(printf '%s' "$GRPC_STDERR" | tr '\n' ' ')"
    return 1
  fi
  return 0
}

# assert_grpc_code EXPECTED_CODE_NAME LABEL
# Checks that grpcurl stderr contains the given gRPC code name (e.g. NotFound).
assert_grpc_code() {
  local expected="$1" label="${2:-assert_grpc_code}"
  if [[ $GRPC_RC -eq 0 ]]; then
    ASSERT_LAST_ERR="$label: expected code=$expected but call succeeded"
    return 1
  fi
  if ! printf '%s' "$GRPC_STDERR" | grep -q -i "$expected"; then
    ASSERT_LAST_ERR="$label: expected code=$expected in stderr, got: $(printf '%s' "$GRPC_STDERR" | tr '\n' ' ')"
    return 1
  fi
  return 0
}

# scenario_start — reset timing + assertion state.
scenario_start() {
  SCENARIO_T0_S=$(python3 -c "import time; print(time.time())")
  ASSERT_LAST_ERR=""
}

# scenario_elapsed_ms — prints elapsed ms since scenario_start.
scenario_elapsed_ms() {
  python3 -c "import time; print(int((time.time() - $SCENARIO_T0_S) * 1000))"
}
