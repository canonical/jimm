#!/bin/bash

# This script registers a Juju controller in Microk8s to JIMM.
# It re-uses the logic from add-controller but does some microk8s 
# specific setup first.
#
# JIMM needs to contact the controller and cannot do so from the docker compose to microk8s easily.
# To expose the Juju api-server, we turn the controller's default service into a node port service.
# This allows the service to be accessed on the host's network at port 30040.
#
# Next, we have TLS issues as the controller only has limited SANs, one of them being "juju-apiserver"
# this is handled by registering the controller with the --tls-hostname flag.
#
# Finally, for routing, we use docker's host network interface address (172.17.0.1), enabling access to the host network.
#
# For routing explanation:
# JIMM -> jujuapi-server -> 172.17.0.1 -> localhost (of the host) -> localhost:30040 -> NodePort -> Cluster -> Controller

# Exit immediately if a command exits with a non-zero status.
set -e  

# Patch the controller such that it is reachable on the host at 30040
microk8s.kubectl patch -n controller-qa-microk8s svc/controller-service --type='json' -p '[{"op":"replace","path":"/spec/type","value":"NodePort"},{"op":"replace","path":"/spec/ports/0/nodePort","value":30040}]'

CONTROLLER_NAME="${CONTROLLER_NAME:-qa-microk8s}"
CLIENT_CREDENTIAL_NAME="${CLIENT_CREDENTIAL_NAME:-microk8s}"
CUSTOM_CONTROLLER_HOSTNAME="${CUSTOM_CONTROLLER_HOSTNAME:-172.17.0.1:30040}"
"$(dirname "${BASH_SOURCE[0]}")/add-controller.sh"
