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
juju bootstrap localhost "${CONTROLLER_NAME}" --config allow-model-access=true --config "${CLOUDINIT_FILE}" --config login-token-refresh-url=https://jimm.localhost/.well-known/jwks.json
