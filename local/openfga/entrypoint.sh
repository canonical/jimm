#!/bin/sh

# This script starts the OpenFGA server, migrates the associated database and applies JIMM's auth model.
# It also manually edits the authorization_model_id to a hardcoded value for easier testing.
# Note that this script expects an authorisation_model.json file to be present. We provide that file
# by mounting the file from the host rather than putting it into the Docker container to avoid duplication.

set -e

# Migrate the database
./openfga migrate --datastore-engine postgres --datastore-uri "$OPENFGA_DATASTORE_URI"

./openfga run &
sleep 3

# Cleanup old auth model from previous starts
psql -Atx "$OPENFGA_DATASTORE_URI" -c "DELETE FROM authorization_model;"
# Adds the auth model and updates its authorisation model id to be the expected hard-coded id such that our local JIMM can utilise it for queries.
wget -q -O - --header 'Content-Type: application/json' --header 'Authorization: Bearer jimm' --post-file authorisation_model.json localhost:8080/stores/01GP1254CHWJC1MNGVB0WDG1T0/authorization-models
psql -Atx "$OPENFGA_DATASTORE_URI" -c "INSERT INTO store (id,name,created_at,updated_at) VALUES ('01GP1254CHWJC1MNGVB0WDG1T0','jimm',NOW(),NOW()) ON CONFLICT DO NOTHING;"
psql -Atx "$OPENFGA_DATASTORE_URI" -c "UPDATE authorization_model SET authorization_model_id = '01GP1EC038KHGB6JJ2XXXXCXKB' WHERE store = '01GP1254CHWJC1MNGVB0WDG1T0';"

# Keep container alive
tail -f /dev/null & trap 'kill %1' TERM ; wait
