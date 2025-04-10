#!/bin/bash

# RUN THIS SCRIPT FROM PROJECT ROOT!
#
# This script adds a local controller to your compose JIMM instance.
# Due to TLS SANs we need to modify JIMMs /etc/hosts to map to the SANs a controller certificate has.
#
# For completeness sake, the SANs are: DNS:anything, DNS:localhost, DNS:juju-apiserver, DNS:juju-mongodb
# "juju-apiserver" feels most appropriate, so we use this.
#
# Requirements to run this script:
# - yq (snap)

# Exit immediately if a command exits with a non-zero status.
set -e  

JIMM_CONTROLLER_NAME="${JIMM_CONTROLLER_NAME:-jimm-dev}"
CONTROLLER_NAME="${CONTROLLER_NAME:-qa-lxd}"
CLIENT_CREDENTIAL_NAME="${CLIENT_CREDENTIAL_NAME:-localhost}"
CUSTOM_CONTROLLER_HOSTNAME="${CUSTOM_CONTROLLER_HOSTNAME:-}"

# Source the `JAAS` variable for executing jaas commands.
source "$(dirname "${BASH_SOURCE[0]}")/detect-jaas.sh"

echo
echo "JIMM controller name is: $JIMM_CONTROLLER_NAME"
echo "Target controller name is: $CONTROLLER_NAME"
echo
echo "Switching juju controller to $JIMM_CONTROLLER_NAME" 
juju switch "$JIMM_CONTROLLER_NAME"
echo
echo "Registering controller $CONTROLLER_NAME with JIMM"
$JAAS register-controller "$CONTROLLER_NAME" --local --tls-hostname juju-apiserver --public-address="$CUSTOM_CONTROLLER_HOSTNAME"
echo
echo "Updating cloud credentials for: $JIMM_CONTROLLER_NAME, from client credential: $CLIENT_CREDENTIAL_NAME"
juju update-credentials "$CLIENT_CREDENTIAL_NAME" --controller "$JIMM_CONTROLLER_NAME"
