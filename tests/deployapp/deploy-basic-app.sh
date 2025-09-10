#!/bin/bash

# This script creates a model and deploys a basic application to it
# before running `juju status` and then destroys the model.

set -euo pipefail

echo "Creating a new model and deploying haproxy"
juju add-model foo localhost
juju deploy haproxy

echo "Waiting 5 seconds and then running 'juju status'"
sleep 5
juju status

echo "Destroying model"
juju destroy-model foo --no-prompt
