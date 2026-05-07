#!/usr/bin/env bash
# 01-second-update-after-transition-no-corruption
#
# Headline reproducer for the user's reported bug. Two consecutive client
# UpdateCountry calls bracket the deferred transition window:
#
#   t = 0   first UpdateCountry → fallback case `updated`
#           (explicit wrap+source) → stub stays clean
#   t ≈ 2s  deferred ready transition fires (background mutation)
#   t = 3s+ second UpdateCountry → time-based resolver returns `ready`
#           (sparse case) → with the fix, inheritance + auto-derive keep
#           the stub clean. Pre-fix the request envelope leaked in:
#           orgId/providerId/environment-style scalars + nested wrapper.
#
# Then a final GetCountry must round-trip through the proto encode without
# error, proving the persisted file is still a valid Country.
set -euo pipefail

SCENARIO_ID="01-second-update-after-transition-no-corruption"
scenario_start

EXPECTED="Two UpdateCountry calls (one before, one after the transition window) both succeed, leave the stub flat (no leaked routing scalars or nested wrapper), and a subsequent GetCountry round-trips cleanly."

stub_file="$STUBS_DIR/countries_lro/MA.json"
mkdir -p "$(dirname "$stub_file")"
cat > "$stub_file" <<'EOF'
{
  "code": "MA",
  "name": "Morocco",
  "continent": "Africa",
  "population": 37000000
}
EOF

# Step 1 — first update at t=0; lands on fallback `updated` (explicit wrap+source).
call_grpc "geo.lro.CountryService/UpdateCountry" \
  '{"regionId":"EMEA","charterId":"charter-1","locale":"fr-MA","id":"MA","country":{"population":37100000}}'

if ! assert_grpc_ok "first UpdateCountry"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "$ASSERT_LAST_ERR" \
    "Fallback updated case broke — wrap/source wiring regression."
  return 0 2>/dev/null || exit 0
fi

# Stub must be flat after step 1.
if jq -e 'has("regionId") or has("charterId") or has("locale") or has("country") or has("id")' "$stub_file" >/dev/null; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub corrupted after first update: $(cat "$stub_file" | tr -d '\n')" \
    "Fallback case did not strip routing scalars / nested wrapper."
  return 0 2>/dev/null || exit 0
fi

# Step 2 — wait past the 2s transition window; the deferred ready transition
# fires concurrently and merges defaults onto the file.
sleep 2.5

# Step 3 — second update; resolver returns `ready` (sparse case). With the
# fix, inheritance + auto-derive recover wrap+source.
call_grpc "geo.lro.CountryService/UpdateCountry" \
  '{"regionId":"EMEA","charterId":"charter-1","locale":"fr-MA","id":"MA","country":{"population":37500000}}'

if ! assert_grpc_ok "second UpdateCountry (post-transition)"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "$ASSERT_LAST_ERR" \
    "BUG REPRO: sparse ready transition case persisted full request envelope; encode round-trip on response failed."
  return 0 2>/dev/null || exit 0
fi

# Stub must STILL be flat after the post-transition update.
if jq -e 'has("regionId") or has("charterId") or has("locale") or has("country") or has("id")' "$stub_file" >/dev/null; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub corrupted after post-transition update: $(cat "$stub_file" | tr -d '\n')" \
    "BUG REPRO: sparse ready case bypassed inheritance + auto-derive."
  return 0 2>/dev/null || exit 0
fi

# Population merged.
if ! jq -e '(.population | tonumber) == 37500000' "$stub_file" >/dev/null; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "population not merged: $(cat "$stub_file" | tr -d '\n')" \
    "Source extraction did not capture the inner country.population."
  return 0 2>/dev/null || exit 0
fi

# Step 4 — GET round-trip via proto encoder.
call_grpc "geo.lro.CountryService/GetCountry" \
  '{"regionId":"EMEA","charterId":"charter-1","locale":"fr-MA","id":"MA"}'

if ! assert_grpc_ok "GetCountry round-trip"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "$ASSERT_LAST_ERR" \
    "BUG REPRO: persisted file is no longer a valid Country — encode failure on read."
  return 0 2>/dev/null || exit 0
fi

if ! printf '%s' "$GRPC_STDOUT" | jq -e '(.country.population | tonumber) == 37500000' >/dev/null; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "GetCountry response did not surface population=37500000: $GRPC_STDOUT" \
    "Wrap on GET path lost the entity."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "two updates clean across transition; GET round-trips" ""
