#!/usr/bin/env bash
# 03-wrapped-response-autoderive-ready-case
#
# Shape B, case 2 — THE BUG. The `ready` transition case has NO explicit
# source/wrap. After the transition matures (≥ 2s), a real client request
# lands on the `ready` case and applyGRPCPersist runs auto-derive against a
# response-wrapped proto shape (UpdateCityResponse { City city = 1; }).
#
# Pre-fix:  RequestEntityField looks for a request field whose message type
#           equals UpdateCityResponse. No such field exists — the request has
#           a `City city = 3`, whose type is City, not UpdateCityResponse.
#           Auto-derive returns ""; no source extraction; the full reqMap
#           (parent, cityName, nested city) shallow-merges into the existing
#           flat stub, corrupting it. The response is returned unwrapped and
#           EncodeResponse fails: "message type UpdateCityResponse has no
#           known field named languages".
#
# Post-fix: RequestEntityField unwraps UpdateCityResponse to its single
#           message field (City), matches request.city, infers both
#           source="city" and wrap="city". Merge is clean, response is
#           wrapped, encode succeeds.
set -euo pipefail

SCENARIO_ID="03-wrapped-response-autoderive-ready-case"
scenario_start

EXPECTED="UpdateCity with response-wrapped proto and no explicit source/wrap on the ready case — auto-derive must infer both, extract only the city sub-object, and return a wrapped response."

# Seed the stub so the merge has base data (mirrors the user's gateway stub).
stub_file="$STUBS_DIR/cities_wrapped/marrakech.json"
mkdir -p "$(dirname "$stub_file")"
cat > "$stub_file" <<'EOF'
{
  "name": "marrakech",
  "country": "MA",
  "population": 928850,
  "languages": ["ar", "fr", "ber"],
  "status": "ready"
}
EOF

# Call 1 at t=0 — lands on `updated` fallback (explicit source+wrap, works).
# This also arms the transition timer for the route.
call_grpc "geo.CityService/UpdateCity" \
  '{"parent":{"continent":"africa","country":"MA"},"cityName":"marrakech","city":{"name":"marrakech","population":930000}}'
if ! assert_grpc_ok "update call 1 (fallback path)"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Initial UpdateCity failed — unrelated pre-condition broke."
  return 0 2>/dev/null || exit 0
fi

# Wait for the transition to mature so a second real request resolves to the
# `ready` case (which has no explicit source/wrap).
sleep 2.3

# Call 2 at t≈2.3s — lands on `ready` case. THIS IS THE BUG TRIGGER.
call_grpc "geo.CityService/UpdateCity" \
  '{"parent":{"continent":"africa","country":"MA"},"cityName":"marrakech","city":{"name":"marrakech","population":950000}}'

# Pre-fix, this call fails with "encode persist response: unmarshal response
# json: message type ... no known field named languages" (or similar).
if ! assert_grpc_ok "update call 2 (ready case, auto-derive)"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "$ASSERT_LAST_ERR" \
    "BUG REPRODUCED: ready-case auto-derive returned empty for response-wrapped proto shape; applyGRPCPersist merged full reqMap and returned unwrapped data, EncodeResponse then rejected the stub against UpdateCityResponse. Fix: RequestEntityField must unwrap a single-message-field response type to find the entity type before scanning request fields."
  return 0 2>/dev/null || exit 0
fi

# Pre-fix, even if the call somehow succeeded, the stub would have leaked
# fields.
if jq -e 'has("parent") or has("cityName") or has("city")' "$stub_file" >/dev/null; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub after ready-case update: $(cat "$stub_file" | tr -d '\n')" \
    "BUG REPRODUCED: request envelope leaked into stub — auto-derive produced no source, so applyGRPCPersist merged the whole reqMap."
  return 0 2>/dev/null || exit 0
fi

# Population should reflect the second update, languages preserved.
if ! jq -e '(.population | tonumber) == 950000 and (.languages | length) == 3' "$stub_file" >/dev/null; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub after ready-case update: $(cat "$stub_file" | tr -d '\n')" \
    "Ready-case merge did not apply the city sub-object correctly."
  return 0 2>/dev/null || exit 0
fi

# Response must be wrapped so EncodeResponse can round-trip it.
if ! printf '%s' "$GRPC_STDOUT" | jq -e '(.city.population | tonumber) == 950000' >/dev/null; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "grpc response not wrapped under city: $GRPC_STDOUT" \
    "Auto-derive returned source without wrap — applyGRPCPersist did not use auto-derived wrap when c.Wrap was empty."
  return 0 2>/dev/null || exit 0
fi

# Sanity: subsequent GetCity must succeed (proves the file encodes against
# the response type).
call_grpc "geo.CityService/GetCity" '{"name":"marrakech"}'
if ! assert_grpc_ok "get marrakech"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "$ASSERT_LAST_ERR" \
    "GetCity failed after ready-case update — stub file is no longer a valid UpdateCityResponse."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "auto-derive on response-wrapped shape clean" ""
