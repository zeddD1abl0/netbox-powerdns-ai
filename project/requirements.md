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
| REQ-026 | The project is self-contained and not tied to any forge. | Q-004, [ADR-0003](../docs/adr/0003-self-contained-forge-neutral-toolchain.md) |

## Open questions

**Blocking** questions must be answered before M1 is designed. **Needed by**
names the milestone that needs the answer, using the provisional milestone list
in [M00](milestones/M00-foundation.md).

### Product and domain

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-005 | **Blocking.** What are the product name, binary name, Go module path, env var prefix and license? Does "ai" in the repo name mean "built by AI" or "AI features in the product"? | To be decided. The module path is painful to change later. "ai" means built by AI. | M0 |
| Q-006 | **Blocking.** Where does NetBox hold DNS data: the NetBox DNS plugin (zones, records, views, nameservers, DNSSEC policies, IPAM DNSsync), core IPAM `dns_name`, or both? | The NetBox DNS plugin. IPAM-derived records come through the plugin's DNSsync, not logic of our own. | M1 |
| Q-007 | Which NetBox and plugin versions are supported, and how does the app authenticate to NetBox? | Current NetBox 4.x plus the previous minor, with a read-only API token. | M3 |
| Q-008 | **Blocking.** Which PowerDNS Authoritative versions and backends are in use (gpgsql/gmysql, LMDB with LightningStream, bind)? | 4.9 and 5.x. The backend decides where writes go. | M1 |
| Q-009 | **Blocking.** Which topologies must be supported: primary/secondary (AXFR/NOTIFY), DB replication, LightningStream, hidden primary, multi-site, split-horizon (NetBox DNS views mapped to server groups or PowerDNS 5 views), catalog zones? | A **server group** model: one write endpoint and N verify endpoints per group, with a view→group mapping. | M1 |
| Q-010 | **Blocking.** What are the sync semantics? Trigger, direction, drift policy, records in PowerDNS but not in NetBox, and importing existing PowerDNS zones into NetBox (brownfield). | A NetBox event-rule webhook plus a periodic full reconcile. One-way. Drift policy per zone: enforce, report or ignore. An import tool for first adoption. | M1 |
| Q-011 | Where does data that NetBox doesn't model live: TSIG keys, zone metadata (ALLOW-AXFR-FROM, ALSO-NOTIFY, SOA-EDIT-API), serial policy, DNSSEC key rollover? | In NetBox wherever the plugin models it. Everything else is per-zone or per-group config in this app. | M4 |
| Q-012 | Does "change settings" mean this app's settings only, or PowerDNS server config (`pdns.conf`) too? | This app's settings plus per-zone PowerDNS metadata. `pdns.conf` stays with Ansible. | M1 |
| Q-013 | What change safety is needed: dry-run diff, four-eyes approval, change windows, blast-radius limits, rollback? | Every sync computes a plan and auto-applies below thresholds. Above a threshold (such as more than N deletes, or NS/SOA changes) it needs approval. | M4 |
| Q-014 | What validation runs before and after changes? | Pre-flight checks (CNAME at apex, dangling NS, TTL bounds, syntax). After apply, query every server for the SOA serial and sample records. | M4 |
| Q-015 | Is multi-tenancy needed: are permissions scoped to zone, NetBox tenant or server group? | Global roles in v1, scoped by server group. Tenant scoping is a later ADR. | M2 |
| Q-016 | What is the web UI's scope? | Settings, ops dashboard (sync status, drift, per-server health), approvals, audit viewer, users and roles. **No record editor**, since NetBox is the editor. | M1 |
| Q-017 | What are the scale targets: servers, zones, records, change rate, propagation latency from NetBox to servers? | Needs an answer. A possible design target is 50 servers, 10k zones, 1M records, and under 60 s propagation. | M3 |
| Q-018 | Why build this rather than extend an existing tool? Prior art: ArnesSI/netbox-powerdns-sync, a NetBox plugin last supported on NetBox 3.6. | Record the differentiators in the brief: standalone, audit and SIEM, multiple topologies, API and IaC. | M0 |

### Architecture and deployment

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-019 | **Blocking.** Persistence: SQLite, PostgreSQL, or both? | PostgreSQL for containers and HA. Embedded SQLite for the single binary. One migration set, and CI runs against both. | M1 |
| Q-020 | **Blocking.** Is HA with multiple replicas required? | Active-active API with a leader-elected sync worker (Postgres advisory lock). SQLite mode is single-instance. | M1 |
| Q-021 | How does the app reach PowerDNS: direct push, or agents on the DNS hosts? | Direct HTTPS push. An agent mode is a later extension point. | M3 |
| Q-022 | How is the PowerDNS API secured in transit? Its built-in webserver has no native TLS. | A TLS proxy in front of it, with mTLS optional. Document the reference setup. | M3 |
| Q-023 | How are secrets stored at rest (PowerDNS API keys, TSIG, OIDC client secrets)? | Envelope encryption with a master key from env or file. Vault/OpenBao later. | M1 |
| Q-024 | When the UI and IaC both manage a setting, which one owns it? | A `managed_by` field on each resource. Resources owned by IaC are read-only in the UI. | M1 |
| Q-025 | What packaging is needed? | Static linux amd64/arm64 binary, distroless non-root image, Helm chart, systemd unit, compose example, air-gap bundle. Linux only. | M1 (binary, image), M6 (rest) |
| Q-026 | How are upgrades, backup and restore, and config export handled? | Forward-only migrations. `export` and `import` commands for app config. | M1 |
| Q-027 | What happens when things fail? | The app is **never in the DNS data path**. NetBox down: keep the last-known state and alert. PowerDNS server down: retry, mark the group degraded, report partial applies. | M3 |

### Identity and access

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-028 | Which IdP and protocols? | Authentik, with OIDC, SAML and trusted proxy headers, each toggled independently. No LDAP in v1. | M2 |
| Q-029 | Are local users, MFA and break-glass access needed? | Local users with TOTP/WebAuthn, plus an env-only break-glass token. Every use is audited. | M2 |
| Q-030 | What RBAC model? | Viewer, Operator and Admin built in. Custom roles. IdP group → role mapping. Permissions checked per action. | M2 |
| Q-031 | How do Terraform, Ansible and CI pipelines authenticate? | Scoped, expiring, hashed API tokens and service accounts. OIDC workload identity (CI JWTs) later. | M2 |
| Q-032 | Is SCIM provisioning needed? | Not in v1. | M2 |

### Audit, logging and SIEM

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-033 | Which SIEMs or log platforms must be supported (Splunk, Elastic, Sentinel, Wazuh, Graylog, Loki, QRadar)? | Sinks: stdout JSON, file, syslog RFC 5424 over TLS, HTTP (HEC-compatible), OTLP logs. | M5 |
| Q-034 | What wire format? | Native versioned JSON first. OCSF and CEF mappings are ADR candidates. | M5 |
| Q-035 | What audit coverage, retention and tamper evidence are needed, and what happens when the SIEM is down? | All auth, config, RBAC, token, plan, apply, drift and approval events. Hash-chained. Retention configurable. An outbox buffers events, with an alert on backlog. | M1 |
| Q-036 | Which compliance frameworks apply (ISO 27001, SOC 2, PCI DSS, Essential Eight/ISM, NIS2)? | Needs an answer. It affects retention, crypto and MFA. | M1 |
| Q-037 | How far does traceability go? | End to end: carry NetBox's changelog identity (user, `request_id`, ObjectChange ID) through sync → apply → each server → verification, so one trace links the NetBox edit to every PowerDNS write. | M3 |

### API, metrics and extensibility

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-038 | Do "clearly defined metrics" cover this app only, or PowerDNS stats too? | App metrics: sync lag, plan size, failures, drift count, per-server serial lag, API RED metrics. PowerDNS stats stay on PowerDNS's own `/metrics`. | M1 |
| Q-039 | How will the app be extended in future? | A backend interface (for other DNS vendors later), outbound webhooks and an event stream. A plugin runtime is deferred to an ADR. | M1 |
| Q-040 | Are rate limits and quotas needed? | Limits per token and per IP. | M2 |

### IaC

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-041 | What does IaC manage? With NetBox as the source of truth, DNS records go through NetBox's own Terraform provider. | IaC for this app covers **its own config**: server groups, sync policies, sinks, roles, tokens and settings. | M1 |
| Q-042 | Where do the Terraform/OpenTofu provider and the Ansible collection live, and which registries do they publish to? | Separate repos in later milestones, built with terraform-plugin-framework. The API is designed for them from M1: declarative, idempotent, stable IDs, import support. | M7 |
| Q-043 | Is a declarative config file (GitOps) needed? | Resources can be declared in the config file and are then `managed_by=file`. | M1 |

### Documentation

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-044 | Which documentation platform? | Hugo: a Go binary pinned like the other tools, with a theme that needs no Node build (Hextra or hugo-book, chosen by spike). GitHub alerts and Mermaid render on the site and raw on both forges. Alternative: Zensical, with a containerised Python toolchain. | M0 |
| Q-045 | Who is the documentation for? | Operators, API and IaC consumers, security and audit reviewers, contributors (Claude). | M0 |
| Q-046 | How are the docs hosted and versioned? | The site is a release artifact that can be hosted anywhere. The API docs are served by the binary. Versioned docs from v1.0. | M0 |

### Process and environment

| ID | Question | Proposed default | Needed by |
|---|---|---|---|
| Q-047 | **Blocking.** What is the commit model? | Same as go-redbarkwebhook: **the user commits at milestone boundaries**, with Conventional Commit messages that Claude drafts. | M0 |
| Q-048 | **Blocking.** Is there a real NetBox and PowerDNS lab that Claude may reach, and with what credentials? | `deploy/dev/compose.yaml` with NetBox, the DNS plugin, and a three-node PowerDNS (primary, secondary, LightningStream pair) for local end-to-end tests. | M1 |
| Q-049 | Which CI runners? | The existing self-hosted GitLab Kubernetes privileged runners, with Docker-in-Docker for integration tests. A GitHub Actions wrapper is kept ready but unused. | M0 |
| Q-050 | Which UI technology? | Server-rendered templ + htmx with vendored assets and no Node build, embedded in the binary. | M1 |
| Q-051 | What accessibility and localisation level? | WCAG 2.2 AA. English only. | M1 |

## Answered

| ID | Question | Answer | Date | Produced |
|---|---|---|---|---|
| Q-001 | Which DNS server software must the first release manage? | PowerDNS Authoritative only. | 2026-09-25 | REQ-023, [ADR-0004](../docs/adr/0004-v1-scope-netbox-source-of-truth-powerdns-auth.md) |
| Q-002 | What is NetBox's role relative to this application? | NetBox is the source of truth. | 2026-09-25 | REQ-024, [ADR-0004](../docs/adr/0004-v1-scope-netbox-source-of-truth-powerdns-auth.md) |
| Q-003 | Where should the project's work be tracked? | In-repo Markdown only. | 2026-09-25 | REQ-025, [ADR-0002](../docs/adr/0002-track-work-in-repo.md) |
| Q-004 | Which documentation platform? | Platform not chosen; the answer set a constraint instead. The project must not be GitLab-only and should be as self-contained as possible. The platform question continues as Q-044. | 2026-09-25 | REQ-026, [ADR-0003](../docs/adr/0003-self-contained-forge-neutral-toolchain.md) |
