#!/bin/bash

# This script is used to grant all users with access to a controller
# in JAAS, this is useful for CI where we want to allow all users
# to add models to a backing controller.

set -eux

# Source the `JAAS` variable for executing jaas commands.
source "$(dirname "${BASH_SOURCE[0]}")/detect-jaas.sh"

$JAAS add-permission user-everyone@external can_addmodel controller-"$CONTROLLER_NAME"
