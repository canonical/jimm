#!/bin/bash

# This test upgrades a single model from one version to another
# utilising JAAS' upgrade-to command.

# This test is currently skipped due to:
# 1. jaas add-model command not yet available.

set -euo pipefail

source "local/jimm/detect-jaas.sh"

# See: https://warthogs.atlassian.net/browse/JUJU-8938
JIMM_CONTROLLER_NAME="${JIMM_CONTROLLER_NAME:-jimm-dev}"
UPGRADE_CONTROLLER="upgrade-source-controller"
SOURCE_CONTROLLER_VERSION="3.6.19"
# Generate a random 4-character suffix for the model name.
RAND_SUFFIX=$(tr -dc 'a-z0-9' </dev/urandom | head -c 4 || true)
UPGRADING_MODEL_NAME="upgrading-model-$RAND_SUFFIX"

# Setup controller with desired agent version then add the controller to JIMM.
echo
controller_exists=$(juju controllers | grep -c "$UPGRADE_CONTROLLER" || true)
if [[ "$controller_exists" -gt 0 ]]; then
    echo "Controller $UPGRADE_CONTROLLER already exists, skipping setup."
else
    echo "Setting up $UPGRADE_CONTROLLER"
    AGENT_VERSION="$SOURCE_CONTROLLER_VERSION" CONTROLLER_NAME="$UPGRADE_CONTROLLER" local/jimm/setup-controller.sh
    CONTROLLER_NAME="$UPGRADE_CONTROLLER" local/jimm/add-controller.sh
fi

# Switch to JIMM controller
juju switch "$JIMM_CONTROLLER_NAME"

# Create the model that will be upgraded.
echo
echo "Creating $UPGRADING_MODEL_NAME"
$JAAS add-model "$UPGRADING_MODEL_NAME" localhost --target-controller "$UPGRADE_CONTROLLER"

echo
echo "Fetching current model version"
model_info="$(juju show-model "$UPGRADING_MODEL_NAME" --format json)"
model_uuid="$(echo "$model_info" | jq -r ".[\"$UPGRADING_MODEL_NAME\"].\"model-uuid\"")"
current_model_version="$(echo "$model_info" | jq -r ".[\"$UPGRADING_MODEL_NAME\"].\"agent-version\"")"
echo "Current model version is $current_model_version"

if [ "$current_model_version" != "$SOURCE_CONTROLLER_VERSION" ]; then
    echo "Model should be at version $SOURCE_CONTROLLER_VERSION to perform upgrade test, but is at $current_model_version"
    exit 1
fi

echo
echo "Listing migration targets for $UPGRADING_MODEL_NAME"
migration_targets_yaml="$($JAAS list-migration-targets "$model_uuid" --format yaml)"
target_controller="$(echo "$migration_targets_yaml" | yq -r '.[0].name')"
target_controller_version="$(echo "$migration_targets_yaml" | yq -r '.[0].agentversion')"
if [[ -z "$target_controller" || "$target_controller" == "null" ]]; then
    echo "No valid migration target controllers found for model $UPGRADING_MODEL_NAME"
    exit 1
fi
if [[ -z "$target_controller_version" || "$target_controller_version" == "null" ]]; then
    echo "Unable to determine target controller version for $target_controller"
    exit 1
fi
echo "Selected migration target controller: $target_controller (agent version: $target_controller_version)"

echo
echo "Upgrading Juju $current_model_version model to $target_controller_version on target controller $target_controller"
$JAAS upgrade-to "$target_controller" "$model_uuid"

echo
echo "Verifying model has been upgraded and moved to $target_controller"
echo "Waiting for upgrade to complete (timeout: 5 minutes)..."
max_attempts=60  # 60 attempts = 5 minutes / 5 seconds
attempt=1
while [ $attempt -le $max_attempts ]; do
    sleep 5
    controller_info="$($JAAS show-model "$model_uuid" --format json)"
    current_controller="$(echo "$controller_info" | jq -r '."controller-name"')"
    if [ "$current_controller" = "$target_controller" ]; then
        echo "Model upgrade completed on controller $current_controller."
        break
    fi
    echo
    echo "Upgrade still in progress (attempt $attempt/$max_attempts), current backing controller: $current_controller"
    attempt=$((attempt + 1))
done
if [ $attempt -gt $max_attempts ]; then
    echo "Upgrade did not complete within 5 minutes."
    exit 1
fi

echo
echo "Upgrade test completed successfully."
