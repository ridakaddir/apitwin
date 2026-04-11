#!/usr/bin/env bash
# 08-transition-name-equals-fallback
# Route where fallback="ready" AND transitions has case="ready".
# Per handler.go:135: `isTransitionCase := caseName != route.Fallback && len(route.Transitions) > 0`
# If caseName == route.Fallback, the 404-fallback retry does NOT fire.
# In this route, the append/fallback case is "ready" (defaults=country-ready).
# Expected happy behaviour: create via "ready" case, persistence appends,
# no NotFound retry needed. Transition timing is edge-case: the first resolveCase
# might return "provisioning" (if < 2s elapsed). That case has merge=update
# against a just-created file — should succeed.
set -euo pipefail

SCENARIO_ID="08-transition-name-equals-fallback"
scenario_start

EXPECTED="CreateCountryFallbackTrap succeeds and persists a country; subsequent Get returns status=ready (since fallback case uses ready defaults)."

call_grpc "geography.GeographyService/CreateCountryFallbackTrap" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR (trap)" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Call returned error — edge case where transition resolved to a case with a path that did not exist and 404-retry was blocked (handler.go:135 isTransitionCase check excludes fallback-named cases)."
  return 0 2>/dev/null || exit 0
}

stub_file="$STUBS_DIR/countries_trap/FR.json"
if [[ ! -f "$stub_file" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub file $stub_file missing after successful create" \
    "Create call reported OK but no file — applyGRPCPersist path broken for fallback-named append case."
  return 0 2>/dev/null || exit 0
fi

# Immediate Get — status should be ready (ready case has country-ready defaults).
call_grpc "geography.GeographyService/GetCountryFallbackTrap" '{"code":"FR"}'
assert_grpc_ok "get FR (trap)" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "get failed"
  return 0 2>/dev/null || exit 0
}

final_status="$(get_field "$GRPC_STDOUT" status)"
if [[ "$final_status" != "ready" && "$final_status" != "provisioning" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "status='$final_status' — expected ready or provisioning" \
    "Unexpected status from route where transition case name == fallback."
  return 0 2>/dev/null || exit 0
fi

# Now stress the 404 fallback trap with a SECOND create of a different country
# after the transition window expires. The transition state for the route has
# first-hit already past 2s, so resolveCase will return "ready" (the terminal).
# ready case is the fallback; its file pattern is the append directory.
# It will be resolved and since it's the fallback case, the isTransitionCase
# guard disables the 404-retry. But in this setup, "ready" is the append case
# (directory) so the call should actually still succeed.
sleep 2.5

call_grpc "geography.GeographyService/CreateCountryFallbackTrap" \
  '{"code":"DE","name":"Germany","continent":"Europe"}'
if [[ $GRPC_RC -ne 0 ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "second create DE failed: $GRPC_STDERR" \
    "With transition.ready == fallback, the 404-retry guard in handler.go:135 prevents retry; once transitions resolve to 'ready' the append still works because 'ready' IS the append case here. Failure means something else is broken."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "fallback-trap route functions" \
  "Note: the 404-retry guard in handler.go:135 still has the identified asymmetry; this scenario doesn't trigger the pathological case because fallback case is also an append."
