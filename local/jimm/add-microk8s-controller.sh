#!/bin/bash


# Explanation:
# JIMM needs to contact the controller and cannot do so from the docker compose to microk8s easily.
# As such, we turn the controllers default service into a node port service.
# This allows the service to be access on the hosts network at 30040.

# Next, we have TLS issues as the controller only has limited SANs, one of them being "juju-apiserver"
# As such, we update jimm's container to map juju-apiserver to "172.17.0.1". This IP address is dockers
# host network interface address, enabling access to the localhost of the host.

# Finally, we update jimmctls info output attempt to contact the controller on "juju-apiserver"
# and due to the SAN matching, having a nodeport available and using dockers host network interface,
# we can contact.

# For routing explanation:
# JIMM -> jujuapi-server -> 172.17.0.1 -> localhost (of the host) -> localhost:30040 -> NodePort -> Cluster -> Controller

JIMM_CONTROLLER_NAME="${JIMM_CONTROLLER_NAME:-jimm-dev}"
CONTROLLER_NAME="${CONTROLLER_NAME:-qa-microk8s}"
CONTROLLER_YAML_PATH="${CONTROLLER_NAME}".yaml
CLIENT_CREDENTIAL_NAME="${CLIENT_CREDENTIAL_NAME:-localhost}"
KUBECTL="${KUBECTL:-microk8s.kubectl}"
YQ="${YQ:-yq}"
JIMMCTL="jimmctl"

echo
echo "JIMM controller name is: $JIMM_CONTROLLER_NAME"
echo "Target controller name is: $CONTROLLER_NAME"
echo "Target controller path is: $CONTROLLER_YAML_PATH"
echo
which jimmctl
jimmctlAvailable=$?
if [ $jimmctlAvailable -ne 0 ] && [ ! -f ./jimmctl ]; then
    echo "Building jimmctl..."
    # Build jimmctl so we may add a controller.
    go build ./cmd/jimmctl
    echo "Built jimmctl."
    echo 
else
    echo "jimmctl available, skipping build"
fi
if [ -f ./jimmctl ]; then
    JIMMCTL="./jimmctl"
fi
if which jimmctl | grep -q 'snap'; then
    CONTROLLER_YAML_PATH="$HOME/snap/jimmctl/common/$CONTROLLER_YAML_PATH"
    echo "jimmctl is installed as a snap"
    echo "placing controller info file at $CONTROLLER_YAML_PATH"
fi

# Patch the controller such that it is reachable on the host at 30040
$KUBECTL patch -n controller-"$CONTROLLER_NAME" svc/controller-service --type='json' -p '[{"op":"replace","path":"/spec/type","value":"NodePort"},{"op":"replace","path":"/spec/ports/0/nodePort","value":30040}]'

# 172.17.0.1 is dockers host interface, enabling access the host machines host network
# despite being in a strictly confined docker compose network.
docker compose exec jimm-dev bash -c "echo '172.17.0.1 juju-apiserver' >> /etc/hosts"

$JIMMCTL controller-info --local "$CONTROLLER_NAME" "$CONTROLLER_YAML_PATH"

# Update api & public addresses to match /etc/hosts of jimm container
$YQ e -i '.api-addresses = ["juju-apiserver:30040"]' "$CONTROLLER_YAML_PATH"
$YQ e -i '.public-address = "juju-apiserver:30040"' "$CONTROLLER_YAML_PATH"

# Finally add the controller to jimm and add the microk8s credential
echo "Switching juju controller to $JIMM_CONTROLLER_NAME" 
juju switch "$JIMM_CONTROLLER_NAME"
$JIMMCTL add-controller "$CONTROLLER_YAML_PATH"

juju update-credentials microk8s --controller "$JIMM_CONTROLLER_NAME"

