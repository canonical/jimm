#!/bin/bash

# This script is used to setup a service account by making it a JIMM admin.
# Default values below assume a lxd controller is added to JIMM.

set -eux

SERVICE_ACCOUNT_ID="${SERVICE_ACCOUNT_ID:-test-client-id}"
CLOUD="${CLOUD:-localhost}"
CREDENTIAL_NAME="${CREDENTIAL_NAME:-localhost}"

# Source the `JAAS` variable for executing jaas commands.
source "$(dirname "${BASH_SOURCE[0]}")/detect-jaas.sh"

$JAAS add-permission user-"$SERVICE_ACCOUNT_ID"@serviceaccount administrator controller-jimm
