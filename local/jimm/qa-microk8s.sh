#!/bin/bash

# QA-microk8s
# This script spins up JIMM (from compose) and sets up a K8S controller and a test model
# to QA against.
#
# It handles the removal of all older resources to ensure a fresh QA env.
#
# Please make sure you've run make "make dev-env-setup" for this script to work.

cleanup() {
    echo "Destroying qa-microk8s controller if exists..."
    destroy_qa_output=$(juju destroy-controller qa-microk8s --force --no-prompt --destroy-all-models 2>&1) || true
    if [ $? -ne 0 ]; then
        echo "$destroy_qa_output"
    fi

    echo "Unregistering jimm-dev controller if exists..."
    unregister_jimm_output=$(juju unregister jimm-dev --no-prompt 2>&1) || true
    if [ $? -ne 0 ]; then
        echo "$unregister_jimm_output"
    fi

    echo "Tearing down compose..."
    compose_teardown_output=$(docker compose --profile dev down 2>&1) || true
    if [ $? -ne 0 ]; then
        echo "$compose_teardown_output"
    fi
}

cleanup

docker compose --profile dev up -d

juju login jimm.localhost -c jimm-dev

./local/jimm/setup-microk8s-controller.sh
./local/jimm/add-microk8s-controller.sh

# Add a test model
juju add-model test microk8s

