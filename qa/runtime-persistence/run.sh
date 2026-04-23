#!/usr/bin/env bash
# run.sh — QA harness orchestrator for runtime-state persistence (REST).
#
# Each scenario typically starts the server, does work, stops it, and restarts
# it WITHOUT wiping .apitwin/state/ so the persistence behaviour can be
# asserted. The `reset_stubs` helper (only called once per scenario, before the
# first start) wipes both the runtime state dir AND the seed `stubs/` dir and
# re-seeds from `seed-template/`.
set -euo pipefail

HARNESS_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$HARNESS_DIR/../.." && pwd)"

BINARY="/tmp/apitwin-qa"
ITER=""
SCENARIO_FILTER=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BINARY="$2"; shift 2 ;;
    --iteration) ITER="$2"; shift 2 ;;
    --scenario) SCENARIO_FILTER="$2"; shift 2 ;;
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

if ! command -v jq >/dev/null 2>&1; then
  echo "error: jq not found on PATH" >&2
  exit 2
fi

CONFIG_DIR="$HARNESS_DIR"
SEED_TEMPLATE_DIR="$HARNESS_DIR/seed-template"
SEED_STUBS_DIR="$HARNESS_DIR/stubs"
RUNTIME_DIR="$HARNESS_DIR/.apitwin/state"
META_DIR="$RUNTIME_DIR/.apitwin-meta"
REPORT_DIR="$HARNESS_DIR/report"
LOG_FILE="$REPORT_DIR/iteration-${ITER}.log"
RESULTS_FILE="$REPORT_DIR/iteration-${ITER}.jsonl"
REPORT_FILE="$REPORT_DIR/iteration-${ITER}.json"

mkdir -p "$REPORT_DIR"
: > "$LOG_FILE"
: > "$RESULTS_FILE"

# shellcheck disable=SC1091
source "$HARNESS_DIR/lib/http.sh"
# shellcheck disable=SC1091
source "$HARNESS_DIR/lib/wait.sh"
# shellcheck disable=SC1091
source "$HARNESS_DIR/lib/assert.sh"

export CONFIG_DIR SEED_TEMPLATE_DIR SEED_STUBS_DIR RUNTIME_DIR META_DIR RESULTS_FILE LOG_FILE

pick_port() {
  python3 -c 'import socket; s=socket.socket(); s.bind(("",0)); print(s.getsockname()[1]); s.close()'
}

# reset_stubs — wipe BOTH the seed stubs dir and the runtime state dir, then
# re-seed from seed-template/. Intended to be called exactly once per scenario,
# at the top, so each scenario starts from a known clean slate. Never call
# this between restart steps within a scenario — that's what restart_server
# is for.
reset_stubs() {
  rm -rf "$RUNTIME_DIR"
  rm -rf "$SEED_STUBS_DIR"
  mkdir -p "$SEED_STUBS_DIR"
  # Seed from template if present.
  if [[ -d "$SEED_TEMPLATE_DIR/stubs" ]]; then
    cp -R "$SEED_TEMPLATE_DIR/stubs/." "$SEED_STUBS_DIR/"
  fi
}

SERVER_PID=""
HTTP_PORT=""

# _start_server_process EXTRA_ARGS...
# Internal helper shared by start_server and restart_server. Does NOT touch
# .apitwin/state/.
_start_server_process() {
  HTTP_PORT=$(pick_port)
  export HTTP_PORT

  {
    echo "----"
    echo "$(date -u +%FT%TZ) starting server http_port=$HTTP_PORT extra=$*"
  } >> "$LOG_FILE"

  "$BINARY" --config "$CONFIG_DIR" \
    --port "$HTTP_PORT" \
    "$@" \
    >>"$LOG_FILE" 2>&1 &
  SERVER_PID=$!

  # Poll readiness for up to 8 seconds via /__api/routes (guaranteed to 200
  # when the HTTP mux is up, and a clean local endpoint that doesn't itself
  # fire any transitions).
  local i
  for i in $(seq 1 80); do
    if curl -sS -o /dev/null --max-time 0.5 "http://127.0.0.1:${HTTP_PORT}/__api/routes" 2>/dev/null; then
      return 0
    fi
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
      echo "server exited prematurely — see $LOG_FILE" >&2
      return 1
    fi
    sleep 0.1
  done
  echo "server did not become ready within 8s" >&2
  return 1
}

# start_server — initial scenario start (reset_stubs should have run first).
start_server() { _start_server_process "$@"; }

# restart_server — stop + start, preserving .apitwin/state/.
restart_server() {
  stop_server
  _start_server_process "$@"
}

# stop_server — clean SIGTERM + wait. Does NOT touch .apitwin/state/.
stop_server() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    # Wait up to 5s for graceful shutdown.
    local i
    for i in $(seq 1 50); do
      if ! kill -0 "$SERVER_PID" 2>/dev/null; then
        break
      fi
      sleep 0.1
    done
    # Force-kill if still alive.
    if kill -0 "$SERVER_PID" 2>/dev/null; then
      kill -9 "$SERVER_PID" 2>/dev/null || true
    fi
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  SERVER_PID=""
}

export -f _start_server_process start_server restart_server stop_server pick_port reset_stubs

cleanup() { stop_server 2>/dev/null || true; }
trap cleanup EXIT INT TERM

STARTED_AT="$(date -u +%FT%TZ)"

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

  # Run scenario in a subshell so `return 0 2>/dev/null || exit 0` patterns
  # don't kill the orchestrator.
  (
    # shellcheck disable=SC1090
    source "$sf"
  ) || echo "scenario $sname raised non-zero (should not happen — scenarios use record_result)" >> "$LOG_FILE"

  # Defensive: make sure whatever server the scenario started is stopped
  # before the next one begins.
  stop_server
done

# Assemble final report.
PASSED=$(jq -s 'map(select(.status=="pass")) | length' "$RESULTS_FILE")
FAILED=$(jq -s 'map(select(.status=="fail")) | length' "$RESULTS_FILE")
TOTAL=$(jq -s 'length' "$RESULTS_FILE")
SCENARIOS_JSON="$(jq -s '.' "$RESULTS_FILE")"

jq -n \
  --argjson iteration "$ITER" \
  --arg binary "$BINARY" \
  --arg started_at "$STARTED_AT" \
  --argjson total "$TOTAL" \
  --argjson passed "$PASSED" \
  --argjson failed "$FAILED" \
  --argjson scenarios "$SCENARIOS_JSON" \
  '{iteration:$iteration,binary:$binary,started_at:$started_at,total:$total,passed:$passed,failed:$failed,scenarios:$scenarios}' \
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
