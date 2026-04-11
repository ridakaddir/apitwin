#!/usr/bin/env bash
# 04-repeated-calls-no-corruption
#
# Stress-test: run 10 UpdateCity calls in quick succession, interleaving
# before and after the transition boundary. Mirrors the user's repro loop on
# the gateway proto. On a fixed build all 10 must succeed and the stub must
# remain a valid UpdateCityResponse at every step.
set -euo pipefail

SCENARIO_ID="04-repeated-calls-no-corruption"
scenario_start

EXPECTED="Ten consecutive UpdateCity calls straddling the transition boundary all succeed; stub is never corrupted."

stub_file="$STUBS_DIR/cities_wrapped/rabat.json"
mkdir -p "$(dirname "$stub_file")"
cat > "$stub_file" <<'EOF'
{
  "name": "rabat",
  "country": "MA",
  "population": 577827,
  "languages": ["ar", "fr"],
  "status": "ready"
}
EOF

fail=""
for i in 1 2 3 4 5 6 7 8 9 10; do
  pop=$((577000 + i))
  call_grpc "geo.CityService/UpdateCity" \
    "{\"parent\":{\"continent\":\"africa\",\"country\":\"MA\"},\"cityName\":\"rabat\",\"city\":{\"name\":\"rabat\",\"population\":${pop}}}"
  if [[ $GRPC_RC -ne 0 ]]; then
    fail="attempt ${i}: rc=${GRPC_RC} stderr=$(printf '%s' "$GRPC_STDERR" | tr '\n' ' ')"
    break
  fi
  if jq -e 'has("parent") or has("cityName") or has("city")' "$stub_file" >/dev/null 2>&1; then
    fail="attempt ${i}: stub corrupted: $(cat "$stub_file" | tr -d '\n')"
    break
  fi
  # Straddle the 2s transition boundary: first 3 calls pre-transition, then
  # sleep, then 7 post-transition. This exercises both the fallback (with
  # explicit source/wrap) and the ready case (auto-derive path) in one run.
  if [[ $i -eq 3 ]]; then
    sleep 2.3
  fi
done

if [[ -n "$fail" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$fail" \
    "Response-wrapped auto-derive corrupts the stub once transitions mature. Fix in RequestEntityField must unwrap single-message-field response types."
  return 0 2>/dev/null || exit 0
fi

if ! jq -e '(.population | tonumber) == 577010' "$stub_file" >/dev/null; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "final stub population mismatch: $(cat "$stub_file" | tr -d '\n')" \
    "Some attempts silently skipped the merge."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "10/10 calls clean across transition boundary" ""
