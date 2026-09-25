# JIMM local benchmark tools

`ssh-stream-benchmark.sh` measures the impact of an SSH-forwarding workload
on the JIMM container and on an unrelated Juju API request. It writes raw CSV
data into one self-contained run directory; it deliberately does not calculate
results while the run is in progress.

Each run contains:

- `container.csv`: JIMM CPU, memory, cumulative network I/O, and open FDs;
- `juju-models.csv`: one timed `juju models --format json` probe per interval;
- `metadata.env`: workload configuration plus exact phase timestamps;
- `sampler-errors.log`: transient sampling failures, if any.

The phases are `baseline` (no SSH load), `load` (the concurrent SSH workload),
and `cooldown` (after all sessions have finished). This makes a performance
regression visible as a change in `juju models` latency during `load`, and
confirms whether it returns to baseline during `cooldown`.

## Collect a run

```bash
# Default interactive workload: 10 sessions for 20 seconds.
./local/jimm/ssh-stream-benchmark.sh

# Saturate the forwarding path and name the output explicitly.
WORKLOAD=stream RUN_DIR=results/chacha20 \
    ./local/jimm/ssh-stream-benchmark.sh 10 30

# Capture a directly comparable run after a configuration change.
WORKLOAD=stream RUN_DIR=results/aes-gcm \
    ./local/jimm/ssh-stream-benchmark.sh 10 30

# Run concurrent, end-to-end 1 GiB `juju scp` uploads in the same CSV schema.
RUN_DIR=results/scp ./local/jimm/juju-scp-benchmark.sh 10
```

Use the same concurrency, phase durations, selected Juju model, and target
unit for comparable runs. `JIMM_SSH_MAX_CONCURRENT_CONNECTIONS` must be at
least the requested concurrency. The SCP workload waits for all transfers to
finish rather than using `stream_seconds`; use `SCP_TIMEOUT_SECS` to bound a
transfer. It creates one unique temporary remote file per session and removes
all local and remote transfer files during cleanup.

## Analyze and compare runs

Install the plotting dependency once in the selected Python environment:

```bash
python3 -m pip install -r local/jimm/requirements-benchmark.txt
```

Then analyze one or more directories:

```bash
# Analyze every immediate run directory under results/.
./local/jimm/analyze_benchmark.py results

# Or select only the runs to compare.
./local/jimm/analyze_benchmark.py results/chacha20 results/aes-gcm
```

The analyzer writes:

- `benchmark-summary.csv` beside the run directories: mean/max container
  metrics and mean/p50/p95/max `juju models` durations per phase;

It opens an interactive chart for every run (CPU, memory, and every `juju
models` latency sample with load and cooldown boundaries), plus a comparison
chart when given two or more runs. Charts are intentionally not saved.

Only probes with exit code zero are included in latency aggregates; failures
are counted separately in `models_failures` and plotted as red crosses.