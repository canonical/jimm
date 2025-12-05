#!/bin/bash

# This test upgrades a single model juju controller from one version to another
# utilising JAAS' upgrade-to command.

set -euo pipefail
source "local/jimm/detect-jaas.sh"

# Reset Juju on EXIT.
trap 'snap refresh juju --channel=3/stable' EXIT

export CONTROLLER_NAME="man-of-iron"
JIMM_CONTROLLER_NAME="${JIMM_CONTROLLER_NAME:-jimm-dev}"

# Switch to an older Juju temporarily, so that we can upgrade the model when migrated.
snap refresh juju --channel=3/stable --revision=32912

echo
echo "Bootstrapping lxd controller configured with login-token-refresh-url"
local/jimm/setup-controller.sh

echo
echo "Adding controller to jimm"
local/jimm/add-controller.sh

echo
echo "Upgrading Juju 3.6.11 model to 3.6.12"
$JAAS upgrade-to 3.6.12 $(juju show-model test-lxd | yq '.test-lxd.model-uuid')

