#!/bin/bash

# This script is used to setup a Juju CLI to be authenticated with JIMM without going through login.
# This is particularly useful in headless environments like CI/CD.

set -eux

# Note that we are working around the fact that yq is a snap and doesn't have permission to hidden folders due to snap confinement.
cat ~/.local/share/juju/accounts.yaml | yq '.controllers += {"jimm":{"type": "oauth2-device", "user": "jimm-test@canonical.com", "access-token": strenv(JWT)}}' | cat > temp-accounts.yaml && mv temp-accounts.yaml ~/.local/share/juju/accounts.yaml
cat ~/.local/share/juju/controllers.yaml | yq '.controllers += {"jimm":{"uuid": "3217dbc9-8ea9-4381-9e97-01eab0b3f6bb", "api-endpoints": ["jimm.localhost:443"]}}' | cat > temp-controllers.yaml && mv temp-controllers.yaml ~/.local/share/juju/controllers.yaml
