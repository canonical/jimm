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
# We want an anchor immediately above each heading:
#   (command-jaas-<command>)=
#
# Commands are expected to be a single token (no spaces), so we can
# transform matching lines directly without extra sanitization.
sed -E '/^# jaas [^[:space:]]+[[:space:]]*$/{h;s/^# jaas /(command-jaas-/;s/[[:space:]]*$/)=/;G}' "${md_in}" > "${out_file}"

echo "Updated: ${out_file}"
