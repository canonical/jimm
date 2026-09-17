#!/bin/sh

# Starts the hook-service, migrates its database and seeds IdP groups and
# members used by JIMM integration tests.
#
# To add users or groups, edit GROUP_NAME/GROUP_MEMBERS below and
# recreate the container: docker compose up -d --build hook-service

set -e

DSN="postgres://groups:groups@hook-service-db:5432/groups?sslmode=disable"

# Migrate the database
/usr/bin/hook-service migrate --dsn "$DSN" up

# Seed IdP groups and their members. Groups are created via SQL (the CLI
# has no group-creation command); memberships are added via the hook-service
# CLI so the user ID format matches what its own code expects.
#
# User ID format (matching the hook-service token hook):
#   - humans: email, e.g. jimm-group-user@canonical.com
#   - service accounts: raw client ID, e.g. jimm-group-client
#     (no @serviceaccount suffix)
GROUP_NAME="canonical"
GROUP_MEMBERS="jimm-group-user@canonical.com,jimm-group-client"

# Create the group (idempotent).
GROUP_ID=$(psql -At "$DSN" -c "
  INSERT INTO groups (id, name, tenant_id, description, type)
  VALUES (gen_random_uuid(), '$GROUP_NAME', 'default', 'JIMM local test group', 0)
  ON CONFLICT (name, tenant_id) DO UPDATE SET name = EXCLUDED.name
  RETURNING id
" | grep -E '^[0-9a-f-]{36}$')

# Add members via the hook-service CLI (idempotent upserts).
/usr/bin/hook-service groups add-users "$GROUP_ID" --dsn "$DSN" -u "$GROUP_MEMBERS"

# Start the hook-service
export DSN="$DSN"
export AUTHENTICATION_ENABLED="false"
export AUTHORIZATION_ENABLED="false"
export TRACING_ENABLED="false"
export LOG_LEVEL="debug"
export GRPC_PORT="9090"
/usr/bin/hook-service serve &

# This container is now healthy
touch /tmp/healthy

# Handle exit signals
trap 'kill %1' TERM ; wait
