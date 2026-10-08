# Requirements and open questions

- **Requirements** (`REQ-nnn`) come from the [brief](brief.md) and from answered
  questions.
- **Open questions** (`Q-nnn`) are the gaps the brief leaves open. Each one has a
  proposed default, which applies unless the user decides otherwise.
- **IDs are permanent.** Never renumber or reuse one. A requirement that's
  dropped stays in the table, marked `withdrawn`.

When a question is answered:
1. move its row to [Answered](#answered), with the date and the answer;
2. add or update the REQ it produces, or write an ADR;
3. link the two.

## Requirements

| ID | Requirement | Source |
|---|---|---|
| REQ-001 | Written in Go. | Brief |
| REQ-002 | Runs as a single binary. | Brief |
| REQ-003 | Runs as a container or Kubernetes pod. | Brief |
| REQ-004 | Configurable through a web interface. | Brief |
| REQ-005 | Configurable through environment variables. | Brief |
| REQ-006 | Configurable through a config file. | Brief ("possibly") |
| REQ-007 | Provides user management. | Brief |
| REQ-008 | Supports single sign-on. | Brief |
| REQ-009 | Manages multiple DNS servers in various complex setups. | Brief |
| REQ-010 | Sends logs to a SIEM and/or other external systems. | Brief |
| REQ-011 | Supports auditing. | Brief |
| REQ-012 | Supports traceability. | Brief |
| REQ-013 | Provides an API. | Brief |
| REQ-014 | Exposes clearly defined metrics. | Brief |
| REQ-015 | Settings can be changed, including through the API. | Brief |
| REQ-016 | Designed so it can be extended later. | Brief |
| REQ-017 | Configuration can be driven by IaC: Terraform, OpenTofu, Ansible. | Brief ("ideally") |
| REQ-018 | Documentation is clear and covers every API endpoint. | Brief |
| REQ-019 | The API follows a documented standard. | Brief |
| REQ-020 | Documentation follows a standard process. | Brief |
| REQ-021 | Documentation reads well without embellishment, and uses the features of the chosen docs platform. | Brief |
| REQ-022 | The development setup is repeatable and expandable, for development done entirely by Claude. | Brief |
| REQ-023 | v1 manages PowerDNS Authoritative. | Q-001, [ADR-0004](../docs/adr/0004-v1-scope-netbox-source-of-truth-powerdns-auth.md) |
| REQ-024 | NetBox is the source of truth for DNS data. | Q-002, [ADR-0004](../docs/adr/0004-v1-scope-netbox-source-of-truth-powerdns-auth.md) |
| REQ-025 | Project work is tracked in the repo, in Markdown. | Q-003, [ADR-0002](../docs/adr/0002-track-work-in-repo.md) |
| REQ-026 | The project is self-contained and not tied to any forge. | Q-004, [ADR-0003](../docs/adr/0003-self-contained-forge-neutral-toolchain.md), superseded by [ADR-0013](../docs/adr/0013-toolchain-per-tool-modules-and-c-compiler.md), then [ADR-0014](../docs/adr/0014-toolchain-pinned-release-binaries-on-glibc-linux.md), then [ADR-0022](../docs/adr/0022-toolchain-with-the-c-compiler-that-race-needs.md) |
| REQ-027 | DNS data is read from the NetBox DNS plugin. | Q-006, [ADR-0006](../docs/adr/0006-integrations-netbox-dns-plugin-and-powerdns-api.md) |
| REQ-028 | The app reaches PowerDNS only through its HTTP API, never through backend databases or zone files. | Q-008, [ADR-0006](../docs/adr/0006-integrations-netbox-dns-plugin-and-powerdns-api.md) |
| REQ-029 | v1 supports primary → secondaries topologies, including hidden primaries. | Q-009, [ADR-0007](../docs/adr/0007-server-groups-and-catalog-zones.md) |
| REQ-030 | v1 supports independent sites and clusters, each managed as a separate server group. | Q-009, [ADR-0007](../docs/adr/0007-server-groups-and-catalog-zones.md) |
| REQ-031 | Drift handling is set per zone: enforce, report or ignore. New zones default to report. | Q-010, [ADR-0008](../docs/adr/0008-per-zone-drift-policy.md) |
| REQ-032 | Data is stored in PostgreSQL or in embedded SQLite. | Q-019, [ADR-0009](../docs/adr/0009-persistence-and-high-availability.md) |
| REQ-033 | On PostgreSQL, several replicas can run at once, with exactly one active sync worker. SQLite runs as a single instance. | Q-020, [ADR-0009](../docs/adr/0009-persistence-and-high-availability.md) |
| REQ-034 | Secondaries learn about zones through catalog zones (RFC 9432). | Q-052, [ADR-0007](../docs/adr/0007-server-groups-and-catalog-zones.md) |
| REQ-035 | The project is licensed under Apache-2.0. | Q-005, [ADR-0005](../docs/adr/0005-project-identity.md) |
| REQ-036 | End-to-end tests run against a containerised lab: NetBox with the DNS plugin, a PowerDNS primary, and secondaries. | Q-048 |
| REQ-037 | The API follows the Zalando RESTful API Guidelines in full, with no version in URL paths. | Q-056, [ADR-0012](../docs/adr/0012-api-standard.md) |
| REQ-038 | CI and container build images are glibc-based (Debian or Ubuntu); Alpine/musl isn't required. The runtime image is Debian's distroless static, since nbpdns is a static binary. | User, 2026-09-25 and 2026-10-07; [ADR-0015](../docs/adr/0015-glibc-based-debian-or-ubuntu-images-for-ci-and-con.md), superseded by [ADR-0031](../docs/adr/0031-debian-images-for-ci-with-a-distroless-static-runt.md) |
| REQ-039 | CI pipelines are split into stages (lint, test, build, security, and more later); `make ci` remains the local "do everything" target. | User, 2026-09-25 |
| REQ-040 | Reads NetBox 4.7, with the NetBox DNS plugin 1.7.x, through the REST API with a read-only token. More releases are added as the CI runners have room to test them. Narrowed from 4.7 and 4.6 on 2026-10-06. | Q-007, [ADR-0020](../docs/adr/0020-netbox-client-and-normalized-dns-model.md), superseded by [ADR-0023](../docs/adr/0023-netbox-client-and-normalized-dns-model-with-netbox.md) |
| REQ-041 | Reads PowerDNS Authoritative 5.1 through its HTTP API, from each server group's primary. More releases are added as the CI runners have room to test them. Narrowed from 5.1 and 5.0 on 2026-10-06. | Q-053, [ADR-0024](../docs/adr/0024-read-powerdns-through-its-api-from-server-groups-i.md), superseded by [ADR-0026](../docs/adr/0026-read-powerdns-through-its-api-with-powerdns-5-1-on.md) |
| REQ-042 | Zones are assigned to server groups by NetBox view, and a view may be served by several groups. | User, 2026-10-06; [ADR-0024](../docs/adr/0024-read-powerdns-through-its-api-from-server-groups-i.md), superseded by [ADR-0026](../docs/adr/0026-read-powerdns-through-its-api-with-powerdns-5-1-on.md) |
| REQ-043 | nbpdns is designed and tested for 1,000 zones and 100,000 records per run; larger targets are set when a deployment needs them. | Q-017, [ADR-0027](../docs/adr/0027-report-drift-between-netbox-and-each-server-group.md) |
| REQ-044 | nbpdns exposes Prometheus metrics for its drift reports, its refreshes, and its requests to NetBox and PowerDNS. PowerDNS's own statistics stay on PowerDNS's `/metrics`. | Q-038, [ADR-0029](../docs/adr/0029-run-nbpdns-as-a-service-with-prometheus-metrics-an.md) |
| REQ-045 | nbpdns is released, on version tags, as static linux amd64 and arm64 binaries with SHA-256 checksums, and as a multi-arch, non-root, distroless container image with an SBOM, built the same way every time. | Q-025, [ADR-0030](../docs/adr/0030-release-with-goreleaser-and-ko-to-gitlab-on-versio.md), [ADR-0031](../docs/adr/0031-debian-images-for-ci-with-a-distroless-static-runt.md) |
| REQ-046 | nbpdns serves a read-only API, documented by `api/openapi.yaml`, of each server group's drift, its zones and their changes, and the service's status. Unauthenticated until M10. | Q-041, [ADR-0033](../docs/adr/0033-a-read-only-api-spec-first-generated-with-oapi-cod.md), [ADR-0034](../docs/adr/0034-a-vendored-locked-down-scalar-viewer-at-api-docs.md) |
| REQ-047 | The API serves each zone's DNS records as NetBox defines them, in nbpdns's normalized form, for IaC to read, for example to publish them through other providers. Records are written only in NetBox. | Q-041, [ADR-0033](../docs/adr/0033-a-read-only-api-spec-first-generated-with-oapi-cod.md) |

## Open questions

**Blocking** questions must be answered before M1 is designed. **Needed by**
names the milestone that needs the answer, from the milestone list in
[ADR-0019](../docs/adr/0019-re-slice-the-milestones-into-smaller-steps.md).

### Product and domain

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-011 | Where does data that NetBox doesn't model live: TSIG keys, zone metadata (ALLOW-AXFR-FROM, ALSO-NOTIFY, SOA-EDIT-API), serial policy, DNSSEC key rollover? | In NetBox wherever the plugin models it. Everything else is per-zone or per-group config in this app. | M13 |
| Q-012 | Does "change settings" mean this app's settings only, or PowerDNS server config (`pdns.conf`) too? | This app's settings plus per-zone PowerDNS metadata. `pdns.conf` stays with Ansible. | M08 |
| Q-013 | What change safety is needed: dry-run diff, four-eyes approval, change windows, blast-radius limits, rollback? | Every sync computes a plan and auto-applies below thresholds. Above a threshold (such as more than N deletes, or NS/SOA changes) it needs approval. | M14 |
| Q-014 | What validation runs before and after changes? | Pre-flight checks (CNAME at apex, dangling NS, TTL bounds, syntax). After apply, query every server for the SOA serial and sample records. | M14 |
| Q-015 | Is multi-tenancy needed: are permissions scoped to zone, NetBox tenant or server group? | Global roles in v1, scoped by server group. Tenant scoping is a later ADR. | M11 |
| Q-016 | What is the web UI's scope? | Settings, ops dashboard (sync status, drift, per-server health), approvals, audit viewer, users and roles. **No record editor**, since NetBox is the editor. | M12 |
| Q-054 | The parts of Q-010 not yet answered: how is a sync triggered, and how are existing PowerDNS zones adopted into NetBox (brownfield import)? | A NetBox event-rule webhook plus a periodic full reconcile. An import tool for first adoption, with imported zones starting in report mode. | M07 (triggers), M15 (import) |

### Architecture and deployment

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-023 | How are secrets stored at rest (PowerDNS API keys, TSIG, OIDC client secrets)? | Envelope encryption with a master key from env or file. Vault/OpenBao later. | M08 |
| Q-024 | When the UI and IaC both manage a setting, which one owns it? | A `managed_by` field on each resource. Resources owned by IaC are read-only in the UI. | M08 |
| Q-026 | How are upgrades, backup and restore, and config export handled? | Forward-only migrations. `export` and `import` commands for app config. | M08 (migrations), M17 (rest) |

### Identity and access

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-028 | Which IdP and protocols? | Authentik, with OIDC, SAML and trusted proxy headers, each toggled independently. No LDAP in v1. | M11 |
| Q-029 | Are local users, MFA and break-glass access needed? | Local users with TOTP/WebAuthn, plus an env-only break-glass token. Every use is audited. | M10 |
| Q-030 | What RBAC model? | Viewer, Operator and Admin built in. Custom roles. IdP group → role mapping. Permissions checked per action. | M11 |
| Q-031 | How do Terraform, Ansible and CI pipelines authenticate? | Scoped, expiring, hashed API tokens and service accounts. OIDC workload identity (CI JWTs) later. | M10 |
| Q-032 | Is SCIM provisioning needed? | Not in v1. | M11 |

### Audit, logging and SIEM

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-033 | Which SIEMs or log platforms must be supported (Splunk, Elastic, Sentinel, Wazuh, Graylog, Loki, QRadar)? | Sinks: stdout JSON, file, syslog RFC 5424 over TLS, HTTP (HEC-compatible), OTLP logs. | M16 |
| Q-034 | What wire format? | Native versioned JSON first. OCSF and CEF mappings are ADR candidates. | M16 |
| Q-035 | What audit coverage, retention and tamper evidence are needed, and what happens when the SIEM is down? | All auth, config, RBAC, token, plan, apply, drift and approval events. Hash-chained. Retention configurable. An outbox buffers events, with an alert on backlog. | M08 |
| Q-036 | Which compliance frameworks apply (ISO 27001, SOC 2, PCI DSS, Essential Eight/ISM, NIS2)? | Needs an answer. It affects retention, crypto and MFA. | M08 |
| Q-037 | How far does traceability go? | End to end: carry NetBox's changelog identity (user, `request_id`, ObjectChange ID) through sync → apply → each server → verification, so one trace links the NetBox edit to every PowerDNS write. | M07 |

### API, metrics and extensibility

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-040 | Are rate limits and quotas needed? | Limits per token and per IP. | M10 |

### IaC

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-042 | Where do the Terraform/OpenTofu provider and the Ansible collection live, and which registries do they publish to? | Separate repos in later milestones, built with terraform-plugin-framework. The API is designed for them from M1: declarative, idempotent, stable IDs, import support. | M18 |

### Documentation

| ID | Question | Proposed default | Needed by |
|---|---|---|---|

### Process and environment

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-050 | Which UI technology? | Server-rendered templ + htmx with vendored assets and no Node build, embedded in the binary. | M12 |
| Q-051 | What accessibility and localisation level? | WCAG 2.2 AA. English only. | M12 |

## Answered

| ID | Question | Answer | Date | Produced |
|---|---|---|---|---|
| Q-001 | Which DNS server software must the first release manage? | PowerDNS Authoritative only. | 2026-09-25 | REQ-023, [ADR-0004](../docs/adr/0004-v1-scope-netbox-source-of-truth-powerdns-auth.md) |
| Q-002 | What is NetBox's role relative to this application? | NetBox is the source of truth. | 2026-09-25 | REQ-024, [ADR-0004](../docs/adr/0004-v1-scope-netbox-source-of-truth-powerdns-auth.md) |
| Q-003 | Where should the project's work be tracked? | In-repo Markdown only. | 2026-09-25 | REQ-025, [ADR-0002](../docs/adr/0002-track-work-in-repo.md) |
| Q-004 | Which documentation platform? | Platform not chosen; the answer set a constraint instead. The project must not be GitLab-only and should be as self-contained as possible. The platform question continues as Q-044. | 2026-09-25 | REQ-026, [ADR-0003](../docs/adr/0003-self-contained-forge-neutral-toolchain.md) |
| Q-005 | Product name, binary name, Go module path, env var prefix, license; the meaning of "ai" in the repo name. | Product and binary `nbpdns`, env prefix `NBPDNS_`. Module path `github.com/<owner>/netbox-powerdns-ai`, keeping the repo name; the owner is split out as Q-055. License Apache-2.0. The product name drops "ai", so the repo name's meaning no longer matters to users. | 2026-09-25 | REQ-035, [ADR-0005](../docs/adr/0005-project-identity.md) |
| Q-006 | Where does NetBox hold DNS data? | The NetBox DNS plugin. | 2026-09-25 | REQ-027, [ADR-0006](../docs/adr/0006-integrations-netbox-dns-plugin-and-powerdns-api.md) |
| Q-008 | Which PowerDNS Authoritative versions and backends are in use? | "Ideally, we'll use the PowerDNS API rather than direct interaction with zone information." The app is backend-agnostic and uses only the API. Versions are split out as Q-053. | 2026-09-25 | REQ-028, [ADR-0006](../docs/adr/0006-integrations-netbox-dns-plugin-and-powerdns-api.md) |
| Q-009 | Which topologies must be supported? | Primary → secondaries (including hidden primaries), and independent sites/clusters. Not in v1: shared-storage multi-writer, split-horizon views. | 2026-09-25 | REQ-029, REQ-030, [ADR-0007](../docs/adr/0007-server-groups-and-catalog-zones.md) |
| Q-010 | What are the sync semantics? | Drift policy per zone: enforce, report or ignore. Trigger and brownfield import are split out as Q-054. | 2026-09-25 | REQ-031, [ADR-0008](../docs/adr/0008-per-zone-drift-policy.md) |
| Q-019 | Persistence: SQLite, PostgreSQL, or both? | Both: PostgreSQL, and embedded SQLite for the single binary. | 2026-09-25 | REQ-032, [ADR-0009](../docs/adr/0009-persistence-and-high-availability.md) |
| Q-020 | Is HA with multiple replicas required? | Yes, on PostgreSQL: multiple replicas with one leader-elected sync worker. SQLite is single-instance. | 2026-09-25 | REQ-033, [ADR-0009](../docs/adr/0009-persistence-and-high-availability.md) |
| Q-047 | What is the commit model? | Claude commits per item on a milestone branch. The user reviews, merges and pushes. | 2026-09-25 | [ADR-0010](../docs/adr/0010-claude-commits-per-item-on-milestone-branches.md) |
| Q-048 | Is there a real lab Claude may reach? | No. The container lab only. | 2026-09-25 | REQ-036 |
| Q-052 | How do secondaries learn about zones created or deleted on the primary? | Catalog zones (RFC 9432). | 2026-09-25 | REQ-034, [ADR-0007](../docs/adr/0007-server-groups-and-catalog-zones.md) |
| Q-018 | Why build this rather than extend an existing tool? | Independent of NetBox upgrades; audit, SIEM and traceability; topologies and change safety. Recorded in the brief. | 2026-09-25 | [brief](brief.md#answers-2026-09-25-m0b-discovery-continued) |
| Q-044 | Which documentation platform? | Hugo. | 2026-09-25 | [ADR-0011](../docs/adr/0011-documentation-platform-hugo.md) |
| Q-045 | Who is the documentation for? | Default accepted: operators, API and IaC consumers, security and audit reviewers, contributors (Claude). | 2026-09-25 | [docs index](../docs/_index.md) |
| Q-046 | How are the docs hosted and versioned? | Default accepted: the site is a release artifact that can be hosted anywhere; the API docs are served by the binary; versioned docs from v1.0. | 2026-09-25 | [ADR-0011](../docs/adr/0011-documentation-platform-hugo.md) |
| Q-049 | Which CI runners? | Default accepted: the self-hosted GitLab Kubernetes privileged runners. A GitHub Actions wrapper is kept ready. | 2026-09-25 | ITEM-0006 |
| Q-055 | Which GitHub owner goes in the module path? | `zeddD1abl0`, from the `github` remote: `github.com/zeddD1abl0/netbox-powerdns-ai`. | 2026-09-25 | [ADR-0005](../docs/adr/0005-project-identity.md) |
| Q-056 | Which API style guide, and are versions put in URL paths? | The Zalando RESTful API Guidelines, followed in full, including rule 115: no version in URL paths. | 2026-09-25 | REQ-037, [ADR-0012](../docs/adr/0012-api-standard.md) |
| Q-007 | Which NetBox and plugin versions are supported, and how does the app authenticate to NetBox? | NetBox 4.7 and 4.6, with the DNS plugin 1.7.x and 1.6.x, read through the REST API. A read-only v2 token; a v1 token still works, with a warning. On 2026-10-06, narrowed to NetBox 4.7 only, since the CI runners can't fit a NetBox per release; more are added later. | 2026-09-27 | REQ-040, [ADR-0020](../docs/adr/0020-netbox-client-and-normalized-dns-model.md), [ADR-0023](../docs/adr/0023-netbox-client-and-normalized-dns-model-with-netbox.md) |
| Q-021 | How does the app reach PowerDNS: direct push, or agents on the DNS hosts? | Directly, through each primary's API, over HTTPS wherever it's behind TLS. No agents. | 2026-10-06 | REQ-041, [ADR-0024](../docs/adr/0024-read-powerdns-through-its-api-from-server-groups-i.md) |
| Q-022 | How is the PowerDNS API secured in transit? Its built-in webserver has no native TLS. | `http://` is allowed, with a warning. The reference setup is a TLS reverse proxy on the PowerDNS host, passing all methods, with an optional client certificate (mTLS); the webserver binds to `127.0.0.1` and the key is stored hashed. | 2026-10-06 | [ADR-0024](../docs/adr/0024-read-powerdns-through-its-api-from-server-groups-i.md) |
| Q-039 | How will the app be extended in future? | No backend interface or plugin runtime yet. The PowerDNS client returns the shared model, and M03, the first code using two sources, defines the interface it needs. A plugin runtime needs its own ADR. | 2026-10-06 | [ADR-0024](../docs/adr/0024-read-powerdns-through-its-api-from-server-groups-i.md) |
| Q-043 | Is a declarative config file (GitOps) needed? | Server groups are declared in the config file. When M08 stores resources in the database, those from the file become `managed_by=file`. | 2026-10-06 | [ADR-0024](../docs/adr/0024-read-powerdns-through-its-api-from-server-groups-i.md) |
| Q-053 | Which PowerDNS Authoritative versions must be supported? | 5.1 and 5.0. 4.9 reached end of life around September 2026. Later the same day, narrowed to 5.1 only, since the CI runners couldn't fit both. | 2026-10-06 | REQ-041, [ADR-0024](../docs/adr/0024-read-powerdns-through-its-api-from-server-groups-i.md), [ADR-0026](../docs/adr/0026-read-powerdns-through-its-api-with-powerdns-5-1-on.md) |
| Q-017 | What are the scale targets: servers, zones, records, change rate, propagation latency from NetBox to servers? | 1,000 zones and 100,000 records per run for now, tested and measured; raised when a deployment needs more. Propagation latency applies once nbpdns writes (M13). | 2026-10-06 | REQ-043, [ADR-0027](../docs/adr/0027-report-drift-between-netbox-and-each-server-group.md) |
| Q-027 | What happens when things fail? | For the command line (M03): if NetBox can't be read, the run fails; if a group's primary can't be read, that group is marked failed and the others are still compared, and the run fails as incomplete. Keeping the last-known state and alerting come with the service (M04) and the database (M08). | 2026-10-06 | [ADR-0027](../docs/adr/0027-report-drift-between-netbox-and-each-server-group.md) |
| Q-038 | Do "clearly defined metrics" cover this app only, or PowerDNS stats too? | This app's only: drift per server group and per drifted zone, refreshes, and requests to NetBox and PowerDNS, through `prometheus/client_golang`. PowerDNS's statistics stay on PowerDNS's own `/metrics`. Sync lag and plan size come with writes (M13), and serial lag on secondaries with M14. | 2026-10-07 | REQ-044, [ADR-0029](../docs/adr/0029-run-nbpdns-as-a-service-with-prometheus-metrics-an.md) |
| Q-025 | What packaging is needed? | For M05: static linux amd64 and arm64 binaries with SHA-256 checksums, and a multi-arch distroless static non-root image with an SBOM, built by GoReleaser and ko, and published to GitLab on version tags. Signing comes with M17, as do the Helm chart, systemd unit, compose example and air-gap bundle. Linux only. | 2026-10-07 | REQ-045, [ADR-0030](../docs/adr/0030-release-with-goreleaser-and-ko-to-gitlab-on-versio.md), [ADR-0031](../docs/adr/0031-debian-images-for-ci-with-a-distroless-static-runt.md) |
| Q-041 | What does IaC manage? With NetBox as the source of truth, DNS records go through NetBox's own Terraform provider. | nbpdns's own configuration (server groups, sync policies, sinks, roles, tokens and settings), plus reading DNS records through the API, for example for Terraform to read a zone's records and push them to other providers. Records are written only in NetBox. | 2026-10-08 | REQ-046, REQ-047, [ADR-0033](../docs/adr/0033-a-read-only-api-spec-first-generated-with-oapi-cod.md) |
