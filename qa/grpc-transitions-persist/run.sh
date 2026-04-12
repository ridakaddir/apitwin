#!/usr/bin/env bash
# run.sh — QA harness orchestrator for gRPC transitions + persistence.
set -euo pipefail

HARNESS_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$HARNESS_DIR/../.." && pwd)"

BINARY="/tmp/apitwin-qa/apitwin"
ITER=""
SCENARIO_FILTER=""
KEEP_STUBS=0
SHARED_SERVER=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BINARY="$2"; shift 2 ;;
    --iteration) ITER="$2"; shift 2 ;;
    --scenario) SCENARIO_FILTER="$2"; shift 2 ;;
    --keep-stubs) KEEP_STUBS=1; shift ;;
    --shared-server) SHARED_SERVER=1; shift ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$ITER" ]]; then
  echo "error: --iteration N is required" >&2
  exit 2
fi

if [[ ! -x "$BINARY" ]]; then
  echo "error: binary not found at $BINARY" >&2
  exit 2
fi

if ! command -v grpcurl >/dev/null 2>&1; then
  GRPCURL="/opt/homebrew/bin/grpcurl"
else
  GRPCURL="$(command -v grpcurl)"
fi
if [[ ! -x "$GRPCURL" ]]; then
  echo "error: grpcurl not found" >&2
  exit 2
fi

CONFIG_DIR="$HARNESS_DIR"
PROTO_FILE="$HARNESS_DIR/geography.proto"
STUBS_DIR="$HARNESS_DIR/stubs"
REPORT_DIR="$HARNESS_DIR/report"
LOG_FILE="$REPORT_DIR/iteration-${ITER}.log"
RESULTS_FILE="$REPORT_DIR/iteration-${ITER}.jsonl"
REPORT_FILE="$REPORT_DIR/iteration-${ITER}.json"

mkdir -p "$REPORT_DIR"
: > "$LOG_FILE"
: > "$RESULTS_FILE"

# shellcheck disable=SC1091
source "$HARNESS_DIR/lib/grpc.sh"
# shellcheck disable=SC1091
source "$HARNESS_DIR/lib/wait.sh"
# shellcheck disable=SC1091
source "$HARNESS_DIR/lib/assert.sh"

export GRPCURL PROTO_FILE CONFIG_DIR STUBS_DIR RESULTS_FILE LOG_FILE

pick_port() {
  python3 -c 'import socket; s=socket.socket(); s.bind(("",0)); print(s.getsockname()[1]); s.close()'
}

reset_stubs() {
  if [[ $KEEP_STUBS -eq 0 ]]; then
    rm -rf "$STUBS_DIR"
    mkdir -p "$STUBS_DIR"
  fi
}

SERVER_PID=""
start_server() {
  GRPC_PORT=$(pick_port)
  HTTP_PORT=$(pick_port)
  export GRPC_PORT HTTP_PORT

  {
    echo "----"
    echo "$(date -u +%FT%TZ) starting server grpc_port=$GRPC_PORT http_port=$HTTP_PORT"
  } >> "$LOG_FILE"

  # --no-runtime-dir: the QA scenarios pre-seed stub files directly in
  # $STUBS_DIR after start_server and then assert against the same paths
  # post-mutation. The default runtime-mirror behavior would hide those
  # writes from the running server (it reads/writes via .apitwin/state/).
  "$BINARY" --config "$CONFIG_DIR" \
    --grpc-proto "$PROTO_FILE" \
    --grpc-port "$GRPC_PORT" \
    --port "$HTTP_PORT" \
    --no-runtime-dir \
    >>"$LOG_FILE" 2>&1 &
  SERVER_PID=$!

  # Poll readiness for up to 5 seconds via a TCP connect to the gRPC port.
  local i
  for i in $(seq 1 50); do
    if python3 -c "import socket,sys;s=socket.socket();s.settimeout(0.2)
try:
    s.connect(('127.0.0.1',$GRPC_PORT));s.close()
except Exception:
    sys.exit(1)" >/dev/null 2>&1; then
      return 0
    fi
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
      echo "server exited prematurely — see $LOG_FILE" >&2
      return 1
    fi
    sleep 0.2
  done
  echo "server did not become ready within 5s" >&2
  return 1
}

stop_server() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  SERVER_PID=""
}

cleanup() { stop_server; }
trap cleanup EXIT INT TERM

STARTED_AT="$(date -u +%FT%TZ)"

# Pre-flight: wipe stubs and start server once to validate config.
reset_stubs
if ! start_server; then
  jq -nc \
    --arg id "00-startup" \
    --arg status "fail" \
    --arg expected "binary starts with QA config" \
    --arg actual "server did not become ready — see log" \
    --arg hypothesis "apitwin crashed or failed to load config — check $LOG_FILE" \
    --argjson duration_ms 0 \
    '{id:$id,status:$status,duration_ms:$duration_ms,expected:$expected,actual:$actual,hypothesis:$hypothesis}' \
    >> "$RESULTS_FILE"
  SCENARIOS_JSON="$(jq -s '.' "$RESULTS_FILE")"
  jq -n \
    --argjson iteration "$ITER" \
    --arg binary "$BINARY" \
    --argjson grpc_port 0 \
    --arg started_at "$STARTED_AT" \
    --argjson total 1 \
    --argjson passed 0 \
    --argjson failed 1 \
    --argjson scenarios "$SCENARIOS_JSON" \
    '{iteration:$iteration,binary:$binary,grpc_port:$grpc_port,started_at:$started_at,total:$total,passed:$passed,failed:$failed,scenarios:$scenarios}' \
    > "$REPORT_FILE"
  echo "STARTUP FAILED — see $LOG_FILE" >&2
  exit 1
fi

# If --shared-server, keep this one. Otherwise stop it; scenarios will restart.
if [[ $SHARED_SERVER -eq 0 ]]; then
  stop_server
fi

shopt -s nullglob
SCENARIO_FILES=( "$HARNESS_DIR/scenarios/"*.sh )
shopt -u nullglob
IFS=$'\n' SCENARIO_FILES=( $(printf '%s\n' "${SCENARIO_FILES[@]}" | sort) )
unset IFS

for sf in "${SCENARIO_FILES[@]}"; do
  sname="$(basename "$sf" .sh)"
  if [[ -n "$SCENARIO_FILTER" && "$sname" != "$SCENARIO_FILTER" ]]; then
    continue
  fi

  echo "==== running $sname ====" >> "$LOG_FILE"

  if [[ $SHARED_SERVER -eq 0 ]]; then
    reset_stubs
    if ! start_server; then
      jq -nc \
        --arg id "$sname" \
        --arg status "fail" \
        --arg expected "server starts for scenario" \
        --arg actual "server did not become ready" \
        --arg hypothesis "apitwin crashed on restart between scenarios" \
        --argjson duration_ms 0 \
        '{id:$id,status:$status,duration_ms:$duration_ms,expected:$expected,actual:$actual,hypothesis:$hypothesis}' \
        >> "$RESULTS_FILE"
      continue
    fi
  fi

  # Run scenario in a subshell so `return 0 2>/dev/null || exit 0` patterns
  # don't kill the orchestrator.
  (
    # shellcheck disable=SC1090
    source "$sf"
  ) || echo "scenario $sname raised non-zero (should not happen — scenarios use record_result)" >> "$LOG_FILE"

  if [[ $SHARED_SERVER -eq 0 ]]; then
    stop_server
  fi
done

# Assemble final report.
PASSED=$(jq -s 'map(select(.status=="pass")) | length' "$RESULTS_FILE")
FAILED=$(jq -s 'map(select(.status=="fail")) | length' "$RESULTS_FILE")
TOTAL=$(jq -s 'length' "$RESULTS_FILE")
SCENARIOS_JSON="$(jq -s '.' "$RESULTS_FILE")"

jq -n \
  --argjson iteration "$ITER" \
  --arg binary "$BINARY" \
  --argjson grpc_port "${GRPC_PORT:-0}" \
  --arg started_at "$STARTED_AT" \
  --argjson total "$TOTAL" \
  --argjson passed "$PASSED" \
  --argjson failed "$FAILED" \
  --argjson scenarios "$SCENARIOS_JSON" \
  '{iteration:$iteration,binary:$binary,grpc_port:$grpc_port,started_at:$started_at,total:$total,passed:$passed,failed:$failed,scenarios:$scenarios}' \
  > "$REPORT_FILE"

echo
echo "=========================================="
echo "Summary: $PASSED / $TOTAL passed, $FAILED failed"
echo "Report:  $REPORT_FILE"
echo "Log:     $LOG_FILE"
echo "=========================================="

if [[ "$FAILED" -gt 0 ]]; then
  exit 1
fi
exit 0
