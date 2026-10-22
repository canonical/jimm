#!/bin/bash

# This script assumes that you have JIMM running with a controller
# named "qa-lxd" attached (can be overriden via BACKING_CONTROLLER_NAME).
#
# This script creates a controller called source-controller and
# then creates two models: source-model and sink-model,
# relates them, and then migrates the source model to JIMM.
#
# The controller and model names can be overridden via SOURCE_CONTROLLER_NAME,
# PROVIDER_MODEL_NAME and CONSUMER_MODEL_NAME, and the Juju version of the
# source controller via JUJU_BOOTSTRAP_BINARY.
#
# Usage: ./tests/modelmigration/migrate-providing-model.sh 
set -euo pipefail

# Notes:
# - This script is partially idempotent, meaning you can run it multiple times
# without recreating the source-controller or models, but this only applies up
# to the point of migration. If migration fails, the script is not idempotent.
#
# - The script uses || true in some places to avoid failing the script
# if a command has non-zero exit code, used to check if certain resources
# already exist. This is done to allow the script to be partially idempotent.

SOURCE_CONTROLLER_NAME="${SOURCE_CONTROLLER_NAME:-source-controller}"
PROVIDER_MODEL_NAME="${PROVIDER_MODEL_NAME:-source-model}"
CONSUMER_MODEL_NAME="${CONSUMER_MODEL_NAME:-sink-model}"
JIMM_CONTROLLER_NAME="${JIMM_CONTROLLER_NAME:-jimm-dev}"
BACKING_CONTROLLER_NAME="${BACKING_CONTROLLER_NAME:-qa-lxd}"

# Call add-controller.sh and avoid connecting 
# the controller to JIMM. The controller does 
# however need to be able to route requests to JIMM.
if juju controllers --format json | jq -e --arg n "$SOURCE_CONTROLLER_NAME" '.controllers | has($n)' > /dev/null; then
    echo "Controller $SOURCE_CONTROLLER_NAME already exists, switching to $SOURCE_CONTROLLER_NAME and skipping setup."
    juju switch "$SOURCE_CONTROLLER_NAME"
else
    echo "Setting up $SOURCE_CONTROLLER_NAME"
    SKIP_CONNECT_JIMM="true" CONTROLLER_NAME="$SOURCE_CONTROLLER_NAME" local/jimm/setup-controller.sh
fi

# Create a cloud-init template that will be used in model config.
# This will ensure agents can resolve JIMM's address and connect
# to it using the testing CA certificate.
CLOUDINIT_TEMPLATE=$\
'preruncmd:
  - echo "%s    jimm.localhost" >> /etc/hosts
ca-certs:
  trusted:
    - |\n%s'

echo
echo "Rendering cloud-init config for models"
# shellcheck disable=SC2059 # We are using the variable as the printf template
CLOUDINIT_CONFIG=$(printf "$CLOUDINIT_TEMPLATE" "$(lxc network get lxdbr0 ipv4.address | cut -f1 -d/)" "$(cat local/traefik/certs/ca.crt | sed -e 's/^/      /')")

echo
echo "Using the following cloud-init config for models"
echo "$CLOUDINIT_CONFIG"

if juju models --format json | jq -e --arg n "$PROVIDER_MODEL_NAME" 'any(.models[]; ."short-name" == $n)' > /dev/null; then
    echo "Model $PROVIDER_MODEL_NAME already exists, skipping creation and app deploy."
else
    echo "Creating $PROVIDER_MODEL_NAME"
    echo "Adding source model"
    juju add-model "$PROVIDER_MODEL_NAME" --config cloudinit-userdata="$CLOUDINIT_CONFIG" localhost
    juju deploy juju-qa-dummy-source
    juju offer dummy-source:sink
fi

echo
if juju models --format json | jq -e --arg n "$CONSUMER_MODEL_NAME" 'any(.models[]; ."short-name" == $n)' > /dev/null; then
    echo "Model $CONSUMER_MODEL_NAME already exists, skipping creation and app deploy."
else
    echo "Adding sink model"
    juju add-model "$CONSUMER_MODEL_NAME" --config cloudinit-userdata="$CLOUDINIT_CONFIG" localhost
    juju deploy juju-qa-dummy-sink
fi

echo
echo "Changing admin password for $SOURCE_CONTROLLER_NAME"
juju change-user-password admin --no-prompt <<< "test-password"

user_exists=$(juju users | grep -c "alice" || true)
if [[ "$user_exists" -gt 0 ]]; then
    echo "User alice already exists, skipping user creation."
else
    echo "Adding local user and permissions to $SOURCE_CONTROLLER_NAME"
    juju add-user alice
    juju change-user-password alice --no-prompt <<< "test-password"
fi

echo
echo "Granting permissions to alice on $SOURCE_CONTROLLER_NAME"
juju grant alice admin "$CONSUMER_MODEL_NAME" || true # For accessing the model.
juju grant alice admin "$PROVIDER_MODEL_NAME" || true # For changing config of the dummy-sink app.
juju grant alice consume admin/"$PROVIDER_MODEL_NAME".dummy-source || true # For consuming the offer.

echo
echo "Logging in to $SOURCE_CONTROLLER_NAME as local user"
juju logout 
juju login -c "$SOURCE_CONTROLLER_NAME" -u alice --no-prompt <<< "test-password"

juju switch admin/"$CONSUMER_MODEL_NAME"

relation_exists=$(juju status --relations | grep -c "dummy-sink:source" || true)
if [[ "$relation_exists" -gt 0 ]]; then
    echo "Relation already exists, skipping relation creation."
else
    echo "Relating source and sink models"
    juju relate dummy-sink admin/"$PROVIDER_MODEL_NAME".dummy-source
fi

echo
echo "Setting up token for relation"
juju config -m admin/"$PROVIDER_MODEL_NAME" dummy-source token=abc

echo
echo "Switching back to admin user"
juju logout
juju login -c "$SOURCE_CONTROLLER_NAME" -u admin --no-prompt <<< "test-password"

echo
echo "Waiting for dummy-sink to be ready and receive token over the relation"
success=0
for _ in {1..120}; do
    app_status=$(juju status dummy-sink --format json | jq -r '.applications["dummy-sink"]["application-status"].current')
    if [[ "$app_status" == "active" ]]; then
        success=1
        break
    fi
    sleep 5
done
if [[ $success -ne 1 ]]; then
    echo "dummy-sink did not become active within 10 minutes."
    exit 1
fi

# From this point on, the test is not idempotent
# because it becomes it becomes more complex to
# decide how to determine if steps were already done.

# Source the `JAAS` variable for executing jaas commands.
source "local/jimm/detect-jaas.sh"

echo
echo "Migrating $PROVIDER_MODEL_NAME to JIMM"
cat <<EOF > ./user-mapping.yaml
admin: jimm-test@canonical.com
alice: jimm-test@canonical.com
EOF
$JAAS migrate admin/"$PROVIDER_MODEL_NAME" "$JIMM_CONTROLLER_NAME" --backing-controller="$BACKING_CONTROLLER_NAME" --user-mapping="./user-mapping.yaml"
rm ./user-mapping.yaml

echo
echo "Waiting for model migration to complete"
echo "Switching to $JIMM_CONTROLLER_NAME and waiting for the model to appear"
juju switch "$JIMM_CONTROLLER_NAME"
# Loop for up to 10 minutes (60 iterations of 10 seconds each)
success=0
for _ in {1..60}; do
    if juju show-model "$PROVIDER_MODEL_NAME" > /dev/null 2>&1; then
        success=1
        break
    fi
    sleep 10
done
if [[ $success -ne 1 ]]; then
    echo "Model $PROVIDER_MODEL_NAME did not appear after 10 minutes."
    exit 1
fi

echo
echo "Changing the token on the dummy-source app in the $JIMM_CONTROLLER_NAME controller"
juju config -m jimm-test@canonical.com/"$PROVIDER_MODEL_NAME" dummy-source token=def

echo
echo "Checking if the token change appears on the sink model"
# A JQ query to grab the token from the relation data bag.
JQ_TOKEN_QUERY='."dummy-sink/0"."relation-info"[0]."related-units"."dummy-source/0".data.token'
juju switch "$SOURCE_CONTROLLER_NAME"
# Get the token from the dummy-sink unit without quotes and compare it to "def".
# Retry for up to 1 minute (12 iterations of 5 seconds each).
success=0
for _ in {1..12}; do
    new_token=$(juju show-unit -m admin/"$CONSUMER_MODEL_NAME" dummy-sink/0 --format json | jq -r "$JQ_TOKEN_QUERY")
    if [[ "$new_token" == "def" ]]; then
        success=1
        break
    fi
    sleep 5
done
if [[ $success -ne 1 ]]; then
    echo "Token migration failed, expected 'def' but got '$new_token' after 10 tries."
    exit 1
fi

echo
echo "Token migration successful, got '$new_token' as expected."

echo
echo "Ensuring the dummy-sink app is still running with an active state"
success=0
for _ in {1..120}; do
    app_status=$(juju status -m admin/"$CONSUMER_MODEL_NAME" dummy-sink --format json | jq -r '.applications["dummy-sink"]["application-status"].current')
    if [[ "$app_status" == "active" ]]; then
        success=1
        break
    fi
    sleep 5
done
if [[ $success -ne 1 ]]; then
    echo "dummy-sink did not remain active within 10 minutes."
    exit 1
fi

echo
echo "Cleaning up: removing relation and destroying $CONSUMER_MODEL_NAME model"
juju remove-relation -m admin/"$CONSUMER_MODEL_NAME" dummy-sink dummy-source
juju remove-saas -m admin/"$CONSUMER_MODEL_NAME" dummy-source
juju destroy-model "$CONSUMER_MODEL_NAME" --no-prompt

echo
echo "Cleaning up: destroying $PROVIDER_MODEL_NAME model"
juju switch "$JIMM_CONTROLLER_NAME"
juju destroy-model "$PROVIDER_MODEL_NAME" --no-prompt

echo "Migration completed successfully."
exit 0
