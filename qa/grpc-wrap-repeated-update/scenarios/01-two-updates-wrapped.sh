#!/usr/bin/env bash
# 01-two-updates-wrapped
# Seed with CreateCountryWrapped, then issue TWO successive UpdateCountryWrapped
# calls. Both must succeed and the final on-disk stub must reflect the second
# update's values.
set -euo pipefail

SCENARIO_ID="01-two-updates-wrapped"
scenario_start

EXPECTED="Both wrapped updates return OK with new name; stub reflects 2nd update."

# Create FR (wrapped) — response must come back wrapped.
call_grpc "geography.GeographyService/CreateCountryWrapped" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR (wrapped)" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "CreateCountryWrapped failed — baseline setup broken."
  return 0 2>/dev/null || exit 0
}

stub_file="$STUBS_DIR/countries_wrap/FR.json"
if [[ ! -f "$stub_file" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub $stub_file missing after create" \
    "wrap+append path failed to create stub."
  return 0 2>/dev/null || exit 0
fi
BEFORE_UPDATE_1="$(cat "$stub_file")"

# Update 1: name → "French Republic"
call_grpc "geography.GeographyService/UpdateCountryWrapped" \
  '{"country":{"code":"FR","name":"French Republic"}}'
RC1=$GRPC_RC
STDERR1="$GRPC_STDERR"
STDOUT1="$GRPC_STDOUT"
if [[ $RC1 -ne 0 ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "update#1 failed rc=$RC1 stderr=$(printf '%s' "$STDERR1" | tr '\n' ' ')" \
    "First wrapped update failed — see log."
  return 0 2>/dev/null || exit 0
fi
AFTER_UPDATE_1="$(cat "$stub_file" 2>/dev/null || echo MISSING)"

# Response must be wrapped as {country:{...}} with updated name.
if ! assert_field_eq "$STDOUT1" "country.name" "French Republic" "update#1 response"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Response from first update missing/garbled country.name — stdout=$STDOUT1"
  return 0 2>/dev/null || exit 0
fi

# Update 2: name → "République française"
call_grpc "geography.GeographyService/UpdateCountryWrapped" \
  '{"country":{"code":"FR","name":"République française"}}'
RC2=$GRPC_RC
STDERR2="$GRPC_STDERR"
STDOUT2="$GRPC_STDOUT"
if [[ $RC2 -ne 0 ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "update#2 failed rc=$RC2 stderr=$(printf '%s' "$STDERR2" | tr '\n' ' ')" \
    "BUG REPRODUCED: second successive wrapped update fails. before-update-1=$BEFORE_UPDATE_1 after-update-1=$AFTER_UPDATE_1"
  return 0 2>/dev/null || exit 0
fi

if ! assert_field_eq "$STDOUT2" "country.name" "République française" "update#2 response"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Response from second update missing country.name — stdout=$STDOUT2"
  return 0 2>/dev/null || exit 0
fi

# Stub on disk must match update#2.
if ! jq -e '.name == "République française"' "$stub_file" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub on disk not updated: $(cat "$stub_file")" \
    "Last write did not persist to disk."
  return 0 2>/dev/null || exit 0
fi
if jq -e 'has("country")' "$stub_file" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub on disk has wrapper key 'country': $(cat "$stub_file")" \
    "After updates, stub should remain unwrapped on disk; wrap leaked into persisted file."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "both updates OK; stub name='République française'" ""
