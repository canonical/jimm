#!/bin/bash

# RUN THIS SCRIPT FROM PROJECT ROOT!
# It will bootstrap a Juju controller and configure the necessary config to enable the controller
# to communicate with the docker compose

set -ux

CLOUDINIT_FILE="cloudinit.temp.yaml"
function finish {
    rm "$CLOUDINIT_FILE"
}
trap finish EXIT

CONTROLLER_NAME="${CONTROLLER_NAME:-qa-controller}"
CLOUDINIT_TEMPLATE=$'cloudinit-userdata: |
  preruncmd:
    - echo "%s    jimm.localhost" >> /etc/hosts
  ca-certs:
    trusted:
      - |\n%s'

printf "$CLOUDINIT_TEMPLATE" "$(lxc network get lxdbr0 ipv4.address | cut -f1 -d/)" "$(cat local/traefik/certs/ca.crt | sed -e 's/^/        /')" > "${CLOUDINIT_FILE}"

echo "Bootstrapping controller"
juju bootstrap localhost "${CONTROLLER_NAME}" --config allow-model-access=true --config "${CLOUDINIT_FILE}" --config identity-url=https://candid.localhost
CONTROLLER_ID=$(juju show-controller --format json | jq --arg name "${CONTROLLER_NAME}" '.[$name]."controller-machines"."0"."instance-id"' | tr -d '"')
echo "Adding proxy to LXC instance ${CONTROLLER_ID}"
lxc config device add "${CONTROLLER_ID}" myproxy proxy listen=tcp:0.0.0.0:443 connect=tcp:127.0.0.1:443 bind=instance
