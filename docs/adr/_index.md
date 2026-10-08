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
| [0011](0011-documentation-platform-hugo.md) | Documentation platform — Hugo | superseded by ADR-0017 |
| [0012](0012-api-standard.md) | API standard — OpenAPI 3.1 spec-first, Zalando guidelines | accepted |
| [0013](0013-toolchain-per-tool-modules-and-c-compiler.md) | Self-contained toolchain, revised — per-tool modules and a C compiler | superseded by ADR-0014 |
| [0014](0014-toolchain-pinned-release-binaries-on-glibc-linux.md) | Toolchain: pinned release binaries on glibc Linux | superseded by ADR-0022 |
| [0015](0015-glibc-based-debian-or-ubuntu-images-for-ci-and-con.md) | glibc-based Debian or Ubuntu images for CI and containers | superseded by ADR-0031 |
| [0016](0016-staged-ci-pipelines-that-mirror-make-ci.md) | Staged CI pipelines that mirror `make ci` | superseded by ADR-0032 |
| [0017](0017-documentation-platform-hugo-restated-for-the-relea.md) | Documentation platform — Hugo, restated for the release-binary toolchain | accepted |
| [0018](0018-merge-milestones-through-gitlab-merge-requests.md) | Merge milestones through GitLab merge requests | accepted |
| [0019](0019-re-slice-the-milestones-into-smaller-steps.md) | Re-slice the milestones into smaller steps | accepted |
| [0020](0020-netbox-client-and-normalized-dns-model.md) | NetBox client and normalized DNS model | superseded by ADR-0023 |
| [0021](0021-cobra-and-viper-for-commands-and-configuration.md) | Cobra and Viper for commands and configuration | accepted |
| [0022](0022-toolchain-with-the-c-compiler-that-race-needs.md) | Toolchain with the C compiler that -race needs | accepted |
| [0023](0023-netbox-client-and-normalized-dns-model-with-netbox.md) | NetBox client and normalized DNS model, with NetBox 4.7 only | accepted |
| [0024](0024-read-powerdns-through-its-api-from-server-groups-i.md) | Read PowerDNS through its API, from server groups in the config file | superseded by ADR-0026 |
| [0025](0025-normalize-record-data-with-miekg-dns-v2.md) | Normalize record data with miekg/dns v2 | accepted |
| [0026](0026-read-powerdns-through-its-api-with-powerdns-5-1-on.md) | Read PowerDNS through its API, with PowerDNS 5.1 only | accepted |
| [0027](0027-report-drift-between-netbox-and-each-server-group.md) | Report drift between NetBox and each server group's primary | accepted |
| [0028](0028-split-packaging-into-its-own-milestone.md) | Split packaging into its own milestone | accepted |
| [0029](0029-run-nbpdns-as-a-service-with-prometheus-metrics-an.md) | Run nbpdns as a service with Prometheus metrics and OTLP traces | accepted |
| [0030](0030-release-with-goreleaser-and-ko-to-gitlab-on-versio.md) | Release with GoReleaser and ko to GitLab on version tags | accepted |
| [0031](0031-debian-images-for-ci-with-a-distroless-static-runt.md) | Debian images for CI, with a distroless static runtime image | accepted |
| [0032](0032-staged-ci-pipelines-with-a-tag-only-release-stage.md) | Staged CI pipelines with a tag-only release stage | accepted |
| [0033](0033-a-read-only-api-spec-first-generated-with-oapi-cod.md) | A read-only API, spec-first, generated with oapi-codegen | accepted |
| [0034](0034-a-vendored-locked-down-scalar-viewer-at-api-docs.md) | A vendored, locked-down Scalar viewer at /api/docs | accepted |
| [0035](0035-refresh-the-zones-that-netbox-s-webhooks-name.md) | Refresh the zones that NetBox's webhooks name | accepted |

<!-- projctl:adr-index:end -->
