#!/bin/sh

# This script starts the OpenFGA server, migrates the associated database and applies JIMM's auth model.
# It also manually edits the authorization_model_id to a hardcoded value for easier testing.
# Note that this script expects an authorisation_model.fga file to be present. We provide that file
# by mounting the file from the host rather than putting it into the Docker container to avoid duplication.

set -e

# Migrate the database
./openfga migrate --datastore-engine postgres --datastore-uri "$OPENFGA_DATASTORE_URI"
./fga model transform --file ./authorisation_model.fga --output-format json > ./authorisation_model.json

./openfga run &
sleep 3

# Cleanup old auth model from previous starts
psql -Atx "$OPENFGA_DATASTORE_URI" -c "DELETE FROM authorization_model;"
# Adds the auth model and updates its authorisation model id to be the expected hard-coded id such that our local JIMM can utilise it for queries.
wget -q -O - --header 'Content-Type: application/json' --header 'Authorization: Bearer jimm' --post-file authorisation_model.json localhost:8080/stores/01GP1254CHWJC1MNGVB0WDG1T0/authorization-models
psql -Atx "$OPENFGA_DATASTORE_URI" -c "INSERT INTO store (id,name,created_at,updated_at) VALUES ('01GP1254CHWJC1MNGVB0WDG1T0','jimm',NOW(),NOW()) ON CONFLICT DO NOTHING;"

# Openfga generates a random authorization_model_id when we add the model.
# However, JIMM expects a specific hardcoded authorization_model_id to function correctly.
# So, we replace the generated ID with our hardcoded ID in both the column and the serialized protobuf
# within the database. 
psql -Atx "$OPENFGA_DATASTORE_URI" <<-SQL
  UPDATE authorization_model 
  SET 
    authorization_model_id = '01GP1EC038KHGB6JJ2XXXXCXKB',
    serialized_protobuf = decode(
      replace(
        encode(authorization_model.serialized_protobuf, 'escape'), 
        authorization_model.authorization_model_id, 
        '01GP1EC038KHGB6JJ2XXXXCXKB'
      ), 
      'escape'
    )
  WHERE store = '01GP1254CHWJC1MNGVB0WDG1T0';
SQL

# This container is now healthy
touch /tmp/healthy

# Handle exit signals
trap 'kill %1' TERM ; wait
