#!/bin/sh
# Updates the pinned tool binaries in tools/tools.mk (ADR-0014).
#
#   tools/update.sh            move every tool to its latest release
#   tools/update.sh --current  keep the versions; re-derive every pinned
#                              SHA-256 from the release's checksums file
#
# For each tool it rewrites the _VERSION line and every _SHA256_<platform>
# line, taking hashes from the checksums file each release publishes.
# Needs curl and network access. Set GITHUB_TOKEN to raise the GitHub API rate
# limit.
set -eu
cd "$(dirname "$0")/.."
mk=tools/tools.mk

mode=latest
if [ "${1:-}" = --current ]; then
	mode=current
elif [ $# -gt 0 ]; then
	echo "usage: $0 [--current]" >&2
	exit 2
fi

# mkval NAME [VAR=value…]: the value of NAME in tools.mk, with overrides.
mkval() {
	name=$1
	shift
	make --no-print-directory -s -f "$mk" --eval 'print-value: ; @echo $($(PRINT))' print-value PRINT="$name" "$@"
}

api() {
	if [ -n "${GITHUB_TOKEN:-}" ]; then
		curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" "$1"
	else
		curl -fsSL "$1"
	fi
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

for entry in $(mkval BINARY_TOOLS); do
	prefix=${entry%%:*}
	repo=$(mkval "${prefix}_REPO")
	old=$(mkval "${prefix}_VERSION")
	if [ "$mode" = latest ]; then
		new=$(api "https://api.github.com/repos/$repo/releases/latest" |
			sed -n -E 's/^ *"tag_name": *"v?([^"]+)".*/\1/p' | head -n 1)
		[ -n "$new" ] || { echo "update: no latest release for $repo" >&2; exit 1; }
	else
		new=$old
	fi

	sums=$(mkval "${prefix}_CHECKSUMS" "${prefix}_VERSION=$new")
	curl -fsSL -o "$tmp/sums" "https://github.com/$repo/releases/download/v$new/$sums"

	sed -i -E "s/^(${prefix}_VERSION[[:space:]]*:= ).*/\\1$new/" "$mk"
	for platform in $(sed -n -E "s/^${prefix}_SHA256_([a-z0-9-]+).*/\\1/p" "$mk"); do
		asset=$(mkval "${prefix}_ASSET_$platform" "${prefix}_VERSION=$new")
		hash=$(awk -v a="$asset" '$2 == a || $2 == "*" a { print $1 }' "$tmp/sums")
		[ -n "$hash" ] || { echo "update: $asset isn't in $sums" >&2; exit 1; }
		sed -i -E "s/^(${prefix}_SHA256_${platform}[[:space:]]*:= ).*/\\1$hash/" "$mk"
	done
	echo "update: $repo $old -> $new"
done
