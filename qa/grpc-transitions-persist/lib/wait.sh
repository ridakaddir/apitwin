#!/usr/bin/env bash
# wait.sh — polling helpers for transition-window assertions.
set -euo pipefail

# wait_for_field_eq METHOD JSON_REQ DOTPATH EXPECTED TIMEOUT_SEC
# Polls every 200ms until the response field equals EXPECTED, or times out.
# Sets WAIT_LAST_VALUE on return.
wait_for_field_eq() {
  local method="$1" req="$2" path="$3" expected="$4" timeout="$5"
  local deadline
  deadline=$(python3 -c "import time; print(time.time() + float($timeout))")
  WAIT_LAST_VALUE=""
  while :; do
    call_grpc "$method" "$req"
    if [[ $GRPC_RC -eq 0 ]]; then
      WAIT_LAST_VALUE="$(get_field "$GRPC_STDOUT" "$path")"
      if [[ "$WAIT_LAST_VALUE" == "$expected" ]]; then
        return 0
      fi
    fi
    local now
    now=$(python3 -c "import time; print(time.time())")
    if python3 -c "import sys; sys.exit(0 if $now >= $deadline else 1)"; then
      return 1
    fi
    sleep 0.2
  done
}

# sleep_until_relative T0 SECONDS — sleep until SECONDS past T0 (wall seconds).
sleep_until_relative() {
  local t0="$1" target="$2"
  local now remaining
  now=$(python3 -c "import time; print(time.time())")
  remaining=$(python3 -c "print(max(0.0, ($t0) + $target - $now))")
  python3 -c "import time; time.sleep($remaining)"
}

# now_s — print current wall seconds as float.
now_s() {
  python3 -c "import time; print(time.time())"
}
