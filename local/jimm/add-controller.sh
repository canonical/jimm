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
JIMM_CONTROLLER_NAME="${JIMM_CONTROLLER_NAME:-jimm-dev}"
CONTROLLER_NAME="${CONTROLLER_NAME:-qa-lxd}"
CONTROLLER_YAML_PATH="${CONTROLLER_NAME}".yaml
CLIENT_CREDENTIAL_NAME="${CLIENT_CREDENTIAL_NAME:-localhost}"

echo
echo "JIMM controller name is: $JIMM_CONTROLLER_NAME"
echo "Target controller name is: $CONTROLLER_NAME"
echo "Target controller path is: $CONTROLLER_YAML_PATH"
echo
echo "Building jimmctl..."
# Build jimmctl so we may add a controller.
go build ./cmd/jimmctl
echo "Built."
echo 
echo "Switching juju controller to $JIMM_CONTROLLER_NAME" 
juju switch "$JIMM_CONTROLLER_NAME"
echo
echo "Retrieving controller info for $CONTROLLER_NAME"
./jimmctl controller-info --local "$CONTROLLER_NAME" "$CONTROLLER_YAML_PATH"
if [[ -f "$CONTROLLER_YAML_PATH" ]]; then
    echo "Controller info retrieved."
else
    echo "Controller info couldn't be created, exiting..."
    exit 1
fi
echo
echo "Adding controller from path: $CONTROLLER_YAML_PATH"
./jimmctl add-controller "$CONTROLLER_YAML_PATH"
echo
echo "Updating cloud credentials for: $JIMM_CONTROLLER_NAME, from client credential: $CLIENT_CREDENTIAL_NAME"
juju update-credentials "$CLIENT_CREDENTIAL_NAME" --controller "$JIMM_CONTROLLER_NAME"
