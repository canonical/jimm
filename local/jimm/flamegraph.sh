#!/usr/bin/env bash

# Capture a CPU profile from a running JIMM while the SSH benchmark runs,
# then render it as an interactive flame graph.
#
# Usage:
#   ./flamegraph.sh [seconds] [pprof_url]
#
# Env:
#   JIMM_PPROF_URL   base URL of JIMM's internal server (default: http://localhost:17090)
#   CONCURRENCY      sessions for the benchmark (default: 10)
#   WORKLOAD         ssh benchmark workload, or scp for a 1 GiB juju scp upload
#                    (default: stream)
#   SCP_UNIT         target unit for WORKLOAD=scp (default: 0)
#   SIZE             upload size for WORKLOAD=scp (default: 1G)
#   PPROF_UI_PORT    local port for the pprof web UI (default: 8081)
#
# Requirements: go (for `go tool pprof`), docker, juju.

set -euo pipefail

SECONDS_TO_CAPTURE="${1:-30}"
JIMM_PPROF_URL="${JIMM_PPROF_URL:-http://localhost:17090}"
CONCURRENCY="${CONCURRENCY:-10}"
WORKLOAD="${WORKLOAD:-stream}"
BENCH_SCRIPT="$(cd "$(dirname "$0")" && pwd)/ssh-stream-benchmark.sh"
SCP_BENCH_SCRIPT="$(cd "$(dirname "$0")" && pwd)/juju-scp-benchmark.sh"

echo "Checking pprof endpoint at $JIMM_PPROF_URL ..."
curl -sf "$JIMM_PPROF_URL/debug/pprof/" >/dev/null || {
    echo "ERROR: pprof not reachable at $JIMM_PPROF_URL/debug/pprof/"
    echo "Is JIMM running with the pprof endpoints (rebuild jimm-dev)?"
    exit 1
}

PROFILE="/tmp/jimm-cpu-${WORKLOAD}-$(date +%s).pprof"
echo "Capturing ${SECONDS_TO_CAPTURE}s CPU profile while running the ${WORKLOAD} benchmark..."

# Run the benchmark in the background. The stream benchmark includes a 10s
# baseline; the SCP benchmark measures one real `juju scp` upload instead.
if [ "$WORKLOAD" = "scp" ]; then
    SCP_UNIT="${SCP_UNIT:-0}"
    SIZE="${SIZE:-1G}"
    echo "SCP target: $SCP_UNIT; upload size: $SIZE"
    SIZE="$SIZE" "$SCP_BENCH_SCRIPT" "$SCP_UNIT" &
else
    CONCURRENCY="$CONCURRENCY" WORKLOAD="$WORKLOAD" \
        "$BENCH_SCRIPT" "$CONCURRENCY" "$SECONDS_TO_CAPTURE" &
fi
BENCH_PID=$!

curl -sf "$JIMM_PPROF_URL/debug/pprof/profile?seconds=${SECONDS_TO_CAPTURE}" -o "$PROFILE"
wait "$BENCH_PID" || true

echo
echo "Profile saved to $PROFILE"
echo "Top functions (where the time goes):"
go tool pprof -top -nodecount=15 "$PROFILE"

echo
echo "Generating flame graph..."
UI_PORT="${PPROF_UI_PORT:-8081}"
echo "pprof web UI: http://localhost:${UI_PORT}/  (VIEW -> Flame Graph)"
echo "Leave this script running while you browse; Ctrl-C to exit."
go tool pprof -http="localhost:${UI_PORT}" "$PROFILE"
