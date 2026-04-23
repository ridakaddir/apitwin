#!/usr/bin/env bash
# 04-unwrapped-baseline
# CONTROL: same N=5 successive updates but against the unwrapped UpdateCountry
# route. If the wrap lane breaks (01-03) but this passes, the bug is isolated
# to the wrap code path in internal/grpc/persist.go.
set -euo pipefail

SCENARIO_ID="04-unwrapped-baseline"
scenario_start

EXPECTED="Five successive unwrapped updates all return OK; final stub reflects 5th."

call_grpc "geography.GeographyService/CreateCountry" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR (unwrapped)" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "create failed"
  return 0 2>/dev/null || exit 0
}

stub_file="$STUBS_DIR/countries/FR.json"
NAMES=("name-1" "name-2" "name-3" "name-4" "name-5")

for i in 0 1 2 3 4; do
  n="${NAMES[$i]}"
  call_grpc "geography.GeographyService/UpdateCountry" \
    "{\"code\":\"FR\",\"name\":\"$n\"}"
  RC=$GRPC_RC
  STDERR_CUR="$GRPC_STDERR"
  STDOUT_CUR="$GRPC_STDOUT"
  if [[ $RC -ne 0 ]]; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    record_result "$SCENARIO_ID" fail "$EXPECTED" \
      "update #$((i+1)) failed rc=$RC stderr=$(printf '%s' "$STDERR_CUR" | tr '\n' ' ')" \
      "Unwrapped update failed — control lane is also broken. Bug may not be wrap-specific."
    return 0 2>/dev/null || exit 0
  fi
  if ! assert_field_eq "$STDOUT_CUR" "name" "$n" "update#$((i+1)) response"; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
      "Response #$((i+1)) name mismatch — stdout=$STDOUT_CUR"
    return 0 2>/dev/null || exit 0
  fi
done

if ! jq -e ".name == \"${NAMES[4]}\"" "$stub_file" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "final stub: $(cat "$stub_file")" \
    "Unwrapped final disk mismatch."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "5 unwrapped updates OK; final name='${NAMES[4]}'" ""
