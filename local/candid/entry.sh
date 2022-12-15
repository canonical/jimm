#!/bin/sh
# This script is responsible for handling the CMD override of Candid in a local environment.

echo "Entrypoint being overriden for local environment."

# Grab curl quickly.
apt install curl -y
/root/candidsrv /etc/candid/config.yaml &

# Pseudo readiness probe such that we can continue local dev setup.
until eval curl --output /dev/null --silent --fail http://localhost:8081/debug/status; do
    printf '.'
    sleep 1
done
echo "Server appears to have started."



wait