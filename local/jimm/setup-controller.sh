#!/bin/bash

# RUN THIS SCRIPT FROM PROJECT ROOT!
# It will bootstrap a Juju controller and configure the necessary config to enable the controller
# to communicate with the docker compose

CLOUDINIT_FILE=${CLOUDINIT_FILE:-"cloudinit.temp.yaml"}
CONTROLLER_NAME="${CONTROLLER_NAME:-qa-lxd}"
CLOUDINIT_TEMPLATE=$'cloudinit-userdata: |
  preruncmd:
    - echo "%s    jimm.localhost" >> /etc/hosts
  ca-certs:
    trusted:
      - |\n%s'

# shellcheck disable=SC2059
# We are using the variable as the printf template
printf "$CLOUDINIT_TEMPLATE" "$(lxc network get lxdbr0 ipv4.address | cut -f1 -d/)" "$(cat local/traefik/certs/ca.crt | sed -e 's/^/        /')" > "${CLOUDINIT_FILE}"
echo "created cloud-init file"

if [ "${SKIP_BOOTSTRAP:-false}" == true ]; then
  echo "skipping controller bootstrap"
  echo "skipping controller bootstrap"
  exit 0
fi

echo "Bootstrapping controller"
juju bootstrap lxd "${CONTROLLER_NAME}" --config "${CLOUDINIT_FILE}" --config login-token-refresh-url=https://jimm.localhost/.well-known/jwks.json
rm "$CLOUDINIT_FILE"
