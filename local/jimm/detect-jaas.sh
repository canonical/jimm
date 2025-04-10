#!/bin/bash

# This script detects whether the jaas snap is installed 
# and if not, will build the tool from source.
# Use the JAAS variable after sourcing this script
# to access jaas commands.

# Exit immediately if a command exits with a non-zero status.
set -e  

if command -v jaas >/dev/null 2>&1; then
    echo "jaas available, skipping build"
else
    if [ ! -f ./jaas ]; then
        echo "Building jaas..."
        go build ./cmd/jaas
        echo "Built jaas."
        echo
    fi
fi

if [ -f ./jaas ]; then
    echo "using locally built jaas tool"
    JAAS="./jaas"
else
    if command -v juju | grep -q "snap"; then
        echo "juju cli available as a snap, using jaas as a Juju plugin"
        JAAS="juju jaas"
    else
        echo "juju cli snap not detected, running jaas binary directly"
        JAAS=$(command -v jaas)
    fi
fi

export JAAS
