#!/usr/bin/env bash
# 12-conditions-plus-transitions
# Condition: body.code == "XX" → serve "invalid" (INVALID_ARGUMENT).
# Other codes → transition path.
# Verifies resolveCase priority: conditions > transitions > fallback.
set -euo pipefail

SCENARIO_ID="12-conditions-plus-transitions"
scenario_start

EXPECTED="code=XX returns INVALID_ARGUMENT; code=FR goes through transitions normally."

# 1. code=XX should match the condition and return an error case.
call_grpc "geography.GeographyService/CreateCountryConditioned" \
  '{"code":"XX","name":"bad","continent":"Europe"}'
if [[ $GRPC_RC -eq 0 ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "code=XX returned OK: $GRPC_STDOUT" \
    "Condition matching bypassed — conditions should resolve before transitions per resolveCase (handler.go:249)."
  return 0 2>/dev/null || exit 0
fi
if ! printf '%s' "$GRPC_STDERR" | grep -q -i "InvalidArgument"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "code=XX did not return InvalidArgument: $GRPC_STDERR" \
    "Condition case 'invalid' has status=3 but a different error came back."
  return 0 2>/dev/null || exit 0
fi

# 2. code=FR should go through the transitions path.
call_grpc "geography.GeographyService/CreateCountryConditioned" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "FR create on conditioned route failed — transitions path broken when conditions exist."
  return 0 2>/dev/null || exit 0
}

call_grpc "geography.GeographyService/GetCountryConditioned" '{"code":"FR"}'
assert_grpc_ok "get FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "get after create failed"
  return 0 2>/dev/null || exit 0
}
if ! assert_field_eq "$GRPC_STDOUT" "status" "provisioning" "FR initial"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "FR initial status wrong on conditioned route."
  return 0 2>/dev/null || exit 0
fi

if ! wait_for_field_eq "geography.GeographyService/GetCountryConditioned" '{"code":"FR"}' status ready 4; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "FR never transitioned (last='$WAIT_LAST_VALUE')" \
    "Transitions scheduler did not run when conditions are defined on the same route."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "conditions + transitions coexist correctly" ""
