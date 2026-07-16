#!/usr/bin/env bash
#
# Copyright 2026 Canonical.
#
# Use snap install vault.
# 
# vault-repopulate-creds.sh
#
# Disaster-recovery helper that repopulates JIMM's cloud-credential and
# controller-credential secrets in Vault, matching the paths used by
# internal/vault/vault.go.
#
# Path layout (relative to the KVv2 mount given by VAULT_PATH):
#   Cloud credentials:      creds/<cloud>/<owner>/<name>
#   Controller credentials: creds/<controller-name>
#
# The KVv2 engine inserts a "data/" segment, so the full API paths become:
#   <VAULT_PATH>/data/creds/<cloud>/<owner>/<name>
#   <VAULT_PATH>/data/creds/<controller-name>
#
# Requirements:
#   - vault CLI on PATH, authenticated (VAULT_ADDR + VAULT_TOKEN or equivalent).
#   - VAULT_PATH env var: the KVv2 mount path JIMM uses (the store's KVPath).
#
# Usage:
#   VAULT_PATH=jimm-kv ./vault-repopulate-creds.sh --input creds.json
#   VAULT_PATH=jimm-kv ./vault-repopulate-creds.sh --dry-run --input creds.json
#
# Input JSON format:
# {
#   "cloud_credentials": [
#     {
#       "cloud": "aws",
#       "owner": "alice@canonical.com",
#       "name": "default",
#       "attributes": { "access-key": "AKIA...", "secret-key": "..." }
#     }
#   ],
#   "controller_credentials": [
#     {
#       "controller": "controller-1",
#       "username": "jimm-user",
#       "password": "s3cr3t"
#     }
#   ]
# }

set -euo pipefail

PROG="$(basename "$0")"

DRY_RUN=0
INPUT_FILE=""

usage() {
	cat <<EOF
Usage: VAULT_PATH=<kv-mount> $PROG --input <file.json> [--dry-run]

Repopulates JIMM cloud-credential and controller-credential secrets in Vault.

Options:
  -i, --input FILE   JSON file describing the credentials to write (required).
  -n, --dry-run      Print the vault commands instead of executing them.
  -h, --help         Show this help.

Environment:
  VAULT_PATH   KVv2 mount path used by JIMM (its KVPath). Required.
  VAULT_ADDR   Vault server address (consumed by the vault CLI).
  VAULT_TOKEN  Vault auth token (consumed by the vault CLI).
EOF
}

err() {
	echo "$PROG: error: $*" >&2
	exit 1
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		-i | --input)
			INPUT_FILE="${2:-}"
			shift 2
			;;
		-n | --dry-run)
			DRY_RUN=1
			shift
			;;
		-h | --help)
			usage
			exit 0
			;;
		*)
			err "unknown argument: $1"
			;;
	esac
done

# --- Validation ------------------------------------------------------------

[[ -n "${VAULT_PATH:-}" ]] || err "VAULT_PATH env var is required"
[[ -n "$INPUT_FILE" ]] || err "--input <file.json> is required"
[[ -f "$INPUT_FILE" ]] || err "input file not found: $INPUT_FILE"

command -v vault >/dev/null 2>&1 || err "vault CLI not found on PATH"
command -v jq >/dev/null 2>&1 || err "jq not found on PATH"

# Strip any trailing slash from the mount path for clean joins.
KV_MOUNT="${VAULT_PATH%/}"

# --- Helpers ---------------------------------------------------------------

# vault_put PATH JSON
# Writes (or dry-run prints) a KVv2 secret. The data is supplied as a JSON
# object on stdin so that multi-line values and special characters (e.g. PEM
# private keys) are preserved exactly, rather than being split into separate
# key=value shell arguments.
vault_put() {
	local secret_path="$1"
	local json="$2"
	if [[ "$DRY_RUN" -eq 1 ]]; then
		local keys
		keys="$(jq -r 'keys | join(", ")' <<<"$json")"
		printf 'DRY-RUN: vault kv put %q  (keys: %s)\n' "$secret_path" "$keys" >&2
		return 0
	fi
	printf '%s' "$json" | vault kv put "$secret_path" - >/dev/null
}

# --- Cloud credentials -----------------------------------------------------

cloud_count="$(jq '(.cloud_credentials // []) | length' "$INPUT_FILE")"
echo "$PROG: processing $cloud_count cloud credential(s)" >&2

for i in $(seq 0 $((cloud_count - 1))); do
	[[ "$cloud_count" -eq 0 ]] && break

	cloud="$(jq -r ".cloud_credentials[$i].cloud" "$INPUT_FILE")"
	owner="$(jq -r ".cloud_credentials[$i].owner" "$INPUT_FILE")"
	name="$(jq -r ".cloud_credentials[$i].name" "$INPUT_FILE")"

	if [[ "$cloud" == "null" || "$owner" == "null" || "$name" == "null" ]]; then
		err "cloud_credentials[$i] missing cloud/owner/name"
	fi

	secret_path="${KV_MOUNT}/creds/${cloud}/${owner}/${name}"

	# Extract the attributes object as compact JSON. Values keep their exact
	# form (including newlines) because they are never expanded by the shell.
	attrs_json="$(jq -c ".cloud_credentials[$i].attributes // {}" "$INPUT_FILE")"

	if [[ "$(jq 'length' <<<"$attrs_json")" -eq 0 ]]; then
		echo "$PROG: warning: cloud_credentials[$i] ($secret_path) has no attributes, skipping" >&2
		continue
	fi

	echo "$PROG: writing cloud credential -> $secret_path" >&2
	vault_put "$secret_path" "$attrs_json"
done

# --- Controller credentials ------------------------------------------------

ctrl_count="$(jq '(.controller_credentials // []) | length' "$INPUT_FILE")"
echo "$PROG: processing $ctrl_count controller credential(s)" >&2

for i in $(seq 0 $((ctrl_count - 1))); do
	[[ "$ctrl_count" -eq 0 ]] && break

	controller="$(jq -r ".controller_credentials[$i].controller" "$INPUT_FILE")"
	username="$(jq -r ".controller_credentials[$i].username" "$INPUT_FILE")"
	password="$(jq -r ".controller_credentials[$i].password" "$INPUT_FILE")"

	if [[ "$controller" == "null" || "$username" == "null" || "$password" == "null" ]]; then
		err "controller_credentials[$i] missing controller/username/password"
	fi

	secret_path="${KV_MOUNT}/creds/${controller}"

	ctrl_json="$(jq -cn --arg u "$username" --arg p "$password" '{username: $u, password: $p}')"

	echo "$PROG: writing controller credential -> $secret_path" >&2
	vault_put "$secret_path" "$ctrl_json"
done

echo "$PROG: done" >&2
