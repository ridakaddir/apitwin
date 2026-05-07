#!/usr/bin/env bash
# grpc.sh — thin wrappers around grpcurl for the QA harness.
set -euo pipefail

# call_grpc METHOD JSON_DATA [--capture-err]
# Sets globals GRPC_STDOUT, GRPC_STDERR, GRPC_RC.
call_grpc() {
  local method="$1"
  local data="${2-}"
  if [[ -z "$data" ]]; then
    data='{}'
  fi
  local stdout_file stderr_file
  stdout_file="$(mktemp)"
  stderr_file="$(mktemp)"
  set +e
  "$GRPCURL" -plaintext \
    -import-path "$CONFIG_DIR" \
    -proto "$(basename "$PROTO_FILE")" \
    -d "$data" \
    "localhost:$GRPC_PORT" \
    "$method" \
    >"$stdout_file" 2>"$stderr_file"
  GRPC_RC=$?
  set -e
  GRPC_STDOUT="$(cat "$stdout_file")"
  GRPC_STDERR="$(cat "$stderr_file")"
  rm -f "$stdout_file" "$stderr_file"
  return 0
}

# get_field JSON DOTPATH — extract a field using jq, printing empty on missing.
get_field() {
  local json="$1" path="$2"
  printf '%s' "$json" | jq -r ".${path} // empty" 2>/dev/null || true
}
