#!/usr/bin/env bash
# 14-client-update-missing-resource
# UpdateCountry on a code that was never created must return NotFound (gRPC 5).
# Must NOT silently create the file.
set -euo pipefail

SCENARIO_ID="14-client-update-missing-resource"
scenario_start

EXPECTED="UpdateCountry on missing code ZZ returns gRPC NotFound (5); no stub file is created on disk."

# Ensure ZZ does not exist. Per-scenario stubs wipe guarantees this; be defensive.
call_grpc "geography.GeographyService/DeleteCountry" '{"code":"ZZ"}' || true
# Swallow the expected NotFound from the pre-delete.

call_grpc "geography.GeographyService/UpdateCountry" \
  '{"code":"ZZ","name":"Zed"}'

if [[ $GRPC_RC -eq 0 ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "UpdateCountry succeeded on missing resource: $GRPC_STDOUT" \
    "persist.Update (persist.go:75) should return NotFoundError when the target file is missing — the gRPC handler (internal/grpc/persist.go:66) should map that to codes.NotFound (persist.go:71 in gRPC). Either the route is creating the file (merge=update should not create) or the handler is swallowing the NotFound."
  return 0 2>/dev/null || exit 0
fi

if ! assert_grpc_code "NotFound" "update missing ZZ"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "UpdateCountry on missing resource returned wrong gRPC code — persist.IsNotFound check in grpc handler (internal/grpc/persist.go) may not be mapping to codes.NotFound."
  return 0 2>/dev/null || exit 0
fi

# Make sure no stub file was created as a side effect.
stub_file="$STUBS_DIR/countries/ZZ.json"
if [[ -f "$stub_file" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub file $stub_file was created on update-missing: $(cat "$stub_file")" \
    "persist.Update or its gRPC wrapper created the file despite it not existing — merge=update on a missing path should NOT fall back to append. See internal/grpc/persist.go:66."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" \
  "UpdateCountry on missing code returned NotFound; no stub file created" ""
