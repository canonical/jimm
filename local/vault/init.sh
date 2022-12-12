#!/bin/sh

# See https://developer.hashicorp.com/vault/docs/secrets/databases/postgresql 
# for info on db engine.

# We use dev for custom root token via env only, otherwise rest is prod emulated.
vault server -dev -config=/vault/config/vault.hcl &
sleep 2

# echo "Enabling AppRole auth"
# # Enable auth
# vault auth enable approle
# # Create role
# vault write auth/approle/role/jimm-app \
#     secret_id_ttl=10m \
#     token_num_uses=10 \
#     token_ttl=20m \
#     token_max_ttl=30m \
#     secret_id_num_uses=40

wait