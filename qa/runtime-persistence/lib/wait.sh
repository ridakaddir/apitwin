#!/usr/bin/env bash
# wait.sh — polling helpers for transition-window / boot-sync assertions.
set -euo pipefail

# wait_for_http_field_eq METHOD PATH DOTPATH EXPECTED TIMEOUT_SEC [BODY]
# Polls every 200ms until the response JSON field equals EXPECTED or times out.
# Sets WAIT_LAST_VALUE on return.
wait_for_http_field_eq() {
  local method="$1" path="$2" dp="$3" expected="$4" timeout="$5" body="${6-}"
  local deadline
  deadline=$(python3 -c "import time; print(time.time() + float($timeout))")
  WAIT_LAST_VALUE=""
  while :; do
    call_http "$method" "$path" "$body"
    if [[ "$HTTP_STATUS" == "200" ]]; then
      WAIT_LAST_VALUE="$(get_field "$HTTP_BODY" "$dp")"
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

# wait_for_file_field_eq FILE DOTPATH EXPECTED TIMEOUT_SEC
# Polls the JSON file on disk until the field matches or times out.
wait_for_file_field_eq() {
  local file="$1" dp="$2" expected="$3" timeout="$4"
  local deadline
  deadline=$(python3 -c "import time; print(time.time() + float($timeout))")
  WAIT_LAST_VALUE=""
  while :; do
    if [[ -f "$file" ]]; then
      WAIT_LAST_VALUE="$(jq -r ".${dp} // empty" "$file" 2>/dev/null || echo '')"
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

# now_s — print current wall seconds as float.
now_s() {
  python3 -c "import time; print(time.time())"
}
