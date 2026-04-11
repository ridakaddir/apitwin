#!/usr/bin/env bash
# 09-get-during-transition
# Fire 20 parallel Gets immediately after create, and another 20 during the
# transition boundary (1.5s..2.5s). None should fail or return invalid data.
set -euo pipefail

SCENARIO_ID="09-get-during-transition"
scenario_start

EXPECTED="20 parallel Gets immediately after create + 20 during transition boundary all succeed with valid JSON, no gRPC errors."

call_grpc "geography.GeographyService/CreateCountry" \
  '{"code":"FR","name":"France","continent":"Europe"}'
assert_grpc_ok "create FR" || {
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" "$ASSERT_LAST_ERR" "create failed"
  return 0 2>/dev/null || exit 0
}

parallel_gets() {
  local count="$1" label="$2"
  local tmp; tmp="$(mktemp -d)"
  local i
  for i in $(seq 1 "$count"); do
    (
      "$GRPCURL" -plaintext \
        -import-path "$CONFIG_DIR" \
        -proto "$(basename "$PROTO_FILE")" \
        -d '{"code":"FR"}' "localhost:$GRPC_PORT" \
        "geography.GeographyService/GetCountry" \
        >"$tmp/ok.$i" 2>"$tmp/err.$i"
      echo $? >"$tmp/rc.$i"
    ) &
  done
  wait
  local fails=0 invalid=0 bad_fields=""
  for i in $(seq 1 "$count"); do
    local rc; rc=$(cat "$tmp/rc.$i" 2>/dev/null || echo 99)
    if [[ "$rc" != "0" ]]; then
      fails=$((fails+1))
      bad_fields="$bad_fields [rc=$rc]"
    else
      if ! jq empty "$tmp/ok.$i" >/dev/null 2>&1; then
        invalid=$((invalid+1))
      fi
    fi
  done
  rm -rf "$tmp"
  PAR_FAILS=$fails
  PAR_INVALID=$invalid
  PAR_LABEL="$label"
}

parallel_gets 20 "immediate"
if [[ $PAR_FAILS -ne 0 || $PAR_INVALID -ne 0 ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "immediate batch: fails=$PAR_FAILS invalid=$PAR_INVALID" \
    "Read-during-append race — gRPC handler does not guard stub reads against file rewrites from append."
  return 0 2>/dev/null || exit 0
fi

# Wait into the transition boundary.
sleep 1.6

parallel_gets 20 "boundary"
if [[ $PAR_FAILS -ne 0 || $PAR_INVALID -ne 0 ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "boundary batch: fails=$PAR_FAILS invalid=$PAR_INVALID" \
    "Read-during-update race at transition boundary — persist.Update likely not atomic in gRPC path."
  return 0 2>/dev/null || exit 0
fi

SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "40 parallel reads across two windows all clean" ""
