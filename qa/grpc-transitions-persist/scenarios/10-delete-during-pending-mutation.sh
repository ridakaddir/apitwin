#!/usr/bin/env bash
# 10-delete-during-pending-mutation
# Create, sleep 1s (pending update scheduled for t=2s), delete. After 2s,
# the pending goroutine fires on a missing file and should log a warning
# but not fail noisily. Get must return NotFound.
set -euo pipefail

SCENARIO_ID="10-delete-during-pending-mutation"
scenario_start

EXPECTED="Delete during pending mutation leaves file deleted; Get returns NotFound; scheduler warns benignly about missing file."

call_grpc "geography.GeographyService/CreateCountry" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "create failed"
  return 0 2>/dev/null || exit 0
}

sleep 1

call_grpc "geography.GeographyService/DeleteCountry" '{"code":"FR"}'
assert_grpc_ok "delete" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "delete failed"
  return 0 2>/dev/null || exit 0
}

# Wait for the pending goroutine to fire.
sleep 2

stub_file="$STUBS_DIR/countries/FR.json"
if [[ -f "$stub_file" ]]; then
  content="$(cat "$stub_file")"
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "stub file was recreated by deferred goroutine: $content" \
    "persist.Update in scheduler.go:111 likely creates missing files — should return NotFound so the goroutine can log and exit per scheduler.go:113."
  return 0 2>/dev/null || exit 0
fi

call_grpc "geography.GeographyService/GetCountry" '{"code":"FR"}'
if [[ $GRPC_RC -eq 0 ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "Get succeeded after delete: $GRPC_STDOUT" \
    "File was resurrected by the deferred update — see scheduler.go:111."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "file stayed deleted; Get is NotFound" ""
