#!/bin/bash

# RUN THIS SCRIPT FROM PROJECT ROOT!
# It will bootstrap a Juju controller and configure the necessary config to enable the controller
# to communicate with the docker compose

set -ux

CONTROLLER_NAME="${CONTROLLER_NAME:-qa-controller}"

echo "Bootstrapping controller"
juju bootstrap localhost "${CONTROLLER_NAME}" --config allow-model-access=true --config login-token-refresh-url=https://jimm.localhost
CONTROLLER_ID=$(juju show-controller --format json | jq --arg name "${CONTROLLER_NAME}" '.[$name]."controller-machines"."0"."instance-id"' | tr -d '"')
echo "Adding proxy to LXC instance ${CONTROLLER_ID}"
lxc config device add "${CONTROLLER_ID}" myproxy proxy listen=tcp:0.0.0.0:443 connect=tcp:127.0.0.1:443 bind=instance
echo "Pushing local CA"
lxc file push local/traefik/certs/ca.crt "${CONTROLLER_ID}"/usr/local/share/ca-certificates/
lxc exec "${CONTROLLER_ID}" -- update-ca-certificates
echo "Restarting controller"
lxc stop "${CONTROLLER_ID}"
lxc start "${CONTROLLER_ID}"
