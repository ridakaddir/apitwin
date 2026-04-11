#!/usr/bin/env bash
# 02-multi-resource-independent-timers
# Known suspected bug: gRPC transition state is keyed by route.Match, so a second
# resource created after the route's firstHit is past the terminal boundary will
# be resolved as "ready" immediately via resolveCase — even though its timeline
# should start fresh at t=0.
set -euo pipefail

SCENARIO_ID="02-multi-resource-independent-timers"
scenario_start

EXPECTED="Each created resource follows its own transition timeline; DE created at t=2.5s still shows provisioning immediately after create."

call_grpc "geography.GeographyService/CreateCountry" \
  '{"code":"FR","name":"France","continent":"Europe"}'
if ! assert_grpc_ok "create FR"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "CreateCountry FR failed"
  return 0 2>/dev/null || exit 0
fi

# Wait 2.5s so FR's transition window has elapsed in transition state.
sleep 2.5

call_grpc "geography.GeographyService/CreateCountry" \
  '{"code":"DE","name":"Germany","continent":"Europe"}'
if ! assert_grpc_ok "create DE"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "CreateCountry DE failed"
  return 0 2>/dev/null || exit 0
fi

call_grpc "geography.GeographyService/GetCountry" '{"code":"DE"}'
if ! assert_grpc_ok "get DE"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "GetCountry DE failed"
  return 0 2>/dev/null || exit 0
fi

actual_status="$(get_field "$GRPC_STDOUT" status)"
if [[ "$actual_status" != "provisioning" ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "DE status immediately after create was '$actual_status' instead of 'provisioning'" \
    "gRPC transition state is keyed on route.Match (transitions.go:47), so a second create after the terminal threshold resolves directly to 'ready' via resolveCase. Each resource should get its own timeline — the keying needs to include a resource id."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "DE created after FR's window still shows provisioning" ""
