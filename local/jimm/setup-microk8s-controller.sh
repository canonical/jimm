#!/bin/bash

# Host-access has some issues, TLDR to fix it:
# 1. enable host-access
# 2. ifconfig 172.16.12.223 (get private address)
# 3. append line: 
#   --node-ip=172.16.12.223
#   to /var/snap/microk8s/current/args/kubelet
# 4. sudo snap restart microk8s
juju bootstrap microk8s "qa-microk8s" --config login-token-refresh-url=http://10.0.1.1:17070/.well-known/jwks.json 

