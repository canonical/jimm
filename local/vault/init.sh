#!/bin/sh

# Grab JQ for ease of use.
wget -O jq https://github.com/stedolan/jq/releases/download/jq-1.5/jq-linux64
chmod +x ./jq
cp jq /usr/bin

# Dev mode defaults some addresses, but also enables us
# to have a custom root key & automatically unsealed vault.
vault server -dev -config=/vault/config/vault.hcl &
sleep 2

# Login.
echo "token" | vault login -

# Set address for local client.
export VAULT_ADDR="http://localhost:8200"
# Makes reading easier.
export VAULT_FORMAT=json


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
vault write auth/approle/role/jimm-app \
    secret_id_ttl=10m \
    token_num_uses=10 \
    token_ttl=20m \
    token_max_ttl=30m \
    secret_id_num_uses=40 \
    policies=jimm-app


JIMM_ROLE_ID=$(vault read auth/approle/role/jimm-app/role-id | jq -r '.data.role_id')
echo "AppRole created, role ID is: $JIMM_ROLE_ID"
JIMM_SECRET_ID=$(vault write -force  auth/approle/role/jimm-app/secret-id | jq -r '.data.secret_id')
echo "SecretID applied, secret ID is: $JIMM_SECRET_ID"
# Enable the KV at the defined policy path
echo "Enabling KV at policy path /jimm-kv"
echo "/jimm-kv accessible by policy jimm-app"
vault secrets enable -path /jimm-kv kv
echo "Creating approle auth file."
tee /vault/approle.yaml << END
vault:
    address: http://vault:8200
    approle-path: /auth/approle
    approle-role-id: $JIMM_ROLE_ID
    approle-secret-id: $JIMM_SECRET_ID
    kv-path: /jimm-kv/
END

wait
