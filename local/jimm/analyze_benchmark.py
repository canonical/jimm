#!/usr/bin/env python3
"""Summarize and plot JIMM SSH benchmark run directories.

Each input directory must be created by ssh-stream-benchmark.sh and contain:
  metadata.env, container.csv, and juju-models.csv.

Usage:
    ./analyze_benchmark.py RESULTS_DIR
    ./analyze_benchmark.py RUN_DIR [RUN_DIR ...]

Outputs, next to the first run directory:
  benchmark-summary.csv        phase-level numerical comparison

matplotlib is required to display charts. It is intentionally the only Python
runtime dependency; collection itself remains shell-only.
"""

from __future__ import annotations

import csv
import math
import statistics
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable


PHASES = ("baseline", "load", "cooldown")


@dataclass(frozen=True)
class ContainerSample:
    timestamp: float
    phase: str
    cpu_pct: float
    mem_mib: float
    rx_kib: float
    tx_kib: float
    fds: int


@dataclass(frozen=True)
class ModelsProbe:
    started_at: float
    finished_at: float
    phase: str
    duration_seconds: float
    exit_code: int


@dataclass
class Run:
    path: Path
    metadata: dict[str, str]
    container: list[ContainerSample]
    models: list[ModelsProbe]

    @property
    def label(self) -> str:
        workload = self.metadata.get("workload", "unknown")
        concurrency = self.metadata.get("concurrency", "?")
        return f"{self.path.name}: {workload} × {concurrency}"


def parse_metadata(path: Path) -> dict[str, str]:
    metadata: dict[str, str] = {}
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        key, separator, value = line.partition("=")
        if not separator:
            raise ValueError(f"invalid metadata line in {path}: {line!r}")
        metadata[key] = value
    return metadata


def require_columns(path: Path, fieldnames: list[str] | None, required: set[str]) -> None:
    missing = required - set(fieldnames or [])
    if missing:
        raise ValueError(f"{path} is missing CSV columns: {', '.join(sorted(missing))}")


def parse_memory_mib(value: str) -> float:
    """Parse Docker memory values into MiB.

    Current collectors write Docker's value (for example ``50.84MiB``), while
    early development output used a plain numeric MiB value. Accept both so
    run directories remain comparable across collector revisions.
    """
    value = value.strip()
    units = {
        "KiB": 1 / 1024,
        "MiB": 1,
        "GiB": 1024,
        "TiB": 1024 * 1024,
    }
    for unit, scale in units.items():
        if value.endswith(unit):
            return float(value.removesuffix(unit)) * scale
    return float(value)


def load_run(path_text: str) -> Run:
    path = Path(path_text).resolve()
    if not path.is_dir():
        raise ValueError(f"not a benchmark run directory: {path}")

    metadata = parse_metadata(path / "metadata.env")
    container: list[ContainerSample] = []
    with (path / "container.csv").open(newline="") as file:
        reader = csv.DictReader(file)
        require_columns(
            path / "container.csv",
            reader.fieldnames,
            {"timestamp", "phase", "cpu_pct", "mem_mib", "net_rx_kib_cum", "net_tx_kib_cum", "fds"},
        )
        for row in reader:
            container.append(
                ContainerSample(
                    timestamp=float(row["timestamp"]),
                    phase=row["phase"],
                    cpu_pct=float(row["cpu_pct"]),
                    mem_mib=parse_memory_mib(row["mem_mib"]),
                    rx_kib=float(row["net_rx_kib_cum"]),
                    tx_kib=float(row["net_tx_kib_cum"]),
                    fds=int(row["fds"]),
                )
            )

    models: list[ModelsProbe] = []
    with (path / "juju-models.csv").open(newline="") as file:
        reader = csv.DictReader(file)
        require_columns(
            path / "juju-models.csv",
            reader.fieldnames,
            {"started_at", "finished_at", "phase", "duration_seconds", "exit_code"},
        )
        for row in reader:
            models.append(
                ModelsProbe(
                    started_at=float(row["started_at"]),
                    finished_at=float(row["finished_at"]),
                    phase=row["phase"],
                    duration_seconds=float(row["duration_seconds"]),
                    exit_code=int(row["exit_code"]),
                )
            )
    return Run(path, metadata, container, models)


def expand_run_paths(arguments: list[str]) -> list[str]:
    """Expand a results directory into its immediate benchmark run directories.

    A run directory is identified by its metadata.env file. This deliberately
    avoids recursively searching arbitrary directories or trying to parse the
    aggregate benchmark-summary.csv as a run.
    """
    paths: list[str] = []
    for argument in arguments:
        path = Path(argument)
        if (path / "metadata.env").is_file():
            paths.append(argument)
            continue
        if path.is_dir():
            runs = sorted(child for child in path.iterdir() if (child / "metadata.env").is_file())
            if not runs:
                raise ValueError(f"no benchmark run directories found in: {path}")
            paths.extend(str(run) for run in runs)
            continue
        raise ValueError(f"not a benchmark run or results directory: {path}")
    return paths


def phase_values(items: Iterable[object], phase: str, attribute: str) -> list[float]:
    return [float(getattr(item, attribute)) for item in items if getattr(item, "phase") == phase]


def mean(values: list[float]) -> float:
    return statistics.fmean(values) if values else math.nan


def percentile(values: list[float], percentile_value: float) -> float:
    if not values:
        return math.nan
    ordered = sorted(values)
    if len(ordered) == 1:
        return ordered[0]
    position = (len(ordered) - 1) * percentile_value / 100
    low, high = math.floor(position), math.ceil(position)
    return ordered[low] + (ordered[high] - ordered[low]) * (position - low)


def phase_summary(run: Run, phase: str) -> dict[str, float]:
    cpu = phase_values(run.container, phase, "cpu_pct")
    mem = phase_values(run.container, phase, "mem_mib")
    fds = phase_values(run.container, phase, "fds")
    probes = [probe for probe in run.models if probe.phase == phase]
    successful = [probe.duration_seconds for probe in probes if probe.exit_code == 0]

    rx_rate = tx_rate = math.nan
    samples = [sample for sample in run.container if sample.phase == phase]
    if len(samples) >= 2:
        elapsed = samples[-1].timestamp - samples[0].timestamp
        if elapsed > 0:
            rx_rate = (samples[-1].rx_kib - samples[0].rx_kib) / elapsed
            tx_rate = (samples[-1].tx_kib - samples[0].tx_kib) / elapsed

    return {
        "container_samples": float(len(samples)),
        "cpu_mean_pct": mean(cpu),
        "cpu_max_pct": max(cpu, default=math.nan),
        "mem_mean_mib": mean(mem),
        "mem_max_mib": max(mem, default=math.nan),
        "fds_mean": mean(fds),
        "fds_max": max(fds, default=math.nan),
        "rx_kib_per_second": rx_rate,
        "tx_kib_per_second": tx_rate,
        "models_probes": float(len(probes)),
        "models_failures": float(sum(probe.exit_code != 0 for probe in probes)),
        "models_mean_seconds": mean(successful),
        "models_p50_seconds": percentile(successful, 50),
        "models_p95_seconds": percentile(successful, 95),
        "models_max_seconds": max(successful, default=math.nan),
    }


def write_summary(runs: list[Run], output: Path) -> None:
    fieldnames = [
        "run", "workload", "concurrency", "phase", "container_samples",
        "cpu_mean_pct", "cpu_max_pct", "mem_mean_mib", "mem_max_mib",
        "fds_mean", "fds_max", "rx_kib_per_second", "tx_kib_per_second",
        "models_probes", "models_failures", "models_mean_seconds",
        "models_p50_seconds", "models_p95_seconds", "models_max_seconds",
    ]
    with output.open("w", newline="") as file:
        writer = csv.DictWriter(file, fieldnames=fieldnames)
        writer.writeheader()
        for run in runs:
            for phase in PHASES:
                writer.writerow({
                    "run": run.path.name,
                    "workload": run.metadata.get("workload", ""),
                    "concurrency": run.metadata.get("concurrency", ""),
                    "phase": phase,
                    **phase_summary(run, phase),
                })


def matplotlib():
    try:
        import matplotlib
        import matplotlib.pyplot as pyplot
    except ImportError as error:
        raise SystemExit("matplotlib is required: install it with 'python3 -m pip install matplotlib'") from error
    backend = matplotlib.get_backend().lower()
    # QtAgg, TkAgg, and GTK*Agg are interactive despite their names. Only the
    # plain Agg backend is the file-only renderer.
    if backend == "agg":
        raise SystemExit(
            f"matplotlib selected the non-interactive '{matplotlib.get_backend()}' backend; "
            "install a GUI backend such as PyQt6 in the active Python environment "
            "(python3 -m pip install PyQt6) and rerun the analyzer"
        )
    return pyplot


def add_phase_boundaries(axis, run: Run) -> None:
    if not run.container:
        return
    origin = run.container[0].timestamp
    for key, label, colour in (
        ("load_started_at", "load begins", "tab:red"),
        ("cooldown_started_at", "cooldown begins", "tab:green"),
    ):
        if key in run.metadata:
            axis.axvline(float(run.metadata[key]) - origin, color=colour, alpha=0.7, linewidth=1, label=label)


def plot_run(run: Run) -> None:
    plt = matplotlib()
    if not run.container:
        raise ValueError(f"{run.path} contains no container samples")
    origin = run.container[0].timestamp
    seconds = [sample.timestamp - origin for sample in run.container]
    figure, axes = plt.subplots(3, 1, figsize=(13, 11), sharex=True, layout="constrained")
    figure.suptitle(f"JIMM benchmark — {run.label}")

    axes[0].plot(seconds, [sample.cpu_pct for sample in run.container], label="CPU %", color="tab:red")
    axes[0].set_ylabel("CPU %")
    axes[1].plot(seconds, [sample.mem_mib for sample in run.container], label="memory", color="tab:purple")
    axes[1].set_ylabel("Memory (MiB)")

    probes = [probe for probe in run.models if probe.exit_code == 0]
    if probes:
        axes[2].scatter(
            [probe.finished_at - origin for probe in probes],
            [probe.duration_seconds * 1000 for probe in probes],
            label="juju models", color="tab:blue", s=20,
        )
    failed = [probe for probe in run.models if probe.exit_code != 0]
    if failed:
        axes[2].scatter(
            [probe.finished_at - origin for probe in failed],
            [probe.duration_seconds * 1000 for probe in failed],
            label="failed juju models", color="tab:red", marker="x", s=45,
        )
    axes[2].set_ylabel("models latency (ms)")
    axes[2].set_xlabel("Seconds since first container sample")

    for axis in axes:
        add_phase_boundaries(axis, run)
        axis.grid(alpha=0.25)
        axis.legend(loc="best")
    axes[1].set_title("Memory")

def plot_comparison(runs: list[Run]) -> None:
    plt = matplotlib()
    metrics = ("models_mean_seconds", "models_p95_seconds", "cpu_mean_pct", "rx_kib_per_second")
    titles = ("juju models mean latency", "juju models p95 latency", "mean container CPU", "RX throughput")
    units = ("seconds", "seconds", "%", "KiB/s")
    figure, axes = plt.subplots(2, 2, figsize=(14, 9), layout="constrained")
    labels = [run.label for run in runs]
    phases = ("baseline", "load", "cooldown")
    colours = ("tab:blue", "tab:red", "tab:green")

    for axis, metric, title, unit in zip(axes.flat, metrics, titles, units):
        positions = list(range(len(runs)))
        width = 0.24
        for index, (phase, colour) in enumerate(zip(phases, colours)):
            values = [phase_summary(run, phase)[metric] for run in runs]
            axis.bar([position + (index - 1) * width for position in positions], values, width, label=phase, color=colour)
        axis.set_title(title)
        axis.set_ylabel(unit)
        axis.set_xticks(positions, labels, rotation=20, ha="right")
        axis.grid(axis="y", alpha=0.25)
        axis.legend()

def format_number(value: float) -> str:
    return "n/a" if math.isnan(value) else f"{value:.3f}"


def print_summary(runs: list[Run]) -> None:
    for run in runs:
        print(f"\n{run.label}")
        print("phase       models mean/p95 (s)   CPU mean (%)   RX (KiB/s)   failed probes")
        for phase in PHASES:
            summary = phase_summary(run, phase)
            print(
                f"{phase:<10} {format_number(summary['models_mean_seconds']):>7}/"
                f"{format_number(summary['models_p95_seconds']):<7} "
                f"{format_number(summary['cpu_mean_pct']):>12} "
                f"{format_number(summary['rx_kib_per_second']):>12} "
                f"{int(summary['models_failures']):>14}"
            )


def main() -> None:
    if len(sys.argv) < 2:
        raise SystemExit("usage: analyze_benchmark.py RESULTS_DIR | RUN_DIR [RUN_DIR ...]")
    try:
        runs = [load_run(path) for path in expand_run_paths(sys.argv[1:])]
    except (OSError, ValueError) as error:
        raise SystemExit(f"error: {error}") from error

    output_dir = runs[0].path.parent
    summary_path = output_dir / "benchmark-summary.csv"
    write_summary(runs, summary_path)
    print_summary(runs)
    print(f"\nWrote {summary_path}")
    for run in runs:
        plot_run(run)
    if len(runs) > 1:
        plot_comparison(runs)
    print("Displaying charts; close their windows to exit.")
    matplotlib().show()


if __name__ == "__main__":
    main()
