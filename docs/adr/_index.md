---
title: Decision records
weight: 50
---

# Architecture decision records

Each significant decision is recorded as one ADR in
[MADR](https://adr.github.io/madr/) format. See
[ADR-0001](0001-record-architecture-decisions.md) for the rules. New ADRs start
from [`template.md`](template.md).

<!-- projctl:adr-index:start -->

| ADR | Title | Status |
|---|---|---|
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions | accepted |
| [0002](0002-track-work-in-repo.md) | Track work in the repo as small files with generated indexes | accepted |
| [0003](0003-self-contained-forge-neutral-toolchain.md) | Self-contained, forge-neutral toolchain | superseded by ADR-0013 |
| [0004](0004-v1-scope-netbox-source-of-truth-powerdns-auth.md) | v1 scope — NetBox is the source of truth, PowerDNS Authoritative is the target | accepted |
| [0005](0005-project-identity.md) | Project identity — name, module path, env prefix, license | accepted |
| [0006](0006-integrations-netbox-dns-plugin-and-powerdns-api.md) | Integrations — read the NetBox DNS plugin, write through the PowerDNS API only | accepted |
| [0007](0007-server-groups-and-catalog-zones.md) | Server groups, v1 topologies and catalog zones | accepted |
| [0008](0008-per-zone-drift-policy.md) | Drift policy per zone | accepted |
| [0009](0009-persistence-and-high-availability.md) | Persistence and high availability — PostgreSQL and SQLite | accepted |
| [0010](0010-claude-commits-per-item-on-milestone-branches.md) | Claude commits per item on milestone branches | accepted |
| [0011](0011-documentation-platform-hugo.md) | Documentation platform — Hugo | accepted |
| [0012](0012-api-standard.md) | API standard — OpenAPI 3.1 spec-first, Zalando guidelines | accepted |
| [0013](0013-toolchain-per-tool-modules-and-c-compiler.md) | Self-contained toolchain, revised — per-tool modules and a C compiler | superseded by ADR-0014 |
| [0014](0014-toolchain-pinned-release-binaries-on-glibc-linux.md) | Toolchain: pinned release binaries on glibc Linux | accepted |
| [0015](0015-glibc-based-debian-or-ubuntu-images-for-ci-and-con.md) | glibc-based Debian or Ubuntu images for CI and containers | accepted |

<!-- projctl:adr-index:end -->
