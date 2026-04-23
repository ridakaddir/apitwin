#!/usr/bin/env bash
# 06-flat-request-to-wrapped-response
# Alternative shape: the Update RPC takes a FLAT request (UpdateCountryRequest
# with code/name/status/continent) but the route has wrap="country" set so the
# response comes back wrapped. No source is configured. Auto-derive walks the
# request fields looking for a Country-typed message — finds none — so source
# stays "" and the whole flat request (minus path placeholders) goes through
# filterToEntityFields. 5 successive updates, watch for rot.
set -euo pipefail

SCENARIO_ID="06-flat-request-to-wrapped-response"
scenario_start

EXPECTED="5 flat→wrapped updates OK; stub reflects 5th; response is wrapped."

# Seed via CreateCountryWrapped (writes to stubs/countries_wrap/FR.json).
call_grpc "geography.GeographyService/CreateCountryWrapped" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR (wrapped seed)" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "create failed"
  return 0 2>/dev/null || exit 0
}

stub_file="$STUBS_DIR/countries_wrap/FR.json"

for i in 1 2 3 4 5; do
  n="flat-$i"
  call_grpc "geography.GeographyService/UpdateCountryFlatToWrapped" \
    "{\"code\":\"FR\",\"name\":\"$n\"}"
  RC=$GRPC_RC
  STDERR_CUR="$GRPC_STDERR"
  STDOUT_CUR="$GRPC_STDOUT"
  if [[ $RC -ne 0 ]]; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    record_result "$SCENARIO_ID" fail "$EXPECTED" \
      "update #$i failed rc=$RC stderr=$(printf '%s' "$STDERR_CUR" | tr '\n' ' ')" \
      "BUG at N=$i in flat→wrapped path. stub at failure: $(cat "$stub_file" 2>/dev/null || echo MISSING)"
    return 0 2>/dev/null || exit 0
  fi
  if ! assert_field_eq "$STDOUT_CUR" "country.name" "$n" "flat→wrapped #$i"; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
      "Response #$i wrong — stdout=$STDOUT_CUR"
    return 0 2>/dev/null || exit 0
  fi
done

if ! jq -e '.name == "flat-5"' "$stub_file" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "final stub: $(cat "$stub_file")" \
    "Final name mismatch in flat→wrapped path."
  return 0 2>/dev/null || exit 0
fi
if jq -e 'has("country")' "$stub_file" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub has 'country' wrapper: $(cat "$stub_file")" \
    "Wrap key leaked into persisted file in flat→wrapped path."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "5 flat→wrapped updates OK; final name='flat-5'" ""
