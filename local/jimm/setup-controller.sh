#!/bin/bash
set -ux
echo "Bootstrapping controller"
juju bootstrap localhost qa-controller --config allow-model-access=true --config identity-url=https://candid.localhost
CONTROLLER=$(juju show-controller --format json | jq '."qa-controller"."controller-machines"."0"."instance-id"' | tr -d '"')
echo "Adding proxy to LXC instance"
lxc config device add "${CONTROLLER}" myproxy proxy listen=tcp:0.0.0.0:443 connect=tcp:127.0.0.1:443 bind=instance
echo "Pushing local CA"
lxc file push local/traefik/certs/ca.crt "${CONTROLLER}"/usr/local/share/ca-certificates/
lxc exec "${CONTROLLER}" -- update-ca-certificates
lxc exec "${CONTROLLER}" -- echo "127.0.0.1 candid.localhost" >> /etc/hosts
echo "Restarting controller"
lxc stop "${CONTROLLER}"
lxc start "${CONTROLLER}"
