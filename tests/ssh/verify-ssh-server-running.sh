#!/bin/bash

# This script verifies SSH access to a machine through JIMM's jump server.

set -euo pipefail

JIMM_CONTROLLER_NAME="${JIMM_CONTROLLER_NAME:-jimm-dev}"
BACKING_CONTROLLER_NAME="${BACKING_CONTROLLER_NAME:-qa-lxd}"
model_name="ssh-test-$RANDOM"
ssh_dir="$HOME/.ssh"
ssh_private_key="$ssh_dir/id_rsa"
ssh_public_key="$ssh_private_key.pub"
generated_default_key=false

# Source the `JAAS` variable for executing jaas commands.
source "local/jimm/detect-jaas.sh"

cleanup() {
	if [[ "$generated_default_key" == true ]]; then
		rm -f "$ssh_private_key" "$ssh_public_key"
	fi
	juju destroy-model "$model_name" --force --no-prompt --destroy-all-models 2>/dev/null || true
}
trap cleanup EXIT

# Tests run sequentially and share Juju's current controller/model. Start on
# JIMM's controller, then create and select this test's model before executing
# model-scoped commands.
juju switch "$JIMM_CONTROLLER_NAME"
$JAAS add-model "$model_name" localhost --target-controller "$BACKING_CONTROLLER_NAME"

mkdir -p "$ssh_dir"
if [[ ! -f "$ssh_public_key" || ! -f "$ssh_private_key" ]]; then
	ssh-keygen -q -t rsa -N "" -f "$ssh_private_key"
	generated_default_key=true
fi
juju add-ssh-key "$(cat "$ssh_public_key")"
juju add-machine
until [ "$(juju status 0 --format json | jq -r '.machines["0"]["juju-status"].current')" = "started" ]; do
	sleep 5
done
juju ssh 0 true
