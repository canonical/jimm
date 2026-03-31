#!/bin/bash

# This script creates a model and deploys a basic application to it
# before running `juju status` and then destroys the model.

set -euo pipefail
source "local/jimm/detect-jaas.sh"

JIMM_CONTROLLER_NAME="${JIMM_CONTROLLER_NAME:-jimm-dev}"

echo 
echo "Adding microk8s k8s to juju"
sudo microk8s config | juju add-k8s testk8s --cluster-name=microk8s-cluster --client

echo
echo "Bootstrapping controller on microk8s"
$JAAS bootstrap testk8s test-controller 3.6.19 --config controller-service-type=loadbalancer

CERT=$(sudo microk8s config | yq '.users[0].user."client-certificate-data"')
KEY=$(sudo microk8s config | yq '.users[0].user."client-key-data"' )

cat > credentials.yaml <<EOF
credentials:
  testk8s:
    testk8s:
      auth-type: clientcertificate
      ClientCertificateData: ${CERT}
      ClientKeyData: ${KEY}
EOF

echo
echo "Adding credential and creating model on the new controller"
# We no op this because despite adding the credential it is exit 0 due to it already existing locally.
juju add-credential testk8s --controller "$JIMM_CONTROLLER_NAME" -f ./credentials.yaml || :
juju add-model test-model testk8s

echo
echo "Destroying model"
juju destroy-model test-model --no-prompt

echo
echo "Destroying controller"
$JAAS destroy-controller test-controller --no-prompt
