#!/bin/sh
# This script is responsible for handling the CMD override of Candid in a local environment.

echo "Entrypoint being overriden for local environment."

apt update
apt install curl -y
exec /root/candidsrv /etc/candid/config.yaml
