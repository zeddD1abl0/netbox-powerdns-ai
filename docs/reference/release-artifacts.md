---
title: Release artifacts
weight: 35
---

# Release artifacts

What each nbpdns release publishes, and where. A release is made from a
`vMAJOR.MINOR.PATCH` tag, such as `v0.1.0`
([ADR-0030](../adr/0030-release-with-goreleaser-and-ko-to-gitlab-on-versio.md)).

## Versions

- Versions are [semantic](https://semver.org/), and 0.x until 1.0.
- The tag has a `v`, and the version in filenames and image tags doesn't:
  tag `v0.1.0` publishes `nbpdns_0.1.0_linux_amd64.tar.gz` and the image
  tag `0.1.0`.
- `nbpdns version` reports the tag, `v0.1.0`, and `modified false`. A build
  from any other commit reports a Go pseudo-version, such as
  `v0.1.1-0.20261012093000-abcdef123456`, with `+dirty` and
  `modified true` if the working tree had uncommitted changes.

## Platforms

| OS | Architecture | `uname -m` |
|---|---|---|
| Linux | amd64 | `x86_64` |
| Linux | arm64 | `aarch64` |

The binaries are static, with no C library or other shared library, built
with `CGO_ENABLED=0`, `-trimpath`, and `-ldflags=-s -w`.

## GitLab release

Each tag has a GitLab release, under **Deploy > Releases**. Its notes are
the version's section of `CHANGELOG.md`. Its files are stored in the
project's generic package registry, and linked from the release:

| File | Holds |
|---|---|
| `nbpdns_<version>_linux_amd64.tar.gz` | The amd64 binary, and the documents below |
| `nbpdns_<version>_linux_arm64.tar.gz` | The arm64 binary, and the documents below |
| `checksums.txt` | The SHA-256 of each archive |

### Archives

Each archive is a gzip-compressed tar file, with these files at its root:

| File | Mode |
|---|---|
| `nbpdns` | `0755` |
| `LICENSE` | `0644` |
| `README.md` | `0644` |
| `CHANGELOG.md` | `0644` |

Every file is owned by `root` (0:0), and its modification time is the time
of the commit. The same commit builds byte-identical archives.

### `checksums.txt`

One line for each archive, in the form `sha256sum` writes and checks: the
hexadecimal SHA-256, two spaces, and the filename. For example, from a
snapshot build:

```text
32a55ce7bc4f8f1eb0a4155ffd8393247f72b65382cdbc28bf5e13e9cf5b3b02  nbpdns_0.0.0-SNAPSHOT-db441f1_linux_amd64.tar.gz
696386ef9352a234aca1d7d3554ed0e118cff6f6abe30a847d1cd564c2fae57b  nbpdns_0.0.0-SNAPSHOT-db441f1_linux_arm64.tar.gz
```

The archives and `checksums.txt` aren't signed until M17.

## Container image

The image is in the project's container registry, under **Deploy >
Container registry**. It's one multi-platform OCI index, for linux/amd64
and linux/arm64/v8, so each tag pulls the image for its host.

### Tags

| Tag | Points at | Moves |
|---|---|---|
| `<version>`, such as `0.1.0` | That release | Never |
| `<major>.<minor>`, such as `0.1` | The latest patch release of that minor version | With each patch release |
| `latest` | The latest release | With each release |

Every release moves `<major>.<minor>` and `latest` to itself. Releases are
made in version order, so that's the newest release.

### Configuration

| Setting | Value |
|---|---|
| Base image | `gcr.io/distroless/static-debian13:nonroot`, pinned by digest |
| User | `65532:65532`, `nonroot` |
| Entrypoint | `/ko-app/nbpdns`, with no default command |
| Working directory | `/home/nonroot` |
| `PATH` | The base image's, with `/ko-app` |
| `SSL_CERT_FILE` | `/etc/ssl/certs/ca-certificates.crt`, Debian's CA certificates |
| `KO_DATA_PATH` | `/var/run/ko`, which nbpdns doesn't use |
| Exposed ports | None declared; `serve` listens on `server.listen`, `:8080` by default |

The image has no shell and no package manager.

### Labels

| Label | Value |
|---|---|
| `org.opencontainers.image.title` | `nbpdns` |
| `org.opencontainers.image.description` | `Keeps PowerDNS Authoritative in step with NetBox's DNS plugin.` |
| `org.opencontainers.image.version` | The version, such as `0.1.0` |
| `org.opencontainers.image.revision` | The full hash of the commit |
| `org.opencontainers.image.created` | The time of the commit, in RFC 3339 form |
| `org.opencontainers.image.source` | `https://github.com/zeddD1abl0/netbox-powerdns-ai` |
| `org.opencontainers.image.licenses` | `Apache-2.0` |

### SBOM

ko attaches an SPDX 2.3 software bill of materials (SBOM), as JSON, to the
index and to each platform's image:

| SBOM of | Lists |
|---|---|
| Each platform's image | The Go modules compiled into nbpdns, with their versions, and the base image, by digest |
| The index | The base image, and the platform images, by digest |

They follow cosign's convention for attachments. Each is stored in the
image's repository, under the tag `sha256-<hex>.sbom`, where `<hex>` is the
digest of the index or image it describes, without its `sha256:` prefix. It's that
tag's one layer, with media type `text/spdx+json`.
