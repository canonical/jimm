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

go build ./cmd/jimmctl

# Patch the controller such that it is reachable on the host at 30040
microk8s.kubectl patch -n controller-qa-microk8s svc/controller-service --type='json' -p '[{"op":"replace","path":"/spec/type","value":"NodePort"},{"op":"replace","path":"/spec/ports/0/nodePort","value":30040}]'

# 172.17.0.1 is dockers host interface, enabling access the host machines host network
# despite being in a strictly confined docker compose network.
docker compose exec jimm bash -c "echo '172.17.0.1 juju-apiserver' >> /etc/hosts"

./jimmctl controller-info --local qa-microk8s ./qa-microk8s-controller.yaml

# Update api & public addresses to match /etc/hosts of jimm container
yq e -i '.api-addresses = ["juju-apiserver:30040"]' ./qa-microk8s-controller.yaml
yq e -i '.public-address = "juju-apiserver:30040"' ./qa-microk8s-controller.yaml

# Finally add the controller to jimm and add the microk8s credential
juju switch jimm-dev
./jimmctl add-controller ./qa-microk8s-controller.yaml

juju update-credentials microk8s --controller jimm-dev

