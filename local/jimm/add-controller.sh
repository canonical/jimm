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

JIMM_CONTROLLER_NAME=$1
CONTROLLER_NAME=$2
CONTROLLER_YAML_PATH=$3
CLIENT_CREDENTIAL_NAME=$4

echo "Checking environment..."
if [ -z "$JIMM_CONTROLLER_NAME" ];
then
    echo "- JIMM controller name NOT SET, setting it to 'jimm-dev'"
    JIMM_CONTROLLER_NAME="jimm-dev"
fi

if [ -z "$CONTROLLER_NAME" ];
then
    echo "- Controller name NOT SET, setting it to 'qa-controller'"
    CONTROLLER_NAME="qa-controller"
fi

if [ -z "$CONTROLLER_YAML_PATH" ];
then
    echo "- Controller YAML path NOT SET, setting it to './qa-controller'"
    CONTROLLER_YAML_PATH="./qa-controller"
fi

if [ -z "$CLIENT_CREDENTIAL_NAME" ];
then
    echo "- Client credential name NOT SET, setting it to 'microk8s'"
    CLIENT_CREDENTIAL_NAME="microk8s"
fi

echo
echo "JIMM controller name is: $JIMM_CONTROLLER_NAME"
echo "Targ controller name is: $CONTROLLER_NAME"
echo "Targ controller path is: $CONTROLLER_YAML_PATH"
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
./jimmctl controller-info "$CONTROLLER_NAME" "$CONTROLLER_YAML_PATH"
if [[ -f "$CONTROLLER_YAML_PATH" ]]; then
    echo "Controller info retrieved."
else
    echo "Controller info couldn't be created, exiting..."
    exit 1
fi
echo
echo "Retrieving controller address"
CONTROLLER_ADDRESS=$(cat "$CONTROLLER_YAML_PATH" | yq '.public-address' |  cut -d ":" -f 1)
echo "Controller address is: $CONTROLLER_ADDRESS" 
echo
echo "Updating $CONTROLLER_YAML_PATH public-address..."
yq -i e '.public-address |= "juju-apiserver:17070"' "$CONTROLLER_YAML_PATH"
echo
echo "Updating containers /etc/hosts..."
docker compose exec -w /etc --no-TTY jimm bash -c "echo '$CONTROLLER_ADDRESS juju-apiserver' >> hosts"
echo
echo "Adding controller from path: $CONTROLLER_YAML_PATH"
./jimmctl add-controller "$CONTROLLER_YAML_PATH"
echo
echo "Updating cloud credentials for: $JIMM_CONTROLLER_NAME, from client credential: $CLIENT_CREDENTIAL_NAME"
juju update-credentials "$CLIENT_CREDENTIAL_NAME" --controller "$JIMM_CONTROLLER_NAME"
