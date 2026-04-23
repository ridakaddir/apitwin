#!/usr/bin/env bash
# http.sh — thin wrappers around curl for the runtime-persistence QA harness.
set -euo pipefail

# call_http METHOD PATH [JSON_BODY]
# Sets globals HTTP_STATUS, HTTP_BODY, HTTP_RC.
# PATH is just the URI path, joined to http://127.0.0.1:$HTTP_PORT.
call_http() {
  local method="$1" path="$2" body="${3-}"
  local url="http://127.0.0.1:${HTTP_PORT}${path}"
  local out_file status_file
  out_file="$(mktemp)"
  status_file="$(mktemp)"

  local curl_args=(
    -sS
    -X "$method"
    -o "$out_file"
    -w '%{http_code}'
    --max-time 10
  )
  if [[ -n "$body" ]]; then
    curl_args+=( -H 'Content-Type: application/json' -d "$body" )
  fi
  curl_args+=( "$url" )

  set +e
  curl "${curl_args[@]}" > "$status_file" 2>/dev/null
  HTTP_RC=$?
  set -e

  HTTP_STATUS="$(cat "$status_file" 2>/dev/null || echo '000')"
  HTTP_BODY="$(cat "$out_file" 2>/dev/null || echo '')"
  rm -f "$out_file" "$status_file"
  return 0
}

# get_field JSON DOTPATH — extract a field using jq, printing empty on missing.
get_field() {
  local json="$1" path="$2"
  printf '%s' "$json" | jq -r ".${path} // empty" 2>/dev/null || true
}
