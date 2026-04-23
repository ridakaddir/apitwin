#!/usr/bin/env bash
# 03-five-updates-wrapped
# Five successive UpdateCountryWrapped calls. Even if N=2 or N=3 is the bug
# threshold, this will trip it too. Useful for pinpointing the exact N when
# run together with 01 and 02.
set -euo pipefail

SCENARIO_ID="03-five-updates-wrapped"
scenario_start

EXPECTED="All five wrapped updates return OK; final stub reflects 5th value."

call_grpc "geography.GeographyService/CreateCountryWrapped" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "create failed"
  return 0 2>/dev/null || exit 0
}

stub_file="$STUBS_DIR/countries_wrap/FR.json"
NAMES=("name-1" "name-2" "name-3" "name-4" "name-5")
STUB_TRAIL=()
STUB_TRAIL+=("pre: $(cat "$stub_file" 2>/dev/null || echo MISSING)")

for i in 0 1 2 3 4; do
  n="${NAMES[$i]}"
  call_grpc "geography.GeographyService/UpdateCountryWrapped" \
    "{\"country\":{\"code\":\"FR\",\"name\":\"$n\"}}"
  RC=$GRPC_RC
  STDERR_CUR="$GRPC_STDERR"
  STDOUT_CUR="$GRPC_STDOUT"
  STUB_TRAIL+=("after #$((i+1)): $(cat "$stub_file" 2>/dev/null || echo MISSING)")
  if [[ $RC -ne 0 ]]; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    trail=$(printf '%s\n' "${STUB_TRAIL[@]}" | tr '\n' '|')
    record_result "$SCENARIO_ID" fail "$EXPECTED" \
      "update #$((i+1)) failed rc=$RC stderr=$(printf '%s' "$STDERR_CUR" | tr '\n' ' ')" \
      "BUG at N=$((i+1)). trail: $trail"
    return 0 2>/dev/null || exit 0
  fi
  if ! assert_field_eq "$STDOUT_CUR" "country.name" "$n" "update#$((i+1)) response"; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
      "Response #$((i+1)) wrong — stdout=$STDOUT_CUR"
    return 0 2>/dev/null || exit 0
  fi
done

if ! jq -e ".name == \"${NAMES[4]}\"" "$stub_file" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "final stub: $(cat "$stub_file")" \
    "Final name on disk does not match update #5."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "5 wrapped updates OK; final name='${NAMES[4]}'" ""
