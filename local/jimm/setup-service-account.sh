#!/bin/bash

# This script is used to setup a service account by adding a set of cloud-credentials.
# Default values below assume a lxd controller is added to JIMM.

set -eux

SERVICE_ACCOUNT_ID="${SERVICE_ACCOUNT_ID:-test-client-id}"
CLOUD="${CLOUD:-localhost}"
CREDENTIAL_NAME="${CREDENTIAL_NAME:-localhost}"

juju add-service-account "$SERVICE_ACCOUNT_ID"
juju update-service-account-credential "$SERVICE_ACCOUNT_ID" "$CLOUD" "$CREDENTIAL_NAME"
