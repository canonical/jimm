#!/usr/bin/env bash

# Create a user in Keycloak.
#
# Usage:
#
#   create-user.sh [<username> [<password> [<email>]]]
#

username="${1:-someone}"
password="${2:-jimm}"
email="${3:-"${username}@canonical.com"}"

access_token=$(curl -k \
    -X POST \
    http://localhost:8082/realms/master/protocol/openid-connect/token \
    --user admin-cli:DOLcuE5Cd7IxuR7JE4hpAUxaLF7RlAWh \
    -H 'content-type: application/x-www-form-urlencoded' \
    -d "username=jimm&password=jimm&grant_type=password" \
    2>/dev/null \
    | jq --raw-output '.access_token')

echo "Access token for admin-cli client:"
echo "$access_token"

curl -k \
    -X POST \
    http://localhost:8082/admin/realms/jimm/users \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $access_token" \
    --data "{ \"username\": \"$username\", \"email\": \"$email\", \"emailVerified\":true, \"enabled\": true, \"realmRoles\": [ \"user\", \"offline_access\" ] }" \
    2>/dev/null

user_id="$(curl -k \
    -X GET \
    http://localhost:8082/admin/realms/jimm/users \
    -H "Authorization: Bearer $access_token" \
    2>/dev/null \
    | jq --raw-output ".[] | select(.username==\"$username\") | .id")"

curl -k \
    -X PUT \
    http://localhost:8082/admin/realms/jimm/users/$user_id/reset-password \
    -H "Content-Type: application/json" \
    -H "Authorization: bearer $access_token" \
    --data "{ \"type\": \"password\", \"temporary\": false, \"value\": \"$password\" }" \
    2>/dev/null

echo
echo "Created user:"
echo "ID:       $user_id"
echo "Email:    $email"
echo "Username: $username"
echo "Password: $password"
