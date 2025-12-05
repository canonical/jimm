#!/bin/bash

# This test upgrades a single model juju controller from one version to another
# utilising JAAS' upgrade-to command.

set -euo pipefail

# TODO: This test is very slow, and appears to be slowing the runner down to a 
# complete hault.
# See: https://warthogs.atlassian.net/browse/JUJU-8938
# source "local/jimm/detect-jaas.sh"

# # Reset Juju on EXIT.
# trap 'sudo snap refresh juju --channel=3/stable' EXIT

# export CONTROLLER_NAME="man-of-iron"
# JIMM_CONTROLLER_NAME="${JIMM_CONTROLLER_NAME:-jimm-dev}"

# # Switch to an older Juju temporarily, so that we can upgrade the model when migrated.
# sudo snap refresh juju --channel=3/stable --revision=32912

# echo
# echo "Bootstrapping lxd controller configured with login-token-refresh-url"
# local/jimm/setup-controller.sh

# echo
# echo "Adding controller to jimm"
# local/jimm/add-controller.sh

# echo
# echo "Upgrading Juju 3.6.11 model to 3.6.12"
# $JAAS upgrade-to 3.6.12 $(juju show-model test-lxd | yq '.test-lxd.model-uuid')

# echo 
# echo "Verifying model has been migrated from the original controller"
# juju switch man-of-iron
# if jujy models --format json | jq -e '.models | (length == 1) and (.[0].name == "admin/controller")'; then
#   echo "Success: Only the controller model remains."
# else
#   echo "Failure: Expected only the controller model, but found something else."
#   juju models
#   exit 1
# fi

# echo
# echo "Verifying model has been upgraded from 3.6.11"
# juju switch $JIMM_CONTROLLER_NAME:test-lxd
# if juju status --format json | jq -e '.model.version != "3.6.11"'; then
#   echo "Validation successful: The model version is not 3.6.11."
#   # Script continues...
# else
#   echo "Validation failed: The model version is still 3.6.11."
#   exit 1
# fi