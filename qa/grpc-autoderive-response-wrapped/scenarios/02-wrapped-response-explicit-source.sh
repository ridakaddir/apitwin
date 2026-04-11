#!/usr/bin/env bash
# 02-wrapped-response-explicit-source
#
# Shape B, case 1: UpdateCity resolves to the `updated` fallback case which
# has EXPLICIT source="city" and wrap="city". This must always work — it's
# the user's documented workaround and must stay green pre-fix and post-fix.
# If this fails pre-fix, the whole explicit-source path is broken and the
# fix scope widens. If it fails post-fix, we regressed the explicit path.
set -euo pipefail

SCENARIO_ID="02-wrapped-response-explicit-source"
scenario_start

EXPECTED="UpdateCity with explicit source+wrap persists only the city sub-object, wraps the response, and passes the proto encode round-trip."

# Seed the stub so update has base data to merge into.
stub_file="$STUBS_DIR/cities_wrapped/casablanca.json"
mkdir -p "$(dirname "$stub_file")"
cat > "$stub_file" <<'EOF'
{
  "name": "casablanca",
  "country": "MA",
  "population": 3360000,
  "languages": ["ar", "fr"],
  "status": "ready"
}
EOF

call_grpc "geo.CityService/UpdateCity" \
  '{"parent":{"continent":"africa","country":"MA"},"cityName":"casablanca","city":{"name":"casablanca","population":3400000}}'

if ! assert_grpc_ok "explicit-source update casablanca"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "$ASSERT_LAST_ERR" \
    "Explicit source='city' / wrap='city' path failed — ExtractSourceField or response wrap is broken even for explicit config."
  return 0 2>/dev/null || exit 0
fi

# Response must be wrapped under `city` (grpcurl prints indented JSON).
if ! printf '%s' "$GRPC_STDOUT" | jq -e '(.city.population | tonumber) == 3400000' >/dev/null; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "grpc response not wrapped under city: $GRPC_STDOUT" \
    "applyGRPCPersist returned persistResult unwrapped despite c.Wrap='city'."
  return 0 2>/dev/null || exit 0
fi

# Stub must have merged population, preserved languages, NOT contain parent
# or cityName or a nested city key.
if ! jq -e '(.population | tonumber) == 3400000 and (.languages | length) == 2 and (has("parent") | not) and (has("cityName") | not) and (has("city") | not)' "$stub_file" >/dev/null; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub after explicit-source update: $(cat "$stub_file" | tr -d '\n')" \
    "Explicit source='city' failed to extract the sub-object before merge."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "explicit source+wrap clean" ""
