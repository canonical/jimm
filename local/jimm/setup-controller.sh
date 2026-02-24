#!/bin/bash

# RUN THIS SCRIPT FROM PROJECT ROOT!
# It will bootstrap a Juju controller and configure the necessary config to enable the controller
# to communicate with the docker compose

set -euo pipefail

CLOUDINIT_FILE=${CLOUDINIT_FILE:-"cloudinit.temp.yaml"}
CONTROLLER_NAME="${CONTROLLER_NAME:-qa-lxd}"
SKIP_CONNECT_JIMM="${SKIP_CONNECT_JIMM:-false}"
JWKS_DNS="${JWKS_DNS:-jimm.localhost}"
CLOUDINIT_TEMPLATE=$'cloudinit-userdata: |
  preruncmd:
    - echo "%s    %s" >> /etc/hosts
  ca-certs:
    trusted:
      - |\n%s'

# shellcheck disable=SC2059
# We are using the variable as the printf template
printf "$CLOUDINIT_TEMPLATE" "$(lxc network get lxdbr0 ipv4.address | cut -f1 -d/)" "${JWKS_DNS}" "$(cat local/traefik/certs/ca.crt | sed -e 's/^/        /')" > "${CLOUDINIT_FILE}"
echo "created cloud-init file"

if [ "${SKIP_BOOTSTRAP:-false}" == true ]; then
  echo "skipping controller bootstrap"
  exit 0
fi

BOOTSTRAP_ARGS=(lxd "${CONTROLLER_NAME}" --config "${CLOUDINIT_FILE}")

if [[ "$SKIP_CONNECT_JIMM" != "true" ]]; then
  BOOTSTRAP_ARGS+=(--config "login-token-refresh-url=https://${JWKS_DNS}/.well-known/jwks.json")
else
  echo "Skipping connecting the controller to JIMM"
fi

if [[ -n "${AGENT_VERSION:-}" ]]; then
  BOOTSTRAP_ARGS+=(--agent-version "${AGENT_VERSION}")
fi

echo "Bootstrapping controller"
JUJU_DEV_FEATURE_FLAGS=ssh-jump juju bootstrap "${BOOTSTRAP_ARGS[@]}"
rm "$CLOUDINIT_FILE"
