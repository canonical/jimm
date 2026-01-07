#!/usr/bin/env bash
set -euo pipefail

# Regenerates the JAAS CLI reference documentation.
#
# Output: docs/reference/jaas-plugin.rst

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out_file="${repo_root}/docs/reference/jaas-plugin.rst"

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
rst_out="${tmp_dir}/documentation.rst"

if command -v pandoc >/dev/null 2>&1; then
	echo "Converting markdown -> reStructuredText (pandoc)..."
	pandoc "${md_in}" -o "${rst_out}" --wrap=none
elif command -v docker >/dev/null 2>&1; then
	echo "Converting markdown -> reStructuredText (pandoc via docker)..."
	docker run --rm -v "${tmp_dir}:/data" -w /data pandoc/core:3.5 \
		documentation.md -o documentation.rst --wrap=none
else
	echo "ERROR: pandoc is required (install pandoc, or install docker for the fallback)." >&2
	exit 2
fi

final_out="${tmp_dir}/jaas-plugin.rst"
{
	echo "\`\`jaas\`\` plugin"
	echo "###############"
	echo
	cat "${rst_out}"
} > "${final_out}"

mkdir -p "$(dirname "${out_file}")"
# Only replace the file once generation succeeded.
cp "${final_out}" "${out_file}"

echo "Updated: ${out_file}"
