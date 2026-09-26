#!/bin/sh
# Updates or checks the pinned tool binaries in tools/tools.mk (ADR-0014).
#
#   tools/update.sh          move every tool to its latest release
#   tools/update.sh --check  keep the versions; re-derive every pinned SHA-256
#                            from the release's checksums file, and fail with
#                            a diff if any differs. Writes nothing.
#
# For each tool it derives the _VERSION line and every _SHA256_<platform>
# line, taking hashes from the checksums file each release publishes. It edits
# a temporary copy, and replaces tools.mk only after every tool has succeeded.
# Needs curl and network access. Set GITHUB_TOKEN to raise the GitHub API rate
# limit.
set -eu
cd "$(dirname "$0")/.."
mk=tools/tools.mk

mode=latest
if [ "${1:-}" = --check ]; then
	mode=check
elif [ $# -gt 0 ]; then
	echo "usage: $0 [--check]" >&2
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
out=$tmp/tools.mk
cp "$mk" "$out"

for entry in $(mkval BINARY_TOOLS); do
	prefix=${entry%%:*}
	repo=$(mkval "${prefix}_REPO")
	old=$(mkval "${prefix}_VERSION")
	if [ "$mode" = latest ]; then
		new=$(api "https://api.github.com/repos/$repo/releases/latest" |
			sed -n -E 's/^ *"tag_name": *"v?([^"]+)".*/\1/p' | head -n 1)
	else
		new=$old
	fi
	# The version goes into a URL, a sed replacement and tools.mk: allow only
	# the characters a release version uses.
	case $new in
	'' | *[!0-9A-Za-z.+-]*)
		echo "update: $repo: unusable version '$new'" >&2
		exit 1
		;;
	esac

	sums=$(mkval "${prefix}_CHECKSUMS" "${prefix}_VERSION=$new")
	curl -fsSL -o "$tmp/sums" "https://github.com/$repo/releases/download/v$new/$sums"

	sed -i -E "s/^(${prefix}_VERSION[[:space:]]*:= ).*/\\1$new/" "$out"
	for platform in $(sed -n -E "s/^${prefix}_SHA256_([a-z0-9-]+).*/\\1/p" "$mk"); do
		asset=$(mkval "${prefix}_ASSET_$platform" "${prefix}_VERSION=$new")
		hash=$(awk -v a="$asset" '$2 == a || $2 == "*" a { print $1 }' "$tmp/sums")
		# Exactly one SHA-256: 64 lowercase hex digits, and no second line.
		case $hash in
		*[!0-9a-f]*)
			echo "update: $asset: no single SHA-256 in $sums" >&2
			exit 1
			;;
		esac
		if [ ${#hash} -ne 64 ]; then
			echo "update: $asset: no SHA-256 in $sums" >&2
			exit 1
		fi
		sed -i -E "s/^(${prefix}_SHA256_${platform}[[:space:]]*:= ).*/\\1$hash/" "$out"
	done
	echo "update: $repo $old -> $new"
done

if [ "$mode" = check ]; then
	git diff --no-index --exit-code -- "$mk" "$out" || {
		echo "update: a pinned hash differs from the published checksums (shown above); $mk is unchanged" >&2
		exit 1
	}
	echo "update: every pin matches its release's published checksums"
else
	cp "$out" "$mk.new"
	mv "$mk.new" "$mk"
fi
