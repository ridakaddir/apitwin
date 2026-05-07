#!/usr/bin/env bash
# 02-validation-fails-on-bad-config
#
# Pins the fail-fast contract: when a config has a transition case with
# persist+merge=update on a multi-message response shape AND neither
# explicit wrap/source nor inheritance nor auto-derive can recover, the
# binary must refuse to start. The user's "fail fast — never save a
# corrupting payload" requirement is enforced at startup, not at runtime.
set -euo pipefail

SCENARIO_ID="02-validation-fails-on-bad-config"
scenario_start

EXPECTED="apitwin refuses to start when an LRO transition case lacks wrap/source and neither inheritance nor auto-derive can supply them."

# Build a deliberately broken config in a tempdir so the harness's main
# config stays untouched.
bad_dir="$HARNESS_DIR/.bad_config_test"
rm -rf "$bad_dir"
mkdir -p "$bad_dir"
cp "$HARNESS_DIR/countries.proto" "$bad_dir/countries.proto"

cat > "$bad_dir/apitwin.toml" <<'EOF'
# Both fallback and transition cases are sparse on a multi-message LRO
# response. Inheritance has nothing to inherit; auto-derive returns
# ambiguous because the broken request input below has TWO Country fields.
# Validation must fail.

[[grpc_routes]]
match    = "/geo.lro.CountryService/UpdateCountry"
fallback = "updated"

  [[grpc_routes.transitions]]
  case = "ready"

  [grpc_routes.cases.updated]
  status  = 0
  file    = "stubs/x/{body.id}.json"
  persist = true
  merge   = "update"

  [grpc_routes.cases.ready]
  status  = 0
  file    = "stubs/x/{body.id}.json"
  persist = true
  merge   = "update"
EOF

# We pass the existing (good) proto. The fallback `updated` case has no
# wrap/source. UpdateCountryResponse has 2 message fields; only `country`
# correlates to the request → Shape 3 succeeds, validation passes.
# So instead, point at a method whose response has TWO message fields
# AND where the request also contains both → ambiguous correlation.
# We don't have such a method in countries.proto, so write a minimal
# secondary proto for this scenario.
cat > "$bad_dir/ambiguous.proto" <<'EOF'
syntax = "proto3";
package geo.lro.amb;

service AmbService {
  rpc Touch (TouchReq) returns (TouchResp);
}

message One { string a = 1; }
message Two { string b = 1; }

message TouchReq  { One one = 1; Two two = 2; }
message TouchResp { string region_id = 1; One one = 2; Two two = 3; }
EOF

# Repoint the route at the ambiguous method.
cat > "$bad_dir/apitwin.toml" <<'EOF'
[[grpc_routes]]
match    = "/geo.lro.amb.AmbService/Touch"
fallback = "u"
  [[grpc_routes.transitions]]
  case = "u"
  [grpc_routes.cases.u]
  file    = "stubs/x/y.json"
  persist = true
  merge   = "update"
EOF

# Run apitwin and capture exit + stderr. Use a fresh ephemeral binary path
# resolved from $BINARY (set by run.sh).
log_file="$bad_dir/apitwin.stderr"
set +e
"$BINARY" --config "$bad_dir" \
  --grpc-proto "$bad_dir/ambiguous.proto" \
  --grpc-port 65431 --port 65432 \
  --no-runtime-dir > "$bad_dir/apitwin.stdout" 2> "$log_file"
rc=$?
set -e

# Best-effort cleanup of any stray process (the validator should have
# refused to start, so usually nothing to kill — but be defensive).
pkill -f "$bad_dir" 2>/dev/null || true

if [[ $rc -eq 0 ]]; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "apitwin started successfully despite invalid config (exit $rc)" \
    "Startup validator missed the multi-message-response + sparse-case shape."
  rm -rf "$bad_dir"
  return 0 2>/dev/null || exit 0
fi

if ! grep -q "config validation failed" "$log_file"; then
  SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
  record_result "$SCENARIO_ID" fail "$EXPECTED" \
    "exit code $rc but stderr lacks 'config validation failed': $(head -c 500 "$log_file")" \
    "Validator surfaced a different error path than the formatted multi-line one."
  rm -rf "$bad_dir"
  return 0 2>/dev/null || exit 0
fi

rm -rf "$bad_dir"
SCENARIO_DURATION_MS=$(scenario_elapsed_ms)
record_result "$SCENARIO_ID" pass "$EXPECTED" "binary refused to start; stderr names the offender" ""
