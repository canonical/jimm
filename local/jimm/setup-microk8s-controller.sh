#!/bin/bash

# Host-access has some issues, TLDR to fix it:
# 1. enable host-access
# 2. ifconfig 172.16.12.223 (get private address)
# 3. append line: 
#   --node-ip=172.16.12.223
#   to /var/snap/microk8s/current/args/kubelet
# 4. sudo snap restart microk8s

# Get the private IP address
PRIVATE_IP=$(hostname -I | awk '{print $1}')

juju bootstrap microk8s "qa-microk8s" --config login-token-refresh-url=http://${PRIVATE_IP}:17070/.well-known/jwks.json --debug

