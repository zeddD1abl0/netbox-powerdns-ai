#!/bin/sh
# Downloads one pinned tool binary (ADR-0014).
#
#   tools/fetch.sh URL SHA256 MEMBER DEST
#
# Downloads URL, checks it against SHA256 (the hash pinned in tools/tools.mk,
# never a checksums file fetched at build time), extracts MEMBER from the
# .tar.gz, and installs it at DEST. DEST appears only if everything succeeds.
set -eu

if [ $# -ne 4 ]; then
	echo "usage: $0 URL SHA256 MEMBER DEST" >&2
	exit 2
fi
url=$1 sha256=$2 member=$3 dest=$4

if [ -z "$sha256" ]; then
	echo "fetch: no pinned SHA-256 for $url" >&2
	exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "fetch: $url"
curl -fsSL --retry 3 -o "$tmp/archive.tar.gz" "$url"
echo "$sha256  $tmp/archive.tar.gz" | sha256sum -c --quiet - || {
	echo "fetch: SHA-256 mismatch for $url" >&2
	exit 1
}
tar -xzf "$tmp/archive.tar.gz" -C "$tmp" "$member"
chmod +x "$tmp/$member"
mkdir -p "$(dirname "$dest")"
mv "$tmp/$member" "$dest.partial"
mv "$dest.partial" "$dest"
