#!/bin/bash

# This script is used to setup a service account by adding a set of cloud-credentials.
# The service account is also made an admin of JIMM.
# Default values below assume a lxd controller is added to JIMM.

set -eux

SERVICE_ACCOUNT_ID="${SERVICE_ACCOUNT_ID:-test-client-id}"
CLOUD="${CLOUD:-localhost}"
CREDENTIAL_NAME="${CREDENTIAL_NAME:-localhost}"

# the reason we use `/snap/jaas/current/bin/jaas` instead of `juju` is because we can't access jaas commands when we build juju
# instead of using the snap. 
/snap/jaas/current/bin/jaas add-service-account "$SERVICE_ACCOUNT_ID"
/snap/jaas/current/bin/jaas update-service-account-credential "$SERVICE_ACCOUNT_ID" "$CLOUD" "$CREDENTIAL_NAME"
jimmctl auth relation add user-"$SERVICE_ACCOUNT_ID"@serviceaccount administrator controller-jimm
