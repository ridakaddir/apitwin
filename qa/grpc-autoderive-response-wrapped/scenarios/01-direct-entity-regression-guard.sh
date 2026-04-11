#!/usr/bin/env bash
# 01-direct-entity-regression-guard
#
# Shape A regression guard: PatchCity returns the entity directly (response IS
# City). Auto-derive of source="city" from the proto has always worked for
# this shape (via RequestEntityField matching request field type == response
# output type). This scenario must keep passing both pre-fix and post-fix to
# prove the fix does not regress the working Google-style direct-entity path.
set -euo pipefail

SCENARIO_ID="01-direct-entity-regression-guard"
scenario_start

EXPECTED="PatchCity with auto-derived source extracts City from the request, merges cleanly, transitions provisioning→ready without leaking parent/id/cityName into the stub."

# Seed the target file so update has something to merge into.
stub_file="$STUBS_DIR/cities_patch/marrakech.json"
mkdir -p "$(dirname "$stub_file")"
cat > "$stub_file" <<'EOF'
{
  "name": "marrakech",
  "country": "MA",
  "population": 928850,
  "languages": ["ar", "fr"],
  "status": "ready"
}
EOF

call_grpc "geo.CityService/PatchCity" \
  '{"parent":{"continent":"africa","country":"MA"},"id":"marrakech","city":{"name":"marrakech","population":1000000}}'

if ! assert_grpc_ok "patch marrakech"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "PatchCity call itself failed — auto-derive for direct-entity response broke."
  return 0 2>/dev/null || exit 0
fi

# Stub must have population updated and languages + country preserved, and
# must NOT contain parent/id leaked from the request envelope.
if ! jq -e '(.population | tonumber) == 1000000 and .country == "MA" and (.languages | length) == 2 and (has("parent") | not) and (has("id") | not)' "$stub_file" >/dev/null; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub after patch: $(cat "$stub_file" | tr -d '\n')" \
    "Auto-derive did not extract city sub-object on Shape A (direct-entity response) — regression."
  return 0 2>/dev/null || exit 0
fi

# After a second call post-transition, the ready case must merge cleanly too.
sleep 2.3
call_grpc "geo.CityService/PatchCity" \
  '{"parent":{"continent":"africa","country":"MA"},"id":"marrakech","city":{"name":"marrakech","population":1100000}}'

if ! assert_grpc_ok "patch marrakech (post-transition)"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Second PatchCity call after transition failed."
  return 0 2>/dev/null || exit 0
fi

if ! jq -e '(.population | tonumber) == 1100000 and (has("parent") | not) and (has("id") | not)' "$stub_file" >/dev/null; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub after second patch: $(cat "$stub_file" | tr -d '\n')" \
    "Ready-case merge on direct-entity response leaked request envelope."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "direct-entity auto-derive clean" ""
