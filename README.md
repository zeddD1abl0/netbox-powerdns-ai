# nbpdns

> This repository is named `netbox-powerdns-ai`; the product and its binary are
> **`nbpdns`** ([ADR-0005](docs/adr/0005-project-identity.md)).

A control plane that takes DNS data from **NetBox** (the source of truth) and
applies it to **PowerDNS Authoritative** servers. It verifies each change,
reports drift, and records a full audit trail that can be sent to a SIEM. It
runs as a single binary or a container, and has a web UI, an API, SSO, and
IaC-driven configuration.

> [!NOTE]
> This project is at **M0 (Foundation)**. There's no product code yet. The
> scope is being settled; see the open questions in
> [`project/requirements.md`](project/requirements.md).

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
[`CLAUDE.md`](CLAUDE.md). The only prerequisites are Go, Docker and make
([ADR-0003](docs/adr/0003-self-contained-forge-neutral-toolchain.md)). Build
commands arrive with the M0 toolchain work.
