#!/bin/bash

# This script runs migrate-providing-model.sh with a Juju 4 source controller,
# so the model is migrated into JIMM using version 8 of the MigrationTarget
# facade (SerializedModelV2) rather than version 6 (model description).
#
# The Juju 4 binary used to bootstrap the source controller can be set via
# JUJU4_BOOTSTRAP_BINARY, defaulting to the juju in PATH.
#
# Usage: ./tests/modelmigration/migrate-providing-model-juju4.sh
set -euo pipefail

JUJU4_BOOTSTRAP_BINARY="${JUJU4_BOOTSTRAP_BINARY:-juju}"

juju_version=$("$JUJU4_BOOTSTRAP_BINARY" version)
if [[ "$juju_version" != 4.* ]]; then
    echo "$JUJU4_BOOTSTRAP_BINARY is Juju $juju_version, expected Juju 4. Set JUJU4_BOOTSTRAP_BINARY to a Juju 4 binary."
    exit 1
fi

JUJU_BOOTSTRAP_BINARY="$JUJU4_BOOTSTRAP_BINARY" \
SOURCE_CONTROLLER_NAME="juju4-migration-source" \
PROVIDER_MODEL_NAME="juju4-source-model" \
CONSUMER_MODEL_NAME="juju4-sink-model" \
    "$(dirname "${BASH_SOURCE[0]}")/migrate-providing-model.sh"
