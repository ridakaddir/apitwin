#!/usr/bin/env bash
# 05-rapid-fire-wrapped
# Fire 10 wrapped updates serially with no deliberate sleep between them.
# Same serial pattern as 03 but denser and longer — if the earlier scenarios
# were green because of lucky timing, this turns up the heat.
set -euo pipefail

SCENARIO_ID="05-rapid-fire-wrapped"
scenario_start

EXPECTED="10 back-to-back wrapped updates all OK; final stub reflects 10th."

call_grpc "geography.GeographyService/CreateCountryWrapped" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "create failed"
  return 0 2>/dev/null || exit 0
}

stub_file="$STUBS_DIR/countries_wrap/FR.json"

for i in $(seq 1 10); do
  n="rapid-$i"
  call_grpc "geography.GeographyService/UpdateCountryWrapped" \
    "{\"country\":{\"code\":\"FR\",\"name\":\"$n\"}}"
  RC=$GRPC_RC
  if [[ $RC -ne 0 ]]; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    record_result "$SCENARIO_ID" fail "$EXPECTED" \
      "update #$i failed rc=$RC stderr=$(printf '%s' "$GRPC_STDERR" | tr '\n' ' ')" \
      "BUG at rapid N=$i; stub currently: $(cat "$stub_file" 2>/dev/null || echo MISSING)"
    return 0 2>/dev/null || exit 0
  fi
  if ! assert_field_eq "$GRPC_STDOUT" "country.name" "$n" "rapid #$i"; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
      "Response rapid #$i wrong — stdout=$GRPC_STDOUT"
    return 0 2>/dev/null || exit 0
  fi
done

if ! jq -e '.name == "rapid-10"' "$stub_file" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "final stub: $(cat "$stub_file")" \
    "Final name on disk does not match 10th update."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "10 rapid wrapped updates OK; final 'rapid-10'" ""
