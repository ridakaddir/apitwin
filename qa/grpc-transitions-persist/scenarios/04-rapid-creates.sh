#!/usr/bin/env bash
# 04-rapid-creates
# Create FR, DE, ES, IT in ~200ms window. All should start provisioning and
# transition to ready within ~3s.
set -euo pipefail

SCENARIO_ID="04-rapid-creates"
scenario_start

EXPECTED="Four rapidly-created countries all start as provisioning and all reach ready within 3s."

countries=(FR DE ES IT)

for c in "${countries[@]}"; do
  call_grpc "geography.GeographyService/CreateCountry" \
    "{\"code\":\"$c\",\"name\":\"$c-name\",\"continent\":\"Europe\"}"
  assert_grpc_ok "create $c" || {
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "create $c failed during rapid batch"
    return 0 2>/dev/null || exit 0
  }
done

# Immediately check all four are provisioning.
for c in "${countries[@]}"; do
  call_grpc "geography.GeographyService/GetCountry" "{\"code\":\"$c\"}"
  st="$(get_field "$GRPC_STDOUT" status)"
  if [[ "$st" != "provisioning" ]]; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    record_result "$SCENARIO_ID" fail "$EXPECTED" \
      "$c status immediately after create = '$st'" \
      "Shared transition state keyed on route.Match caused subsequent creates to resolve to a later transition phase — each resource should get its own window. See transitions.go:47."
    return 0 2>/dev/null || exit 0
  fi
done

# Wait 2.5s — all should be ready.
sleep 2.5

for c in "${countries[@]}"; do
  call_grpc "geography.GeographyService/GetCountry" "{\"code\":\"$c\"}"
  st="$(get_field "$GRPC_STDOUT" status)"
  if [[ "$st" != "ready" ]]; then
    SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
    record_result "$SCENARIO_ID" fail "$EXPECTED" \
      "$c status after 2.5s = '$st'" \
      "Background scheduler didn't finish for $c — Schedule call in handler.go:157 might only be arming once per route."
    return 0 2>/dev/null || exit 0
  fi
done

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "all four creates cycled provisioning→ready" ""
