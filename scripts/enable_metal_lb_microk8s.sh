#!/bin/bash
# This script enables MetalLB on MicroK8s with a specified subnet.
subnet="$(ip route get 1 | head -n 1 | awk '{print $7}' | awk -F. '{print $1 "." $2 "." $3 ".240/24"}')"
echo "MetalLB subnet: $subnet"
sudo microk8s enable metallb:"$subnet"
