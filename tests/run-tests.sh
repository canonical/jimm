#!/bin/bash

# Run all executable files in all subfolders of the current directory
# and stop execution if any test fails.
# This script is intended to be run from the root of the project
# and will execute all test scripts found in the `tests` directory.
#
# The test runner supports running tests in parallel, with a configurable
# maximum number of concurrent jobs. Change max_jobs to adjust this.

set -euo pipefail

TESTS_DIR="tests"

echo "Running tests in $TESTS_DIR"
max_jobs=3
job_count=0

declare -A job_scripts
declare -A job_start_times
fail=0

# Function to handle waiting for a job and reporting result
wait_and_report() {
    local pid="$1"
    wait "$pid"
    local status=$?
    local end_time # Avoid masking error return values SC2155
    end_time=$(date +%s)
    local duration
    duration=$((end_time - job_start_times[$pid]))
    if [ "$status" -eq 0 ]; then
        echo
        echo "Test passed: ${job_scripts[$pid]} (Duration: ${duration}s)"
        echo
    else
        echo
        echo "Test failed: ${job_scripts[$pid]} (Duration: ${duration}s)"
        fail=1
    fi
    unset "job_scripts[$pid]"
    unset "job_start_times[$pid]"
}

for folder in "$TESTS_DIR"/*/ ; do
    # Skip if not a directory
    [ -d "$folder" ] || continue
    for test_script in "$folder"*; do
        # Only run regular files that are executable and not directories
        if [ -f "$test_script" ] && [ -x "$test_script" ]; then
            echo "Running $test_script"
            echo
            start_time=$(date +%s)
            "$test_script" &
            pid=$!
            job_scripts[$pid]="$test_script"
            job_start_times[$pid]=$start_time
            job_count=$((job_count + 1))
            if [ "$job_count" -ge "$max_jobs" ]; then
                # Wait for any job to finish before starting another
                wait -n
                # Check exit status of all background jobs
                for p in "${!job_scripts[@]}"; do
                    if ! kill -0 "$p" 2>/dev/null; then
                        wait_and_report "$p"
                        job_count=$((job_count - 1))
                    fi
                done
            fi
        fi
    done
done


# Wait for remaining jobs
for p in "${!job_scripts[@]}"; do
    wait_and_report "$p"
done

if [ "$fail" -ne 0 ]; then
    exit 1
fi
