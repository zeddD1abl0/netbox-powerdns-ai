# nbpdns

> This repository is named `netbox-powerdns-ai`; the product and its binary are
> **`nbpdns`** ([ADR-0005](docs/adr/0005-project-identity.md)).

A control plane that takes DNS data from **NetBox** (the source of truth) and
applies it to **PowerDNS Authoritative** servers. It verifies each change,
reports drift, and records a full audit trail that can be sent to a SIEM. It
runs as a single binary or a container, and has a web UI, an API, SSO, and
IaC-driven configuration.

> [!NOTE]
> This project is in early development, and its releases are 0.x until 1.0.
> So far, nbpdns reads: it reports drift between NetBox and PowerDNS, but
> doesn't change PowerDNS yet. The [project board](project/README.md) shows
> the current milestone and what comes next.

## Install

Each release is a static Linux binary, for amd64 and arm64, and a
multi-platform container image, on the project's GitLab, under **Deploy >
Releases** and **Deploy > Container registry**.
[Install nbpdns](docs/how-to/install-nbpdns.md) downloads and checks a
release, and [Run nbpdns in a container](docs/how-to/run-nbpdns-in-a-container.md)
runs the image.

## Where to look

| For | Read |
|---|---|
| Current status and next work | [`project/README.md`](project/README.md) |
| What the product must do, and what's still undecided | [`project/requirements.md`](project/requirements.md) |
| Why it's built this way | [`docs/adr/`](docs/adr/) |
| Documentation | [`docs/`](docs/_index.md) |
| How to contribute (people and AI agents) | [`CLAUDE.md`](CLAUDE.md) |

## Development

Claude develops this project from start to finish, following
[`CLAUDE.md`](CLAUDE.md). Development and CI run on glibc Linux amd64 (Debian
or Ubuntu). The only prerequisites are Go, Docker, make, curl, tar,
sha256sum, and a C compiler, which the race detector needs: on Debian or
Ubuntu, `gcc` and `libc6-dev`
([ADR-0022](docs/adr/0022-toolchain-with-the-c-compiler-that-race-needs.md)).
Every other tool is pinned in the repository and fetched on first use. On
other platforms, `make shell` runs everything inside the CI image.

```shell
make            # list every target
make build      # build bin/nbpdns
make ci         # run exactly what CI runs
make docs-serve # preview the documentation site
```
