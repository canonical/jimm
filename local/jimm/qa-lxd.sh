#!/bin/bash

# QA-lxd
# This script spins up JIMM (from compose) and sets up a LXD controller and a test model
# to QA against.
#
# It handles the removal of all older resources to ensure a fresh QA env.
#
# Please make sure you've run make "make dev-env-setup" for this script to work.


cleanup() {
    echo "Destroying qa-lxd controller if exists..."
    destroy_qa_output=$(juju destroy-controller qa-lxd --force --no-prompt --destroy-all-models 2>&1) || true
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

echo "*** Starting QA environment setup ***"

docker compose --profile dev up -d

juju login jimm.localhost -c jimm-dev

./local/jimm/setup-controller.sh
./local/jimm/add-controller.sh

juju add-model test-lxd

# Add a test charm (this is a basic hello-juju, that requires postgres to become healthy)
# Essentially, a perfect test bed for performing relations etc against.
juju deploy hello-juju
