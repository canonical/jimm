#!/usr/bin/env bash

# Run the standard JIMM benchmark with concurrent 1 GiB `juju scp` uploads.
#
# This deliberately delegates collection to ssh-stream-benchmark.sh, producing
# the same metadata.env, container.csv, and juju-models.csv schema as every
# other workload. Analyze SCP alongside stream/logs with:
#
#   ./local/jimm/analyze_benchmark.py results/stream results/scp
#
# Usage:
#   ./juju-scp-benchmark.sh [concurrency]

set -euo pipefail

CONCURRENCY="${1:-10}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if ! [[ "$CONCURRENCY" =~ ^[1-9][0-9]*$ ]]; then
    echo "ERROR: concurrency must be a positive integer" >&2
    exit 1
fi

WORKLOAD=scp "$SCRIPT_DIR/ssh-stream-benchmark.sh" "$CONCURRENCY"
