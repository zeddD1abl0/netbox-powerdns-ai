#!/bin/sh
# Downloads one pinned tool binary (ADR-0022).
#
#   tools/fetch.sh URL SHA256 MEMBER DEST
#
# Downloads URL, checks it against SHA256 (the hash pinned in tools/tools.mk,
# never a checksums file fetched at build time), and installs the binary at
# DEST. MEMBER is the binary's path inside the .tar.gz, or - when the download
# is the binary itself. DEST exists afterwards only if everything succeeded: a
# binary from an earlier pin is removed first.
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

rm -f "$dest"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "fetch: $url"
curl -fsSL --retry 3 -o "$tmp/download" "$url"
echo "$sha256  $tmp/download" | sha256sum -c --quiet - || {
	echo "fetch: SHA-256 mismatch for $url" >&2
	exit 1
}
if [ "$member" = - ]; then
	bin=$tmp/download
else
	mkdir "$tmp/archive"
	tar -xzf "$tmp/download" -C "$tmp/archive" "$member"
	bin=$tmp/archive/$member
fi
chmod +x "$bin"
# tar keeps the archive's mtime. Make compares DEST with tools.mk, so give it
# the time of the fetch.
touch "$bin"
mkdir -p "$(dirname "$dest")"
mv "$bin" "$dest.partial"
mv "$dest.partial" "$dest"
