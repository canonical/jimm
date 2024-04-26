#!/bin/sh

# Grab JQ for ease of use.
ARCH=$(arch | sed s/aarch64/arm64/ | sed s/x86_64/amd64/)
wget -O jq https://github.com/jqlang/jq/releases/download/jq-1.7/jq-linux-$ARCH
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
    policies=jimm-app


# We mimic the normal flow just for reference. Ultimately we passed to secret itself.
# This is because our flow looks at a raw unwrapped secret, rather than carefully
# extracting the role id & secret id from the unwrapped token in cubbyhole.
JIMM_ROLE_ID=$(vault read auth/approle/role/jimm-app/role-id | jq -r '.data.role_id')
echo "AppRole created, role ID is: $JIMM_ROLE_ID"
JIMM_SECRET_WRAPPED=$(vault write -wrap-ttl=10h -force auth/approle/role/jimm-app/secret-id | jq -r '.wrap_info.token')
echo "SecretID applied & wrapped in cubbyhole for 10h, token is: $JIMM_SECRET_WRAPPED"

# Enable the KV at the defined policy path
echo "Enabling KV at policy path /jimm-kv"
echo "/jimm-kv accessible by policy jimm-app"
vault secrets enable -path /jimm-kv kv
echo "Creating approle auth file."
VAULT_TOKEN=$JIMM_SECRET_WRAPPED vault unwrap > /vault/approle_tmp.yaml
echo "$JIMM_ROLE_ID" > /vault/roleid.txt

jq ".data.role_id = \"$JIMM_ROLE_ID\"" /vault/approle_tmp.yaml > /vault/approle.json
role_id=$(cat /vault/approle.json | jq -r ".data.role_id")
role_secret_id=$(cat /vault/approle.json | jq -r ".data.secret_id")
echo "VAULT_ROLE_ID=$role_id" > /vault/vault.env
echo "VAULT_ROLE_SECRET_ID=$role_secret_id" >> /vault/vault.env
wait

