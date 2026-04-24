#!/bin/bash
# bench-remote.sh — measure CGI response times on a remote server.
# リモートサーバーの CGI 応答時間を計測する。
#
# Usage:
#   ./scripts/bench-remote.sh https://murlog.alarky.dev alarky
#   ./scripts/bench-remote.sh https://murlog.alarky.dev alarky 10

set -euo pipefail

BASE_URL="${1:?Usage: $0 <base_url> <username> [iterations]}"
USERNAME="${2:?Usage: $0 <base_url> <username> [iterations]}"
ITERATIONS="${3:-5}"

echo "=== murlog Performance Test ==="
echo "URL: $BASE_URL"
echo "User: $USERNAME"
echo "Iterations: $ITERATIONS"
echo ""

# measure runs curl N times and prints avg ms + QPS.
# curl を N 回実行して平均 ms と QPS を表示する。
measure() {
  local label="$1"; shift
  local total_us=0
  for i in $(seq 1 "$ITERATIONS"); do
    # time_total is in seconds with decimals (e.g. 0.123456).
    local t
    t=$(curl -s -o /dev/null -w "%{time_total}" "$@")
    # Convert to microseconds for integer arithmetic.
    # マイクロ秒に変換して整数演算する。
    local us
    us=$(echo "$t * 1000000" | bc | cut -d. -f1)
    total_us=$((total_us + us))
  done
  local avg_us=$((total_us / ITERATIONS))
  local avg_ms=$((avg_us / 1000))
  local qps
  if [ "$avg_us" -gt 0 ]; then qps=$((1000000 / avg_us)); else qps="∞"; fi
  printf "  %-35s avg: %dms  (~%s QPS)\n" "$label" "$avg_ms" "$qps"
}

echo "--- SSR / Static ---"
measure "Public home (SSR)" "${BASE_URL}/"
measure "Public profile (SSR)" "${BASE_URL}/users/${USERNAME}"
measure "Static file (favicon)" "${BASE_URL}/favicon.svg"

echo ""
echo "--- JSON-RPC (public, no auth) ---"
measure "posts.list (public)" \
  -X POST "${BASE_URL}/api/mur/v1/rpc" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"posts.list","params":{"limit":20}}'
measure "personas.list" \
  -X POST "${BASE_URL}/api/mur/v1/rpc" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"personas.list","params":{}}'

echo ""
echo "Done."
