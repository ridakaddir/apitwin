#!/usr/bin/env bash
# 01-happy-path: Create → immediate status=provisioning → wait → ready.
set -euo pipefail

SCENARIO_ID="01-happy-path"
scenario_start

EXPECTED="CreateCountry FR then GetCountry returns status=provisioning immediately and status=ready within 4s"

call_grpc "geography.GeographyService/CreateCountry" \
  '{"code":"FR","name":"France","continent":"Europe"}'
if ! assert_grpc_ok "create FR"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "CreateCountry failed outright — gRPC persist append broken for baseline route."
  return 0 2>/dev/null || exit 0
fi

call_grpc "geography.GeographyService/GetCountry" '{"code":"FR"}'
if ! assert_grpc_ok "get FR immediate"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "GetCountry immediately after create failed — append did not persist file."
  return 0 2>/dev/null || exit 0
fi

if ! assert_field_eq "$GRPC_STDOUT" "status" "provisioning" "initial status"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" \
    "Initial status was not provisioning — defaults file not applied on append (grpc/persist.go loadGRPCDefaults)."
  return 0 2>/dev/null || exit 0
fi

if ! wait_for_field_eq "geography.GeographyService/GetCountry" '{"code":"FR"}' status ready 4; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "status still=$WAIT_LAST_VALUE after 4s" \
    "Background scheduler never fired — scheduler.Schedule in handler.go:157 or schedule goroutine in scheduler.go:96 not running."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "FR transitioned provisioning→ready within 4s" ""
