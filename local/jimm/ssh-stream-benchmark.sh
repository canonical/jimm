#!/usr/bin/env bash

# Benchmark JIMM SSH forwarding while continuously measuring container health
# and the latency of an unrelated Juju API operation (`juju models`).
#
# The run directory contains data only; analyze_benchmark.py performs all
# aggregation and charting. Keeping collection and analysis separate makes
# results reproducible and straightforward to compare in reviews.
#
# Usage:
#   ./ssh-stream-benchmark.sh [concurrency] [stream_seconds]
#
# Environment:
#   JIMM_CONTAINER     Docker container to sample (default: jimm)
#   WORKLOAD           idle | interactive | logs | stream | scp | <remote command>
#   SCP_SIZE           file size for each concurrent SCP upload (default: 1G)
#   SCP_UNIT           target unit for SCP uploads (default: 0)
#   SCP_TIMEOUT_SECS   maximum duration of each SCP upload (default: 300)
#   BASELINE_SECS      pre-load measurement period (default: 10)
#   COOLDOWN_SECS      post-load measurement period (default: 10)
#   SAMPLE_INTERVAL    container sample interval in seconds (default: 1)
#   PROBE_INTERVAL     minimum interval between `juju models` probes (default: 1)
#   RUN_DIR            destination directory (default: benchmark-<timestamp>-<workload>)
#
# Output files:
#   metadata.env       run configuration and phase timestamps
#   container.csv      CPU, memory, network counters, and file descriptors
#   juju-models.csv    every `juju models` duration and exit status
#   scp-transfers.csv  timing and exit status of each SCP upload (SCP only)
#   sampler-errors.log failures collecting container metrics

set -euo pipefail

CONCURRENCY="${1:-10}"
STREAM_SECS="${2:-20}"
JIMM_CONTAINER="${JIMM_CONTAINER:-jimm}"
SAMPLE_INTERVAL="${SAMPLE_INTERVAL:-1}"
PROBE_INTERVAL="${PROBE_INTERVAL:-1}"
BASELINE_SECS="${BASELINE_SECS:-10}"
COOLDOWN_SECS="${COOLDOWN_SECS:-10}"
WORKLOAD="${WORKLOAD:-interactive}"
SCP_SIZE="${SCP_SIZE:-1G}"
SCP_UNIT="${SCP_UNIT:-0}"
SCP_TIMEOUT_SECS="${SCP_TIMEOUT_SECS:-300}"
RUN_DIR="${RUN_DIR:-benchmark-$(date +%Y%m%dT%H%M%S)-${WORKLOAD}}"
JUJU_SSH=(juju ssh 0)

case "$WORKLOAD" in
    idle)        REMOTE_CMD="timeout ${STREAM_SECS}s sleep ${STREAM_SECS}" ;;
    interactive) REMOTE_CMD="for i in \$(seq 1 40); do echo ok-\$i; sleep 0.5; done" ;;
    logs)        REMOTE_CMD="timeout ${STREAM_SECS}s sh -c 'while true; do head -c 1024 /dev/urandom | base64; sleep 0.05; done'" ;;
    stream)      REMOTE_CMD="timeout ${STREAM_SECS}s cat /dev/zero | base64" ;;
    scp)         REMOTE_CMD="" ;;
    *)           REMOTE_CMD="$WORKLOAD" ;;
esac

for bin in docker juju awk timeout stat; do
    command -v "$bin" >/dev/null || { echo "ERROR: $bin not found" >&2; exit 1; }
done

docker ps --format '{{.Names}}' | grep -qx "$JIMM_CONTAINER" || {
    echo "ERROR: container '$JIMM_CONTAINER' is not running." >&2
    exit 1
}
"${JUJU_SSH[@]}" 'echo ok' >/dev/null 2>&1 || {
    echo "ERROR: 'juju ssh 0' failed; verify the selected Juju model." >&2
    exit 1
}
if [[ "$WORKLOAD" == "scp" ]]; then
    command -v fallocate >/dev/null || { echo "ERROR: fallocate not found" >&2; exit 1; }
    juju ssh "$SCP_UNIT" 'echo ok' >/dev/null 2>&1 || {
        echo "ERROR: 'juju ssh $SCP_UNIT' failed; verify SCP_UNIT." >&2
        exit 1
    }
fi

mkdir -p "$RUN_DIR"
RUN_DIR="$(cd "$RUN_DIR" && pwd)"
PHASE_FILE="$RUN_DIR/.phase"
CONTAINER_CSV="$RUN_DIR/container.csv"
MODELS_CSV="$RUN_DIR/juju-models.csv"
ERROR_LOG="$RUN_DIR/sampler-errors.log"
SCP_CSV="$RUN_DIR/scp-transfers.csv"
SCP_SOURCE="$RUN_DIR/scp-source-${SCP_SIZE}.bin"

printf 'timestamp,phase,cpu_pct,mem_mib,net_rx_kib_cum,net_tx_kib_cum,fds\n' > "$CONTAINER_CSV"
printf 'started_at,finished_at,phase,duration_seconds,exit_code\n' > "$MODELS_CSV"
if [[ "$WORKLOAD" == "scp" ]]; then
    printf 'started_at,finished_at,session,source_bytes,remote_path,duration_seconds,exit_code\n' > "$SCP_CSV"
    # A sparse source avoids a large local disk write; juju scp still reads
    # and transfers its complete logical size.
    fallocate -l "$SCP_SIZE" "$SCP_SOURCE"
    SCP_SOURCE_BYTES=$(stat --format='%s' "$SCP_SOURCE")
fi
set_phase() {
    # Rename makes phase updates atomic for the two background collectors.
    printf '%s\n' "$1" > "${PHASE_FILE}.next"
    mv "${PHASE_FILE}.next" "$PHASE_FILE"
}
set_phase baseline

sample_container() {
    local stats cpu mem rx tx fds
    stats=$(docker stats --no-stream --format '{{.CPUPerc}},{{.MemUsage}},{{.NetIO}}' "$JIMM_CONTAINER" 2>/dev/null) || return 1
    [[ -n "$stats" ]] || return 1
    cpu=$(awk -F',' '{gsub(/%/, "", $1); print $1}' <<<"$stats")
    mem=$(awk -F',' '{split($2, m, " "); print m[1]}' <<<"$stats")
    rx=$(awk -F',' '{split($3, a, " / "); print a[1]}' <<<"$stats")
    tx=$(awk -F',' '{split($3, a, " / "); print a[2]}' <<<"$stats")
    # Docker reports cumulative NetIO in B/kB/MB/GB. Convert to KiB.
    for direction in rx tx; do
        local value="${!direction}"
        case "$value" in
            *GB*) value=$(awk -v v="${value//GB/}" 'BEGIN { printf "%.1f", v * 1048576 }') ;;
            *MB*) value=$(awk -v v="${value//MB/}" 'BEGIN { printf "%.1f", v * 1024 }') ;;
            *kB*|*KB*) value=$(awk -v v="${value%[kK]B}" 'BEGIN { printf "%.1f", v }') ;;
            *B*) value=$(awk -v v="${value%B}" 'BEGIN { printf "%.1f", v / 1024 }') ;;
            *) return 1 ;;
        esac
        printf -v "$direction" '%s' "$value"
    done
    fds=$(docker exec "$JIMM_CONTAINER" sh -c 'find /proc/[0-9]*/fd 2>/dev/null | wc -l' 2>/dev/null) || fds=0
    printf '%s,%s,%s,%s,%s,%s,%s\n' "$(date +%s.%N)" "$(<"$PHASE_FILE")" "$cpu" "$mem" "$rx" "$tx" "${fds:-0}"
}

sample_loop() {
    while :; do
        sample_container >> "$CONTAINER_CSV" || printf '%s container-sample-failed\n' "$(date +%s.%N)" >> "$ERROR_LOG"
        sleep "$SAMPLE_INTERVAL"
    done
}

models_probe_loop() {
    while :; do
        local started finished phase exit_code duration
        started=$(date +%s.%N)
        phase=$(<"$PHASE_FILE")
        if juju models --format json >/dev/null 2>&1; then
            exit_code=0
        else
            exit_code=$?
        fi
        finished=$(date +%s.%N)
        duration=$(awk -v start="$started" -v end="$finished" 'BEGIN { printf "%.6f", end - start }')
        printf '%s,%s,%s,%s,%s\n' "$started" "$finished" "$phase" "$duration" "$exit_code" >> "$MODELS_CSV"
        sleep "$PROBE_INTERVAL"
    done
}

SAMPLER_PID=""
PROBE_PID=""
cleanup() {
    [[ -n "$SAMPLER_PID" ]] && kill "$SAMPLER_PID" 2>/dev/null || true
    [[ -n "$PROBE_PID" ]] && kill "$PROBE_PID" 2>/dev/null || true
    [[ -n "$SAMPLER_PID" ]] && wait "$SAMPLER_PID" 2>/dev/null || true
    [[ -n "$PROBE_PID" ]] && wait "$PROBE_PID" 2>/dev/null || true
    if [[ "$WORKLOAD" == "scp" ]]; then
        rm -f "$SCP_SOURCE"
        for session in $(seq 1 "$CONCURRENCY"); do
            juju ssh "$SCP_UNIT" "rm -f /tmp/jimm-scp-benchmark-${session}.bin" >/dev/null 2>&1 || true
        done
    fi
    rm -f "$PHASE_FILE"
}
trap cleanup EXIT INT TERM

sample_loop & SAMPLER_PID=$!
models_probe_loop & PROBE_PID=$!

BASELINE_STARTED_AT=$(date +%s.%N)
echo "Baseline: ${BASELINE_SECS}s; measuring container metrics and 'juju models' latency."
sleep "$BASELINE_SECS"
BASELINE_FINISHED_AT=$(date +%s.%N)

set_phase load
LOAD_STARTED_AT=$(date +%s.%N)
if [[ "$WORKLOAD" == "scp" ]]; then
    echo "Load: $CONCURRENCY concurrent SCP uploads of $SCP_SIZE to $SCP_UNIT."
else
    echo "Load: $CONCURRENCY ${WORKLOAD} SSH sessions for ${STREAM_SECS}s."
fi
pids=()
for session in $(seq 1 "$CONCURRENCY"); do
    if [[ "$WORKLOAD" == "scp" ]]; then
        (
            started=$(date +%s.%N)
            remote_path="/tmp/jimm-scp-benchmark-${session}.bin"
            if timeout "$SCP_TIMEOUT_SECS" juju scp "$SCP_SOURCE" "$SCP_UNIT:$remote_path" >/dev/null 2>&1; then
                exit_code=0
            else
                exit_code=$?
            fi
            finished=$(date +%s.%N)
            duration=$(awk -v start="$started" -v end="$finished" 'BEGIN { printf "%.6f", end - start }')
            printf '%s,%s,%s,%s,%s,%s,%s\n' "$started" "$finished" "$session" "$SCP_SOURCE_BYTES" "$remote_path" "$duration" "$exit_code" >> "$SCP_CSV"
        ) &
    else
        "${JUJU_SSH[@]}" "$REMOTE_CMD" >/dev/null 2>&1 &
    fi
    pids+=("$!")
done
for pid in "${pids[@]}"; do wait "$pid" 2>/dev/null || true; done
LOAD_FINISHED_AT=$(date +%s.%N)

set_phase cooldown
COOLDOWN_STARTED_AT=$(date +%s.%N)
echo "Cooldown: ${COOLDOWN_SECS}s; continuing measurements."
sleep "$COOLDOWN_SECS"
COOLDOWN_FINISHED_AT=$(date +%s.%N)

cat > "$RUN_DIR/metadata.env" <<EOF
# Shell-compatible metadata for this benchmark run.
workload=$WORKLOAD
concurrency=$CONCURRENCY
stream_seconds=$STREAM_SECS
scp_size=$SCP_SIZE
scp_unit=$SCP_UNIT
scp_timeout_seconds=$SCP_TIMEOUT_SECS
baseline_seconds=$BASELINE_SECS
cooldown_seconds=$COOLDOWN_SECS
sample_interval_seconds=$SAMPLE_INTERVAL
probe_interval_seconds=$PROBE_INTERVAL
jimm_container=$JIMM_CONTAINER
baseline_started_at=$BASELINE_STARTED_AT
baseline_finished_at=$BASELINE_FINISHED_AT
load_started_at=$LOAD_STARTED_AT
load_finished_at=$LOAD_FINISHED_AT
cooldown_started_at=$COOLDOWN_STARTED_AT
cooldown_finished_at=$COOLDOWN_FINISHED_AT
EOF

echo
echo "Benchmark data written to: $RUN_DIR"
echo "Analyze it with: ./local/jimm/analyze_benchmark.py $RUN_DIR"
