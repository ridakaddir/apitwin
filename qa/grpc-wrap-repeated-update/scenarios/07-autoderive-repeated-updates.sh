#!/usr/bin/env bash
# 07-autoderive-repeated-updates
# Same as scenario 02 (wrapped request + wrapped response) but with source
# intentionally OMITTED from the apitwin.toml route so auto-derive has to do
# ALL the work. If applyGRPCPersist caches or mutates the descriptor-derived
# values across calls, it would show up here.
#
# Re-uses UpdateCountryWrapped (source="country" is set) — so this scenario
# is redundant with 02 unless we reconfigure the route. To keep the harness
# self-contained and not touch the route mid-run, we instead drive the
# FlatToWrapped route in the same way and prove auto-derive stays stable.
set -euo pipefail

SCENARIO_ID="07-autoderive-repeated-updates"
scenario_start

EXPECTED="Three alternating-field updates on the flat→wrapped route stay coherent."

call_grpc "geography.GeographyService/CreateCountryWrapped" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "create failed"
  return 0 2>/dev/null || exit 0
}

stub_file="$STUBS_DIR/countries_wrap/FR.json"

# Alternate which field changes each call — sometimes name, sometimes continent.
PAYLOADS=(
  '{"code":"FR","name":"Republique"}'
  '{"code":"FR","continent":"EU-West"}'
  '{"code":"FR","name":"Fifth Republic"}'
)

for i in 0 1 2; do
  call_grpc "geography.GeographyService/UpdateCountryFlatToWrapped" "${PAYLOADS[$i]}"
  RC=$GRPC_RC
  if [[ $RC -ne 0 ]]; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    record_result "$SCENARIO_ID" fail "$EXPECTED" \
      "update #$((i+1)) failed rc=$RC stderr=$(printf '%s' "$GRPC_STDERR" | tr '\n' ' ')" \
      "BUG at N=$((i+1)); payload=${PAYLOADS[$i]}; stub=$(cat "$stub_file" 2>/dev/null || echo MISSING)"
    return 0 2>/dev/null || exit 0
  fi
done

# After 3 alternating updates we expect: name='Fifth Republic' (last),
# continent='EU-West' (set by #2), and original France-era fields.
name_final=$(jq -r '.name // ""' "$stub_file")
continent_final=$(jq -r '.continent // ""' "$stub_file")
if [[ "$name_final" != "Fifth Republic" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "name='$name_final' continent='$continent_final' final stub=$(cat "$stub_file")" \
    "Shallow merge lost name from update #3."
  return 0 2>/dev/null || exit 0
fi
if [[ "$continent_final" != "EU-West" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "continent='$continent_final' final stub=$(cat "$stub_file")" \
    "Shallow merge lost continent from update #2."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "alternating field updates coherent — name='$name_final' continent='$continent_final'" ""
