#!/usr/bin/env bash
set -euo pipefail

# Regenerates the JAAS CLI reference documentation.
#
# Output: docs/reference/list-of-jaas-plugin-commands/index.md

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out_file="${repo_root}/docs/reference/list-of-jaas-plugin-commands/index.md"

tmp_dir="$(mktemp -d)"
cleanup() {
	rm -rf "${tmp_dir}"
}
trap cleanup EXIT

cd "${repo_root}"

echo "Building jaas CLI..."
# Build into the temporary directory so we don't leave binaries behind.
go build -o "${tmp_dir}/jaas" ./cmd/jaas

echo "Generating markdown reference..."
"${tmp_dir}/jaas" documentation --no-index=true --out "${tmp_dir}"

md_in="${tmp_dir}/documentation.md"

mkdir -p "$(dirname "${out_file}")"

# Add MyST markdown anchors for each command heading.
# The generated documentation uses headings of the form:
#   # jaas <command>
# We want a stable anchor immediately above each heading:
#   (command-jaas-<command>)=
#
# This `sed` expression looks for level-1 headings like:
#   # jaas <command>
# and rewrites each match into two lines:
#   (command-jaas-<command>)=
#   # jaas <command>
#
# Notes:
# - GNU sed supports `\s` and `\S` (whitespace / non-whitespace) which read a bit
#   more naturally than POSIX `[[:space:]]`. BSD sed (macOS) does not.
# - The replacement uses a literal newline via a backslash at end-of-line.
sed -E 's/^# jaas\s+(\S+)\s*$/\(command-jaas-\1\)=\
# jaas \1/' "${md_in}" > "${out_file}"

echo "Updated: ${out_file}"
