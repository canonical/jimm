#!/bin/bash

# This script assumes that you have JIMM running with a controller
# named "qa-lxd" attached (can be overriden via BACKING_CONTROLLER_NAME).
#
# This script creates a controller called internal-migration-controller and 
# then creates a model called internal-migration-model, lists viable internal
# migration targets, and then migrates the model internally to another
# controller attached to JIMM before waiting for the migration to complete.
#
# Note that migrating a model back to a controller it was previously on
# is not supported, so the script will create a new model each time with a
# random suffix.
#
# Usage: ./tests/modelmigration/migrate-internal.sh 

set -euo pipefail

# Notes:
# - This script is partially idempotent, meaning you can run it multiple times
# without recreating the migration-controller, but this only applies up
# to a certain point. Because the model is always recreated, all steps after
# model creation are not idempotent.
#
# - The script uses || true in some places to avoid failing the script
# if a command has non-zero exit code, used to check if certain resources
# already exist. This is done to allow the script to be partially idempotent.

JIMM_CONTROLLER_NAME="${JIMM_CONTROLLER_NAME:-jimm-dev}"
INTERNAL_MIGRATION_CONTROLLER="internal-migration-controller"
# Generate a random 4-character suffix for the model name.
RAND_SUFFIX=$(tr -dc 'a-z0-9' </dev/urandom | head -c 4 || true)
MIGRATING_MODEL_NAME="internal-migration-model-$RAND_SUFFIX"

# Call add-controller.sh to add the controller to JIMM.
echo
controller_exists=$(juju controllers | grep -c "$INTERNAL_MIGRATION_CONTROLLER" || true)
if [[ "$controller_exists" -gt 0 ]]; then
    echo "Controller $INTERNAL_MIGRATION_CONTROLLER already exists, switching to $INTERNAL_MIGRATION_CONTROLLER and skipping setup."
    juju switch "$INTERNAL_MIGRATION_CONTROLLER"
else
    echo "Setting up $INTERNAL_MIGRATION_CONTROLLER"
    CONTROLLER_NAME="$INTERNAL_MIGRATION_CONTROLLER" local/jimm/setup-controller.sh
    CONTROLLER_NAME="$INTERNAL_MIGRATION_CONTROLLER" local/jimm/add-controller.sh
fi

# Switch back to JIMM controller
juju switch "$JIMM_CONTROLLER_NAME"

# Create the model that will be migrated.
echo
model_exists=$(juju models | grep -c "$MIGRATING_MODEL_NAME" || true)
if [[ "$model_exists" -gt 0 ]]; then
    echo "Model $MIGRATING_MODEL_NAME already exists, skipping creation and app deploy."
else
    echo "Creating $MIGRATING_MODEL_NAME"
    juju add-model "$MIGRATING_MODEL_NAME" localhost
fi

# I'm unable to determine exactly why this sleep is necessary but without it
# we always get the error 'source prechecks failed: controller: machine 0 not running (pending)'
# when starting the model migration. Even switching to the new controller and
# running `juju wait-for machine 0 --model controller` does not help. 
echo "Sleeping after model creation"
sleep 20

echo 
model_info=$(juju show-model "$MIGRATING_MODEL_NAME" --format json)
model_uuid=$(echo "$model_info" | jq -r ".[\"$MIGRATING_MODEL_NAME\"].\"model-uuid\"")
echo "Model UUID for $MIGRATING_MODEL_NAME is $model_uuid"

# Source the `JAAS` variable for executing jaas commands.
source "local/jimm/detect-jaas.sh"

# Run a command to list viable internal migration targets.
echo
echo "Listing viable internal migration targets for model $MIGRATING_MODEL_NAME"
migration_target=$($JAAS list-migration-targets "$model_uuid" --format yaml | yq '.[0].name')
if [[ -z "$migration_target" || "$migration_target" == "null" ]]; then
    echo "No viable internal migration targets found for model $MIGRATING_MODEL_NAME."
    exit 1
fi
echo "Found migration target: $migration_target"

# Migrate the model to the target controller.
echo
echo "Migrating model $MIGRATING_MODEL_NAME to target controller $migration_target"
migrate_output=$($JAAS migrate-internal "$migration_target" "$model_uuid")
echo "$migrate_output"
# Parse the output and check that error is null
error_val=$(echo "$migrate_output" | yq '.["results"][0].error')
if [[ "$error_val" != "null" ]]; then
    echo "Migration failed: error field is not null: $error_val"
    exit 1
fi

# Run list-migration-targets until the list does not contain the target controller.
echo
echo "Waiting for migration to complete (timeout: 2 minutes)..."
max_attempts=24  # 2 minutes / 5 seconds
attempt=1
while [ $attempt -le $max_attempts ]; do
    sleep 5
    current_targets=$($JAAS list-migration-targets "$model_uuid" --format yaml | yq '.[] | .name')
    if ! echo "$current_targets" | grep -q "$migration_target"; then
        echo "Migration completed, current targets are $current_targets."
        break
    fi
    echo "Migration still in progress, current targets: $current_targets (attempt $attempt/$max_attempts)"
    echo "Running juju show-model to trigger check for migration completion."
    juju show-model > /dev/null || true
    attempt=$((attempt + 1))
done
if [ $attempt -gt $max_attempts ]; then
    echo "Migration did not complete within 2 minutes."
    exit 1
fi

echo
echo "Migration completed successfully, validating with a Juju status command."
juju status
juju destroy-model "$MIGRATING_MODEL_NAME" --no-prompt
