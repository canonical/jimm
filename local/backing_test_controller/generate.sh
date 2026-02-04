#!/bin/bash
set -e

# generate.sh
# This script extracts the current Juju controller's configuration
# and writes them to a controller.yaml file. It retrieves the controller's UUID,
# API endpoints, username, password, and CA certificate.
#
# This enables the developer to quickly set up configuration for
# running E2E tests that require a backing Juju controller.
#
# Usage:
#   ./generate.sh [--controller=<name>] [--controllers=<name1,name2,...>]
#
# If --controller is not provided, it uses the current active controller.
# If --controllers is provided, it generates config for those comma-separated controllers.

CONTROLLER=""
CONTROLLERS=""
for arg in "$@"; do
    case $arg in
        --controller=*)
            CONTROLLER="${arg#*=}"
            shift
            ;;
        --controllers=*)
            CONTROLLERS="${arg#*=}"
            shift
            ;;
        *)
            ;;
    esac
done

# Output path (project root - script should be run from there)
CONFIG_FILE="controllers.yaml"

# Determine which controllers to process
if [ -n "$CONTROLLERS" ]; then
    # Use comma-separated list
    IFS=',' read -ra CONTROLLER_ARRAY <<< "$CONTROLLERS"
else
    # Use single controller (current or specified)
    if [ -z "$CONTROLLER" ]; then
        CONTROLLER=$(juju whoami | yq -r '.Controller')
    fi
    CONTROLLER_ARRAY=("$CONTROLLER")
fi

echo "Extracting controller configuration for ${#CONTROLLER_ARRAY[@]} controller(s)" >&2

# Initialize empty YAML
yq -n '.controllers = {}' | tee "$CONFIG_FILE" > /dev/null

# Loop through and generate configurations for each controller
for CURRENT_CONTROLLER in "${CONTROLLER_ARRAY[@]}"; do
    echo "Processing controller: ${CURRENT_CONTROLLER}" >&2
    
    # Get controller UUID
    CONTROLLER_UUID=$(juju show-controller "${CURRENT_CONTROLLER}" | yq -r ".${CURRENT_CONTROLLER}.details.controller-uuid")

    # Get all controller API addresses as a JSON array
    CONTROLLER_ADDRS=$(juju show-controller "${CURRENT_CONTROLLER}" | yq -r ".${CURRENT_CONTROLLER}.details.\"api-endpoints\" | @json")

    # Get username and password from accounts.yaml
    CONTROLLER_USERNAME=$(cat ~/.local/share/juju/accounts.yaml | yq -r ".controllers.${CURRENT_CONTROLLER}.user")
    CONTROLLER_PASSWORD=$(cat ~/.local/share/juju/accounts.yaml | yq -r ".controllers.${CURRENT_CONTROLLER}.password")

    # Get CA certificate
    CONTROLLER_CACERT=$(juju show-controller "${CURRENT_CONTROLLER}" | yq -r ".${CURRENT_CONTROLLER}.details.\"ca-cert\"")

    # Build the YAML using yq to ensure proper formatting
    export CURRENT_CONTROLLER CONTROLLER_UUID CONTROLLER_ADDRS CONTROLLER_USERNAME CONTROLLER_PASSWORD CONTROLLER_CACERT
    yq -i '
      .controllers.[strenv(CURRENT_CONTROLLER)].uuid = strenv(CONTROLLER_UUID) |
      .controllers.[strenv(CURRENT_CONTROLLER)].addrs = (strenv(CONTROLLER_ADDRS) | fromjson) |
      .controllers.[strenv(CURRENT_CONTROLLER)].username = strenv(CONTROLLER_USERNAME) |
      .controllers.[strenv(CURRENT_CONTROLLER)].password = strenv(CONTROLLER_PASSWORD) |
      .controllers.[strenv(CURRENT_CONTROLLER)]."ca-cert" = strenv(CONTROLLER_CACERT)
    ' "$CONFIG_FILE" > /dev/null
done

echo "Controller configuration written to ${CONFIG_FILE}" >&2
