#!/bin/bash

# This script runs migrate-internal.sh with a Juju 4 controller as the source
# of the internal migration. JIMM only lists migration targets at the same or a
# newer Juju version, so the model is migrated to another Juju 4 controller.
#
# The Juju 4 binary used to bootstrap the source controller can be set via
# JUJU4_BOOTSTRAP_BINARY, defaulting to the juju in PATH.
#
# Usage: ./tests/modelmigration/migrate-internal-juju4.sh
set -euo pipefail

JUJU4_BOOTSTRAP_BINARY="${JUJU4_BOOTSTRAP_BINARY:-juju}"

juju_version=$("$JUJU4_BOOTSTRAP_BINARY" version)
if [[ "$juju_version" != 4.* ]]; then
    echo "$JUJU4_BOOTSTRAP_BINARY is Juju $juju_version, expected Juju 4. Set JUJU4_BOOTSTRAP_BINARY to a Juju 4 binary."
    exit 1
fi

JUJU_BOOTSTRAP_BINARY="$JUJU4_BOOTSTRAP_BINARY" \
INTERNAL_MIGRATION_CONTROLLER="juju4-internal-migration-controller" \
    "$(dirname "${BASH_SOURCE[0]}")/migrate-internal.sh"
