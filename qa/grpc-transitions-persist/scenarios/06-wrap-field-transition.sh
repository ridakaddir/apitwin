#!/usr/bin/env bash
# 06-wrap-field-transition
# wrap="country" on create and transition cases. Persisted file must NOT
# contain the wrapper key; clients get a wrapped envelope response.
set -euo pipefail

SCENARIO_ID="06-wrap-field-transition"
scenario_start

EXPECTED="Wrapped create persists unwrapped data; transitions apply cleanly; response is wrapped."

call_grpc "geography.GeographyService/CreateCountryWrapped" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR (wrapped)" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "CreateCountryWrapped failed"
  return 0 2>/dev/null || exit 0
}

# Response must be wrapped.
if ! get_field "$GRPC_STDOUT" "country.code" | grep -q '^FR$'; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "create response not wrapped: $GRPC_STDOUT" \
    "wrap=country on the append case should return {country:{...}}"
  return 0 2>/dev/null || exit 0
fi

stub_file="$STUBS_DIR/countries_wrap/FR.json"
if [[ ! -f "$stub_file" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub $stub_file missing" \
    "wrap + append path broke file creation"
  return 0 2>/dev/null || exit 0
fi

# Stub must not contain a "country" wrapper key.
if jq -e 'has("country")' "$stub_file" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub file contains wrap key: $(cat "$stub_file")" \
    "Persisted data should not include the wrapper key — applyGRPCPersist should unwrap."
  return 0 2>/dev/null || exit 0
fi

if ! jq -e '.status == "provisioning"' "$stub_file" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "initial status not provisioning: $(cat "$stub_file")" \
    "defaults not applied with wrap"
  return 0 2>/dev/null || exit 0
fi

sleep 2.5

if ! jq -e '.status == "ready"' "$stub_file" >/dev/null 2>&1; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "after 2.5s stub not ready: $(cat "$stub_file")" \
    "deferred transition on wrap case did not apply defaults"
  return 0 2>/dev/null || exit 0
fi

# Get should return wrapped and ready.
call_grpc "geography.GeographyService/GetCountryWrapped" '{"code":"FR"}'
assert_grpc_ok "get wrapped" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "GetCountryWrapped failed"
  return 0 2>/dev/null || exit 0
}
if ! assert_field_eq "$GRPC_STDOUT" "country.status" "ready" "wrapped read"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Get returned unwrapped or stale data"
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "wrap+transition round-tripped cleanly" ""
