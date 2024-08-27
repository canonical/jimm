#!/bin/sh

# Much of the below was lifted from the sample Vault application setup
# in https://github.com/hashicorp/hello-vault-go/tree/main/sample-app

set -e

export VAULT_ADDR='http://localhost:8200'
export VAULT_FORMAT='json'

# Dev mode defaults some addresses, but also enables us
# to have a custom root key & automatically unsealed vault.
vault server -dev &
sleep 5s

# Authenticate container's local Vault CLI
# ref: https://www.vaultproject.io/docs/commands/login
vault login -no-print "${VAULT_DEV_ROOT_TOKEN_ID}"

# AppRole auth is what we use in JIMM, an awesome tutorial
# on how this is setup can be found below.
# HOW-TO: https://developer.hashicorp.com/vault/docs/auth/approle
# AND:
# https://developer.hashicorp.com/vault/tutorials/auth-methods/approle

echo "Enabling AppRole auth"
vault auth enable approle

echo "Creating access policy to JIMM stores"
vault policy write jimm-app /vault/policy.hcl

echo "Creating jimm-app AppRole"
vault write auth/approle/role/jimm-app policies=jimm-app

# Set fixed role ID and secret ID to simplify testing
vault write auth/approle/role/jimm-app/role-id role_id="test-role-id"
vault write auth/approle/role/jimm-app/custom-secret-id secret_id="test-secret-id"

# Enable the KV at the defined policy path
echo "Enabling KV at policy path /jimm-kv"
echo "/jimm-kv accessible by policy jimm-app"
vault secrets enable -version=2 -path /jimm-kv kv

# This container is now healthy
touch /tmp/healthy

# Handle exit signals
trap 'kill %1' TERM ; wait
