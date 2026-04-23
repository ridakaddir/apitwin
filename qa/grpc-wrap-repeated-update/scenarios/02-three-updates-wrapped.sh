#!/usr/bin/env bash
# 02-three-updates-wrapped
# Seed + three successive UpdateCountryWrapped calls. All must succeed; final
# stub must reflect the third update.
set -euo pipefail

SCENARIO_ID="02-three-updates-wrapped"
scenario_start

EXPECTED="Three successive wrapped updates all return OK; stub reflects 3rd value."

call_grpc "geography.GeographyService/CreateCountryWrapped" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "create failed"
  return 0 2>/dev/null || exit 0
}

stub_file="$STUBS_DIR/countries_wrap/FR.json"
NAMES=("French Republic" "République française" "The Fifth Republic")
STUB_SNAPSHOTS=()
STUB_SNAPSHOTS+=("pre: $(cat "$stub_file" 2>/dev/null || echo MISSING)")

for i in 0 1 2; do
  n="${NAMES[$i]}"
  call_grpc "geography.GeographyService/UpdateCountryWrapped" \
    "{\"country\":{\"code\":\"FR\",\"name\":\"$n\"}}"
  RC=$GRPC_RC
  STDERR_CUR="$GRPC_STDERR"
  STDOUT_CUR="$GRPC_STDOUT"
  STUB_SNAPSHOTS+=("after update #$((i+1)): $(cat "$stub_file" 2>/dev/null || echo MISSING)")
  if [[ $RC -ne 0 ]]; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    trail=$(printf '%s\n' "${STUB_SNAPSHOTS[@]}" | tr '\n' '|')
    record_result "$SCENARIO_ID" fail "$EXPECTED" \
      "update #$((i+1)) failed rc=$RC stderr=$(printf '%s' "$STDERR_CUR" | tr '\n' ' ')" \
      "BUG at N=$((i+1)). stub trail: $trail"
    return 0 2>/dev/null || exit 0
  fi
  if ! assert_field_eq "$STDOUT_CUR" "country.name" "$n" "update#$((i+1)) response"; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
      "Response #$((i+1)) missing/garbled name — stdout=$STDOUT_CUR"
    return 0 2>/dev/null || exit 0
  fi
done

# Final disk state
if ! jq -e ".name == \"${NAMES[2]}\"" "$stub_file" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "final stub: $(cat "$stub_file")" \
    "Final name on disk does not match update #3."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "3 wrapped updates OK; final name='${NAMES[2]}'" ""
