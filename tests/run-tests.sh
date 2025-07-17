#!/bin/bash

# Run all executable files in all subfolders of the current directory
# and stop execution if any test fails.
# This script is intended to be run from the root of the project
# and will execute all test scripts found in the `tests` directory.

set -euo pipefail

TESTS_DIR="tests"

echo $"Running tests in $TESTS_DIR"
for folder in "$TESTS_DIR"/*/ ; do
    # Skip if not a directory
    [ -d "$folder" ] || continue
    for test_script in "$folder"*; do
        # Only run regular files that are executable and not directories
        if [ -f "$test_script" ] && [ -x "$test_script" ]; then
            echo "Running $test_script"
            echo
            start_time=$(date +%s)
            if ! "$test_script"; then
                end_time=$(date +%s)
                duration=$((end_time - start_time))
                echo
                echo "Test failed: $test_script (Duration: ${duration}s)"
                exit 1
            else
                end_time=$(date +%s)
                duration=$((end_time - start_time))
                echo
                echo "Test passed: $test_script (Duration: ${duration}s)"
                echo
            fi
        fi
    done
done
