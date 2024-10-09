#!/usr/bin/env bash
# Copyright (C) 2024 Canonical Ltd.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU Affero General Public License as
# published by the Free Software Foundation, either version 3 of the
# License, or (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU Affero General Public License for more details.
#
# You should have received a copy of the GNU Affero General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

set -e

_usage='Exercise running server

Options:
  --help           displays current doc.
  --reset          resets the server state at the beginning; default is false.
  --cleanup        cleans up created entities/relationships at the end; default is false.
  --bail-on-error  exits when a request fails (status code >= 400); default is false.
'

_host="localhost:17070"
_base="$_host/rebac/v1"

generate_random_string() {
    local length=$1
    tr -dc 'a-zA-Z0-9' < /dev/urandom | fold -w "$length" | head -n 1
}
num_strings=5

# Length of each random string
string_length=8

# Generate array with random unique strings
_names=()
while [[ ${#_names[@]} -lt $num_strings ]]; do
    new_string=$(generate_random_string "$string_length")
    if [[ ! " ${_names[*]} " =~ " ${new_string} " ]]; then
        _names+=("$new_string")
    fi
done

# _names=(alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike november oscar papa quebec romeo sierra tango uniform victor whiskey x yankee zulu)

_reset_at_start=""
_cleanup_at_end=""
_bail_on_error=""
_help=""
for option in "$@" ; do
    if [[ "$option" == "--reset" ]] ; then
        _reset_at_start="true"
    fi
    if [[ "$option" == "--cleanup" ]] ; then
        _cleanup_at_end="true"
    fi
    if [[ "$option" == "--bail-on-error" ]] ; then
        _bail_on_error="true"
    fi
    if [[ "$option" == "--help" || "$option" == "-h" ]] ; then
        _help="true"
        break
    fi
done

if [ "$_help" == "true" ]; then
    echo "$_usage"
    exit 0
fi

## Check if the server is running
if ! curl -s "$_host/health"; then
    function onexit {
        curl "$_host/shutdown"
        wait $_PID1
        echo "Server shut down"
    }
    trap onexit EXIT

    ## Run server in background
    go build -o server ./cmd
    bash -c 'sleep 1 && ./server' &
    _PID1=$!

    echo waiting for the server to be ready
    while ! curl -s "$_host/ready"
    do
        sleep 0.1
    done
fi

_opts='-w "\n"'
if [ "$_bail_on_error" == "true" ]; then
    _opts="--fail-with-body $_opts"
fi

## Reset state

if [ "$_reset_at_start" == "true" ]; then
    echo -n 'reset state'
    curl $_opts -X GET "$_host/reset"
fi

## Add single entities
_ids=()
for n in ${_names[@]}
do
    echo -n 'POST group: '
    content=$(curl -s -X POST "$_base/groups" -d "{\"name\":\"group-$n\"}")
    id=$( jq -r  '.id' <<< "${content}" ) 
    _ids+=($id)
    echo -n '  > GET group: '
    curl $_opts -X GET "$_base/groups/$id"
    echo -n '  > PUT group: '
    curl $_opts -X PUT "$_base/groups/$id" -d "{\"id\":\"$id\",\"name\":\"group-$n--updated\"}"
    echo -n '  > GET updated group: '
    curl $_opts -X GET "$_base/groups/$id"
done

identity='admin@canonical.com'
echo -n '  > GET identity: '
curl $_opts -X GET "$_base/identities/$identity"

# for n in ${_names[@]}
# do
#     echo -n 'POST role: '
#     curl $_opts -X POST "$_base/roles" -d "{\"name\":\"role-$n\"}"
#     echo -n '  > GET role: '
#     curl $_opts -X GET "$_base/roles/role-$n"
#     echo -n '  > PUT role: '
#     curl $_opts -X PUT "$_base/roles/role-$n" -d "{\"id\":\"role-$n\",\"name\":\"role-$n--updated\"}"
#     echo -n '  > GET updated role: '
#     curl $_opts -X GET "$_base/roles/role-$n"
# done


# for n in ${_names[@]}
# do
#     echo -n 'POST idp: '
#     curl $_opts -X POST "$_base/authentication" -d "{\"name\":\"idp-$n\"}"
#     echo -n '  > GET idp: '
#     curl $_opts -X GET "$_base/authentication/idp-$n"
#     echo -n '  > PUT idp: '
#     curl $_opts -X PUT "$_base/authentication/idp-$n" -d "{\"id\":\"idp-$n\",\"name\":\"idp-$n--updated\"}"
#     echo -n '  > GET updated idp: '
#     curl $_opts -X GET "$_base/authentication/idp-$n"
# done

## Add relationships

for n in ${_ids[@]}
do
    echo -n "PATCH group identities: group-$n"
    curl $_opts -X PATCH "$_base/groups/$n/identities" -d "{\"patches\":[{\"op\":\"add\",\"identity\":\"$identity\"}]}"
    echo -n '  > GET group identities: '
    curl $_opts -X GET "$_base/groups/$n/identities"
done

# for n in ${_names[@]}
# do
#     echo -n "PATCH group roles: group-$n"
#     curl $_opts -X PATCH "$_base/groups/group-$n/roles" -d "{\"patches\":[{\"op\":\"add\",\"role\":\"role-$n\"}]}"
#     echo -n '  > GET group roles: '
#     curl $_opts -X GET "$_base/groups/group-$n/roles"
# done

for n in ${_ids[@]}
do
    echo -n "PATCH group entitlements: group-$n"
    curl $_opts -X PATCH "$_base/groups/$n/entitlements" -d "{\"patches\":[{\"op\":\"add\",\"entitlement\":{\"entitlement\":\"administrator\",\"entity_id\":\"2d105c41-4531-4103-8e94-3b14118ca03d\",\"entity_type\":\"controller\"}}]}"
    echo -n '  > GET group entitlements: '
    curl $_opts -X GET "$_base/groups/$n/entitlements"
done

group_test=$(generate_random_string "$string_length")
content=$( curl -s -X POST "$_base/groups" -d "{\"name\":\"group-$group_test\"}")
id=$( jq -r  '.id' <<< "${content}" )
echo -n "PATCH identity groups: $identity"
curl $_opts -X PATCH "$_base/identities/$identity/groups" -d "{\"patches\":[{\"op\":\"add\",\"group\":\"group-$id\"}]}"
echo -n '  > GET identity groups: '
curl $_opts -X GET "$_base/identities/$identity/groups"

# for n in ${_names[@]}
# do
#     echo -n "PATCH identity roles: $n@host.com"
#     curl $_opts -X PATCH "$_base/identities/$n@host.com/roles" -d "{\"patches\":[{\"op\":\"add\",\"role\":\"role-$n\"}]}"
#     echo -n '  > GET identity roles: '
#     curl $_opts -X GET "$_base/identities/$n@host.com/roles"
# done

echo -n "PATCH identity entitlements: $identity"
curl $_opts -X PATCH "$_base/identities/$identity/entitlements" -d "{\"patches\":[{\"op\":\"add\",\"entitlement\":{\"entitlement\":\"administrator\",\"entity_id\":\"2d105c41-4531-4103-8e94-3b14118ca03d\",\"entity_type\":\"controller\"}}]}"
echo -n '  > GET identity entitlements: '
curl $_opts -X GET "$_base/identities/$identity/entitlements"

# for n in ${_names[@]}
# do
#     echo -n "PATCH role entitlements: role-$n"
#     curl $_opts -X PATCH "$_base/roles/role-$n/entitlements" -d "{\"patches\":[{\"op\":\"add\",\"entitlement\":{\"entitlement_type\":\"entitlement-type-$n\",\"entity_name\":\"entity-$n\",\"entity_type\":\"entity-type-$n\"}}]}"
#     echo -n '  > GET role entitlements: '
#     curl $_opts -X GET "$_base/roles/role-$n/entitlements"
# done

## Invoke other endpoints

echo -n 'GET resources: '
curl $_opts -X GET "$_base/resources"
echo -n 'GET resources: '
curl $_opts -X GET "$_base/resources?entityType=model"
echo -n 'GET entitlements: '
curl $_opts -X GET "$_base/entitlements"
echo -n 'GET entitlements/raw: '
curl $_opts -X GET "$_base/entitlements/raw"
echo -n 'GET swagger.json (first few chars): '
curl $_opts -X GET "$_base/swagger.json" 2>/dev/null | cut -c1-40
echo -n 'GET capabilities'
curl $_opts -X GET "$_base/capabilities"

if [[ "$_cleanup_at_end" != "true" ]]; then
    exit 0
fi

## Clean up by calling DELETE/PATCH endpoints.

echo -n "PATCH group identities: group-$n"
curl $_opts -X PATCH "$_base/groups/$n/identities" -d "{\"patches\":[{\"op\":\"remove\",\"identity\":\"$identity\"}]}"
echo -n '  > GET group identities: '
curl $_opts -X GET "$_base/groups/$n/identities"


# for n in ${_names[@]}
# do
#     echo -n "PATCH group roles: group-$n"
#     curl $_opts -X PATCH "$_base/groups/group-$n/roles" -d "{\"patches\":[{\"op\":\"remove\",\"role\":\"role-$n\"}]}"
#     echo -n '  > GET group roles: '
#     curl $_opts -X GET "$_base/groups/group-$n/roles"
# done

for n in ${_ids[@]}
do
    echo -n "PATCH group entitlements: group-$n"
    curl $_opts -X PATCH "$_base/groups/$n/entitlements" -d "{\"patches\":[{\"op\":\"remove\",\"entitlement\":{\"entitlement\":\"administrator\",\"entity_id\":\"2d105c41-4531-4103-8e94-3b14118ca03d\",\"entity_type\":\"controller\"}}]}"
    echo -n '  > GET group entitlements: '
    curl $_opts -X GET "$_base/groups/$n/entitlements"
done

content=$( curl -s -X GET "$_base/groups/$id")
id=$( jq -r  '.id' <<< "${content}" ) 
echo -n "PATCH identity groups: $identity"
curl $_opts -X PATCH "$_base/identities/$identity/groups" -d "{\"patches\":[{\"op\":\"remove\",\"group\":\"group-$id\"}]}"
echo -n '  > GET identity groups: '
curl $_opts -X GET "$_base/identities/$identity/groups"

# for n in ${_names[@]}
# do
#     echo -n "PATCH identity roles: $n@host.com"
#     curl $_opts -X PATCH "$_base/identities/$n@host.com/roles" -d "{\"patches\":[{\"op\":\"remove\",\"role\":\"role-$n\"}]}"
#     echo -n '  > GET identity roles: '
#     curl $_opts -X GET "$_base/identities/$n@host.com/roles"
# done

echo -n "PATCH identity entitlements: $identity"
curl $_opts -X PATCH "$_base/identities/$identity/entitlements" -d "{\"patches\":[{\"op\":\"remove\",\"entitlement\":{\"entitlement\":\"administrator\",\"entity_id\":\"2d105c41-4531-4103-8e94-3b14118ca03d\",\"entity_type\":\"controller\"}}]}"
echo -n '  > GET identity entitlements: '
curl $_opts -X GET "$_base/identities/$identity/entitlements"

for n in ${_ids[@]}
do
    echo -n "DELETE group: $n"
    curl $_opts -X DELETE "$_base/groups/$n"
done
echo -n "DELETE group: $group_test"
curl $_opts -X DELETE "$_base/groups/$group_test"

# for n in ${_names[@]}
# do
#     echo -n "DELETE identity: $n@host.com"
#     curl $_opts -X DELETE "$_base/identities/$n@host.com"
# done

# for n in ${_names[@]}
# do
#     echo -n "DELETE role: role-$n"
#     curl $_opts -X DELETE "$_base/roles/role-$n"
# done

# for n in ${_names[@]}
# do
#     echo -n "DELETE idp: idp-$n"
#     curl $_opts -X DELETE "$_base/authentication/idp-$n"
# done
