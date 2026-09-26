---
id: M00
title: Foundation
status: done # planned | in-progress | done
started: 2026-09-25
closed: 2026-09-26
---

# M00: Foundation

## Goal

Set up everything needed to develop the project in a way that's repeatable and
expandable, before writing any product code:
- the working rules;
- work tracking;
- decision records;
- the docs skeleton;
- a pinned toolchain;
- CI;
- answers to the questions that block M1.

## Non-goals

- Product code of any kind. That starts in M1.
- Answering every open question. Only the **blocking** ones in
  [requirements.md](../requirements.md#open-questions) are needed to close M0.
  The rest are answered by the milestone that needs them.

## Phases

| Phase | Scope | Items |
|---|---|---|
| M0a | Process scaffolding (no Go code), commit model | ITEM-0001, ITEM-0012 |
| M0b | Discovery: answer the blocking questions and record ADRs | ITEM-0002, ITEM-0003, ITEM-0009, ITEM-0010 |
| M0c | Toolchain | ITEM-0004 to ITEM-0008 |
| Close | Cold-start test, review, user merges the branch | ITEM-0011 |

## Acceptance criteria

- [x] `CLAUDE.md`, `project/`, `docs/adr/` and the docs skeleton exist (ITEM-0001).
- [x] Every blocking question in `requirements.md` is answered (ITEM-0002; Q-055 answered 2026-09-25).
- [x] ADRs exist for the docs platform, the API standard, and persistence and HA (ITEM-0009: ADR-0011, ADR-0012, ADR-0009).
- [x] `make check` and `make ci` pass locally with only the ADR-0014 prerequisites installed, and in the pinned CI image.
- [x] The GitLab CI pipeline passes by calling make targets only (the staged pipeline at `1a68497`, reported by the user 2026-09-25).
- [x] `projctl lint` fails on a deliberately broken item and passes on a clean tree (26 lint cases in `tools/projctl`; `make project-lint` is clean).
- [x] The docs site builds. An alert and a Mermaid block render on the site (verified) and raw on GitLab (reported by the user 2026-09-26).
- [x] Cold-start test passed (ITEM-0011): two runs, 4 files read each.
- [x] The milestone list M1 to M8 is finalised, with stub files for M01 to M08 (ITEM-0010).
- [ ] The user has merged `m00-foundation` (creating `main` from it, since `main` is unborn).
- [x] Per-item commit model adopted (ITEM-0012, ADR-0010).

## Verification log

Append-only and dated. Record what was run and what was seen.

- 2026-09-25: the `.claude/settings.json` gofmt hook was pipe-tested (a `.go`
  file was reformatted, a `.md` file was left untouched) and confirmed live in
  session: a badly formatted `.go` file was reformatted immediately after being
  written.
- 2026-09-25: The `.claude/hooks/guard-main-commit.sh` PreToolUse guard was
  pipe-tested against a scratch repo on `main`. It denied `git commit -m`, a
  compound `cd && git commit`, and `git -C … commit`. It allowed `git status`
  and `echo git committed`, and allowed `git commit` on `m00-foundation`. A
  temporary sentinel confirmed it fires live on every Bash call.
- 2026-09-25: M0c toolchain verified.
  - `make ci` passes on the host. It also passes inside the pinned CI image
    (`golang:1.27.1@sha256:3680233e…`), run as a non-root user with
    `GOTOOLCHAIN=local`: 1m49s with cold caches, 41s with warm ones. It covers
    vet, lint, test, vuln, secrets, docs-lint, api-lint, project-lint, and the
    docs build with its link check.
  - The first cold-cache run found a bug, since fixed: `docs-lint` counted
    stderr build output as findings.
  - Negative checks pass:
    - a banned word fails `docs-lint`;
    - a broken Zalando rule fails `api-lint`;
    - a broken anchor and a missing page fail `docs-links`;
    - 26 broken-fixture cases fail `projctl lint`.
  - The docs site builds offline, with `GOPROXY=off` and an empty Hugo cache.
  - The goimports format hook was proven live.
- 2026-09-25: M0 close checks.
  - **Cold-start test (ITEM-0011): passed twice.** Fresh read-only subagents
    read 4 files in `CLAUDE.md`'s order and named M00, 11 of 12 items closed,
    and ITEM-0011 as next. Their confusion points were fixed: a superseded-rules
    callout above the approved design, and a precedence rule in `CLAUDE.md`.
  - **`/code-review high`:** 10 findings, each checked against the code and all
    fixed in ITEM-0013. One of them (projctl scanning GitLab's in-checkout
    `.cache/`) would have failed every GitLab pipeline.
  - **`/security-review`: not run.** M0 added no authentication, audit, secrets
    or crypto code, which is the Definition of Done's trigger.
  - **GitLab job emulated:** the pinned image, caches under `/src/.cache` as
    `.gitlab-ci.yml` sets them, cold start. `make ci` passed in 3m30s, with
    1,549 third-party `.md` files in `.cache/` correctly ignored.
  - **Still waiting on the user:** the real GitLab and GitHub pipeline results,
    and the raw-rendering check on GitLab, both after the branch is pushed.
- 2026-09-25: The first real GitLab job was **evicted**: the node ran out of
  ephemeral storage. Building eight tools from source needed about 6.5 GB.
  - Fixed by ITEM-0014 (pinned release binaries, ADR-0014) and ITEM-0015
    (staged CI, ADR-0016).
  - Each of the eight jobs now passes cold in the pinned image in 22–182 MB
    and 2–14 seconds.
  - Node sizing advice given to the user: more ephemeral storage for CI
    nodes, and ephemeral-storage requests and limits on build pods. M1's
    container lab will need more disk than lint jobs.
- 2026-09-25: The user pushed `1a68497` to both remotes, and the first staged
  pipelines are running. The results, and the next steps for closing M00, are
  in ITEM-0011's notes.
- 2026-09-25: The user reported that the pipeline run on `1a68497` passed on
  the GitLab runners, without changing the pods' disk size. That's the run
  that was evicted earlier as a single 6.5 GB job. No GitHub result was
  reported separately.
- 2026-09-26: The user reported two results:
  - the GitHub Actions pipeline for `m00-foundation` passed;
  - GitLab's file view renders the NOTE alert in `docs/_index.md` as a styled
    callout, and draws the Mermaid diagram in ADR-0007.
- 2026-09-26: **M0 close.**
  - **Second `/code-review high`** on the ITEM-0014 and ITEM-0015 code: 10
    findings. Nine were fixed in ITEM-0016, and one was deliberately left:
    the GitHub workflow runs twice for a same-repo PR, but GitHub is a push
    mirror. ADR-0017 restates ADR-0011 with the current toolchain.
    ITEM-0017 (M01) will automate the hook pipe-tests.
  - **`/security-review`: not run.** M00 added no authentication, audit,
    secrets or crypto code. The tool fetcher checks SHA-256 pins with
    `sha256sum`, but implements no crypto.
  - **`make check`** on `0da571a`: vet, lint (0 issues), test, vuln (none),
    secrets (no leaks), docs-lint (no findings), api-lint (ruleset self-test
    passed) and project-lint all passed.
  - **Manual verification**, in a fresh clone with an empty `.cache/` and
    empty Go module and build caches:
    1. `make help` lists the targets by group.
    2. `make ci` passed in 41 s from fully cold, including downloading Go
       1.27.1 and fetching the 6 pinned tools.
    3. `make item TITLE="probe"`, then `make project` and
       `make project-lint`: lint passed, and `git status` showed only the
       new ITEM-0018 file and the regenerated board.
  - **Integration tests:** none. M00 has no NetBox, PowerDNS or database
    code.
  - **Still to do after hand-over:** the user pushes the branch and confirms
    both pipelines, then creates `main` from it. The criterion "The user has
    merged `m00-foundation`" is ticked, with the resulting commit, in the
    first commit on `m01-service-skeleton`.

## Approved design

The plan approved on 2026-09-25, copied verbatim. Its headings are demoted two
levels to nest under this section; the text is unchanged. It's a snapshot. For
the live state of the open questions, see
[requirements.md](../requirements.md), which numbers the gaps below as
Q-005 to Q-051.

> [!IMPORTANT]
> These points in the snapshot have since been superseded. Follow the ADRs:
>
> - **Commits:** Claude commits each item on the milestone branch, and the
>   user merges ([ADR-0010](../../docs/adr/0010-claude-commits-per-item-on-milestone-branches.md)).
>   The user no longer commits, and `git commit` isn't set to ask. `git push`
>   still asks.
> - **CHANGELOG:** only user-facing changes get a line (`CLAUDE.md`), not every
>   item.
> - **API versioning:** paths carry no version, not `/api/v1`
>   ([ADR-0012](../../docs/adr/0012-api-standard.md)).
> - **Prerequisites and tools:** glibc Linux amd64 with Go, Docker, make,
>   curl, tar and sha256sum; tools pinned as release binaries by SHA-256 in
>   `tools/tools.mk`
>   ([ADR-0014](../../docs/adr/0014-toolchain-pinned-release-binaries-on-glibc-linux.md),
>   which superseded ADR-0013 and ADR-0003).
> - **Docs platform:** Hugo with the Hextra theme
>   ([ADR-0017](../../docs/adr/0017-documentation-platform-hugo-restated-for-the-relea.md),
>   which restated ADR-0011; ITEM-0008).

### netbox-powerdns-ai: project foundation plan

#### Context

`netbox-powerdns-ai` is an empty git repo (branch `main`, no commits). Claude will develop the whole project. The brief describes a Go service that manages DNS servers and needs:
- SSO and user management;
- audit and SIEM export;
- a documented API with metrics;
- IaC-driven configuration;
- single-binary and container deployment.

Before any product code is written, the user wants three things:
1. guidelines;
2. a way to track work;
3. a list of everything the brief leaves open.

This plan covers all three. Approving it starts **M0 Foundation**. M0 sets up process scaffolding and runs a discovery pass. It writes no product code.

##### Decisions already made (2026-09-25)

| Topic | Decision |
|---|---|
| DNS scope for v1 | **PowerDNS Authoritative** only. |
| NetBox role | **NetBox is the source of truth.** This app renders NetBox's DNS data, pushes it to PowerDNS, detects drift and audits every step. |
| Work tracking | **In-repo Markdown only.** |
| Portability | **Self-contained and not tied to one forge.** The code is on GitLab today, but a copy may live on GitHub later. Nothing may depend on forge features: issues, wiki, Pages, or CI-specific logic. |

##### Lessons carried over from go-redbarkwebhook

These are the user's existing conventions and where they became a problem:
- **What worked:**
  - milestones (M0, M1, …);
  - plan mode for design, auto mode for implementation, then back to plan mode;
  - **the user makes the commit at each milestone boundary**;
  - decisions recorded with dates;
  - a CI-enforced docs drift check.
- **What hurt:**
  - `README.md` reached 76 KB and `docs/decisions.md` reached 104 KB, and both were read every session;
  - archiving meant moving text verbatim, which needed its own rulebook and a linter (`docsdrift`).
- **Fix used here:**
  - many small files that never move;
  - work state lives in front matter;
  - indexes are generated, not maintained by hand.

---

#### 1. Guidelines

##### 1.1 Principles
1. **Self-contained.** The only prerequisites are Go, Docker and make.
   - Every other tool is pinned in the repo. Go tools use `tool` directives in `tools/go.mod`; anything else runs from a container image pinned by digest.
   - `make ci` runs the whole pipeline locally.
   - `.gitlab-ci.yml` and `.github/workflows/ci.yml` only call make targets.
   - UI and API-docs assets are vendored, with no CDN. The app works air-gapped, which matters for DNS infrastructure.
2. **One source, generated references.** These are each declared once in code and generated into docs, so the docs can't drift:
   - config keys;
   - metrics;
   - audit event types;
   - permissions;
   - API endpoints (in `api/openapi.yaml`).
3. **Spec first.** An API change starts in `api/openapi.yaml`. The server stubs are generated from it and responses are contract-tested against it.
4. **Small files that never move.** Work items, ADRs and milestone specs each get their own file. Closing one changes its status and leaves the file where it is.
5. **Docs ship with code.** A change isn't done until its docs, generated references and CHANGELOG entry are in the same change.

##### 1.2 Engineering standards

| Area | Standard | Enforced by |
|---|---|---|
| Go | Google Go Style Guide. Everything under `internal/`. Stdlib first. Each new dependency justified; big ones get an ADR. License allowlist: MIT, BSD, Apache-2.0, MPL-2.0. | golangci-lint v2, `go vet`, license check |
| API | OpenAPI 3.1, spec first. **Zalando RESTful API Guidelines** as the style guide. Errors: **RFC 9457** problem+json. Versioning: `/api/v1`. Updates: ETag/If-Match. Pagination: cursor-based. Creates: `Idempotency-Key`. Deprecation: `Deprecation`/`Sunset` headers. Deviations go in an ADR. | vacuum (Go, runs Spectral rulesets) |
| Config | Precedence: defaults < file < env < flags. Runtime settings live in the DB and are editable in the UI and API unless env or file pins them; the UI shows where each value came from. Bootstrap keys (DB, listen address, master key) are never editable on the web. Every secret has a `*_FILE` variant. | typed config registry → generated `reference/config.md` |
| Logs | `log/slog` JSON. Every line carries `trace_id` and `request_id`. Secrets are wrapped in a redacting type. The audit stream is kept separate from the operational log. | tests plus a lint rule |
| Metrics and tracing | Prometheus/OpenMetrics naming best practice. OpenTelemetry traces with W3C `traceparent`, propagated into outbound PowerDNS and NetBox calls. | metric registry → generated `reference/metrics.md` |
| Audit | Append-only and hash-chained. Each event records actor, action, target, before and after values, reason, `trace_id`, source IP and outcome. Delivery to SIEMs uses a transactional outbox, at least once. The event schema is versioned. | event registry → generated `reference/audit-events.md` |
| Security | Target **OWASP ASVS 5.0 Level 2**. A STRIDE threat model is kept in `docs/explanation/`. Secrets are encrypted at rest. | govulncheck, gitleaks, Trivy; syft SBOM and cosign signing at release |
| Tests | Table-driven unit tests. Integration tests against **real PowerDNS Auth and NetBox in containers**. API contract tests. `-race` always. The coverage floor is set by ADR. | `make test`, `make test-integration` |
| Versioning | SemVer. Conventional Commits. Keep a Changelog. GoReleaser builds linux amd64 and arm64 binaries plus multi-arch images, and can publish to either forge. | commit-msg check, `make release-dry` |
| Docs | **Diátaxis** structure: tutorials, how-to, reference, explanation. **Google developer documentation style guide**, checked by Vale (a Go tool). Plain Markdown that reads well raw on GitHub and GitLab. | Vale, link checker, generated-files-current check |

##### 1.3 Documentation platform (recommendation, confirmed by ADR in M0)
Recommendation: **Hugo**, a Go binary pinned like the other tools. It needs no Python or Node, which fits the self-contained rule.
- **Theme:** one that needs no Node build. Candidates are Hextra and hugo-book; M0 includes a short spike to choose between them.
- **Callouts:** GitHub-style alerts (`> [!NOTE]`), rendered by Hugo render hooks. GitHub and GitLab render them natively too.
- **Diagrams:** Mermaid code blocks, which also render on both forges and on the site.
- **Tabs:** shortcodes, used only where plain Markdown has no equivalent. The allowed set is listed in the docs style page.
- **API reference:** an OpenAPI viewer with vendored assets, **served by the binary itself** at `/api/docs`. Every running version then carries its own matching reference.
- **Hosting:** the built site is a release artifact that can be hosted anywhere.

The alternative is Zensical, the successor to Material for MkDocs, which is now in maintenance mode. It has a richer look, but it needs a containerised Python toolchain, and its `!!!` admonitions don't render in raw Markdown.

##### 1.4 Definition of Done
**Every item:**
- code and tests are done;
- `make check` is green (fmt, lint, vet, test -race, vuln, OpenAPI lint, docs lint, `projctl lint`);
- regenerated references are committed;
- the relevant docs page is updated;
- there is a CHANGELOG line;
- the item's status is updated.

**Every milestone:**
- all its items are done;
- `/code-review high` has run, and `/security-review` too when auth, audit or secrets were touched;
- the manual verification steps are recorded in the milestone file;
- then **the user commits**.

##### 1.5 Claude Code setup (committed to the repo)
- **`CLAUDE.md`** stays at 150 lines or fewer. It holds:
  - the read order;
  - the non-negotiable rules;
  - the commands;
  - the DoD;
  - the workflow.

  It routes to other files and never holds project state.
- **Nested `CLAUDE.md` files** hold local conventions. Examples: `internal/sync/CLAUDE.md`, `api/CLAUDE.md`. They load only when Claude works in that folder.
- **`.claude/settings.json`**:
  - allow `make *`, `go test/build/vet/run/tool`, `git status/diff/log` and `docker build/compose`;
  - `git commit` and `git push` set to **ask**, since the user commits;
  - a PostToolUse hook that runs `gofmt`/`goimports` on every edited `.go` file.
- **Project skills** in `.claude/skills/` make repeated tasks work the same way every time:
  - `new-item`, `new-adr` and `close-milestone` in M0;
  - `new-endpoint` in M1 (spec → generate → handler → contract test → docs).
- **Workflow:** carried over from go-redbarkwebhook. Each milestone is designed in plan mode, and the approved plan is **copied into `project/milestones/Mxx-*.md`** so the design lives in the repo. It is then implemented in auto mode and verified, and Claude returns to plan mode. The user commits. Milestone N doesn't start until M(N-1) is complete and committed.
- **Memory:** this project's preferences (the workflow, who commits, self-contained) get saved as project memories.

---

#### 2. Work tracking (in the repo)

```
project/
  README.md            GENERATED board: milestones, open items by status, REQ coverage. Never hand-edited.
  brief.md             The original brief verbatim, plus dated answers. Only appended to.
  requirements.md      Numbered requirements (REQ-001…) and open questions (Q-001…)
  milestones/M01-service-skeleton.md   Goal, non-goals, approved design, acceptance, verification log
  items/ITEM-0001-oidc-login.md        One file per feature, bug, debt or task
docs/adr/0001-record-architecture-decisions.md   MADR format; accepted ADRs are superseded, never edited
```

Each item file carries front matter: `id`, `title`, `type` (feature/bug/debt/task), `status` (open/in-progress/blocked/done/wontfix), `milestone`, `requirements: [REQ-…]`, `created` and `closed`. The body has these sections:
- Goal;
- Acceptance criteria (checkboxes);
- Notes (append-only, dated).

`tools/projctl` is a small Go tool with three commands:
- **`new`** scaffolds an item or ADR with the next free ID.
- **`index`** regenerates `project/README.md`.
- **`lint`** checks:
  - IDs are unique;
  - front matter is valid;
  - milestones and REQs exist;
  - a done milestone has no open items;
  - generated files are current;
  - links resolve.

`lint` runs in `make check`. It replaces `docsdrift` with a much simpler rule set, because nothing ever moves.

**Traceability chain:** REQ → ITEM → commit (`Refs: ITEM-0012` trailer) → test → release note. It stays in git and survives a move to another forge.

---

#### 3. Gaps in the brief

Each gap has a proposed default. **B** marks the gaps I need answered before M1 design; the rest can wait until their milestone.

##### A. Product and domain
| # | Gap | Proposed default |
|---|---|---|
| A1 **B** | Product name, binary name, Go module path, env prefix, license. Does "ai" mean built by AI, or AI features in the product? | Module path to be decided (it's painful to change later). "ai" means built by AI. |
| A2 **B** | Where does NetBox hold DNS data: the **NetBox DNS plugin** (zones, records, views, nameservers, DNSSEC policies, IPAM DNSsync), core IPAM `dns_name`, or both? | NetBox DNS plugin. IPAM-derived records come through the plugin's DNSsync, not logic of our own. |
| A3 | Supported NetBox and plugin versions, and how the app authenticates to NetBox | Current NetBox 4.x plus the previous minor version, with a read-only API token |
| A4 **B** | PowerDNS Auth versions and backends in use (gpgsql/gmysql, LMDB with LightningStream, bind) | 4.9 and 5.x. Backend decides where writes go. |
| A5 **B** | Topologies: primary/secondary (AXFR/NOTIFY), DB replication, LightningStream, hidden primary, multi-site, split-horizon (NetBox DNS views mapped to server groups or PDNS 5 views), catalog zones | A **server group** model: one write endpoint and N verify endpoints per group, with a view→group mapping |
| A6 **B** | Sync semantics: trigger, direction, drift policy, records in PDNS but not in NetBox, **brownfield import** of existing PDNS zones into NetBox | NetBox event-rule webhook plus a periodic full reconcile. One-way. Drift policy per zone: enforce, report or ignore. An import tool for first adoption. |
| A7 | Data NetBox doesn't model: TSIG keys, zone metadata (ALLOW-AXFR-FROM, ALSO-NOTIFY, SOA-EDIT-API), serial policy, DNSSEC key rollover | Use NetBox wherever the plugin models it; everything else is per-zone or per-group config in this app |
| A8 | "Change settings": this app's settings only, or PowerDNS server config (`pdns.conf`) too? | This app's settings plus per-zone PDNS metadata. `pdns.conf` stays with Ansible. |
| A9 | Change safety: dry-run diff, approval (four-eyes), change windows, blast-radius limits, rollback | Every sync computes a plan and auto-applies below thresholds. Above a threshold (such as more than N deletes, or NS/SOA changes) it needs approval. |
| A10 | Validation before and after changes | Pre-flight checks (CNAME at apex, dangling NS, TTL bounds, syntax). After apply, query every server for the SOA serial and sample records. |
| A11 | Multi-tenancy: are permissions scoped to zone, NetBox tenant or server group? | Global roles in v1, scoped by server group. Tenant scoping is an ADR later. |
| A12 | Web UI scope | Settings, ops dashboard (sync status, drift, per-server health), approvals, audit viewer, users and roles. **No record editor**, since NetBox is the editor. |
| A13 | Scale targets: servers, zones, records, change rate, NetBox-to-servers latency | Ask. The design target might be 50 servers, 10k zones, 1M records, and under 60 s propagation. |
| A14 | Why build rather than extend? Prior art: ArnesSI/netbox-powerdns-sync, a NetBox plugin last supported on NetBox 3.6. | Record the differentiators in the brief: standalone, audit and SIEM, multiple topologies, API and IaC. |

##### B. Architecture and deployment
| # | Gap | Proposed default |
|---|---|---|
| B1 **B** | Persistence: SQLite, PostgreSQL, or both | PostgreSQL for containers and HA. Embedded SQLite for the single binary. One migration set, and CI runs against both. |
| B2 **B** | HA: multiple replicas? | Active-active API with a leader-elected sync worker (Postgres advisory lock). SQLite mode is single-instance. |
| B3 | How the app reaches PowerDNS: direct push, or agents on the DNS hosts | Direct HTTPS push. An agent mode is a later extension point. |
| B4 | PDNS API transport security. The PDNS webserver has no native TLS. | A TLS proxy in front of it, with mTLS optional. Document the reference setup. |
| B5 | Secrets at rest (PDNS API keys, TSIG, OIDC secrets) | Envelope encryption with a master key from env or file. Vault/OpenBao later. |
| B6 | UI edits vs. IaC: who owns a setting? | `managed_by` on each resource. Resources owned by IaC are read-only in the UI. |
| B7 | Packaging | Static linux amd64/arm64 binary, distroless non-root image, Helm chart, systemd unit, compose example, air-gap bundle. Linux only. |
| B8 | Upgrades, backup and restore, config export | Forward-only migrations. `export`/`import` commands for app config. |
| B9 | Failure behaviour | The app is **never in the DNS data path**. NetBox down: keep the last-known state and alert. PDNS server down: retry, mark the group degraded, report partial applies. |

##### C. Identity and access
| # | Gap | Proposed default |
|---|---|---|
| C1 | IdP and protocols | **Authentik** (as in go-redbarkwebhook) with OIDC, SAML and trusted proxy headers, each toggled independently. LDAP not in v1. |
| C2 | Local users, MFA, break-glass | Local users with TOTP/WebAuthn, plus an env-only break-glass token. Every use is audited. |
| C3 | RBAC | Viewer, Operator and Admin built in. Custom roles. IdP group → role mapping. Permissions checked per action. |
| C4 | Machine identity for Terraform, Ansible and CI | Scoped, expiring, hashed API tokens and service accounts. OIDC workload identity (GitLab/GitHub CI JWT) later. |
| C5 | SCIM provisioning | Not in v1. |

##### D. Audit, logging and SIEM
| # | Gap | Proposed default |
|---|---|---|
| D1 | Which SIEMs? (Splunk, Elastic, Sentinel, Wazuh, Graylog, Loki, QRadar) | Sinks: stdout JSON, file, syslog RFC 5424 over TLS, HTTP (HEC-compatible), OTLP logs |
| D2 | Wire format | Native versioned JSON first. OCSF and CEF mappings are ADR candidates. |
| D3 | Audit coverage, retention, tamper evidence, behaviour when the SIEM is down | All auth, config, RBAC, token, plan, apply, drift and approval events. Hash chain. Retention configurable. Outbox buffer, with an alert on backlog. |
| D4 | Compliance frameworks (ISO 27001, SOC 2, PCI DSS, Essential Eight/ISM, NIS2) | Ask. The answer affects retention, crypto and MFA. |
| D5 | **End-to-end traceability** | Carry NetBox's changelog identity (user, `request_id`, ObjectChange ID) through sync → apply → each server → verification. One trace links the NetBox edit to every PDNS write. |

##### E. API, metrics and extensibility
| # | Gap | Proposed default |
|---|---|---|
| E1 | "Clearly defined metrics": this app only, or PowerDNS stats too? | App metrics: sync lag, plan size, failures, drift count, per-server serial lag, API RED metrics. PDNS stats stay on PDNS's own `/metrics`. |
| E2 | Future extension | A backend interface (other DNS vendors later), outbound webhooks and an event stream. The plugin runtime is deferred to an ADR. |
| E3 | Rate limits and quotas | Limits per token and per IP |

##### F. IaC
| # | Gap | Proposed default |
|---|---|---|
| F1 | IaC scope. With NetBox as source of truth, DNS records go through NetBox's own Terraform provider. | IaC for this app covers **its own config**: server groups, sync policies, sinks, roles, tokens, settings. |
| F2 | Terraform/OpenTofu provider and Ansible collection: which repo, which registry | Separate repos in later milestones, built with terraform-plugin-framework. The API is designed for them from M1: declarative, idempotent, stable IDs, import support. |
| F3 | A declarative config file (GitOps) | Resources can be declared in the config file and are then `managed_by=file`. |

##### G. Docs
| # | Gap | Proposed default |
|---|---|---|
| G1 | Platform | Hugo (§1.3), confirmed by an ADR in M0 |
| G2 | Audiences | Operators, API and IaC consumers, security and audit reviewers, contributors (Claude) |
| G3 | Hosting and versioning | Site as a release artifact. API docs served by the binary. Versioned docs from v1.0. |

##### H. Process and environment
| # | Gap | Proposed default |
|---|---|---|
| H1 **B** | Commit model | Same as go-redbarkwebhook: **the user commits at milestone boundaries**, with Conventional Commit messages that Claude drafts. |
| H2 **B** | Test lab: is there a real NetBox and PowerDNS that Claude may reach, and with what credentials? | `deploy/dev/compose.yaml` with NetBox, the DNS plugin, and a 3-node PDNS (primary, secondary, LightningStream pair) for local end-to-end tests |
| H3 | CI runners | The existing self-hosted GitLab k8s privileged runners (Docker-in-Docker for integration tests). A GitHub wrapper is ready but unused. |
| H4 | UI technology | Server-rendered **templ + htmx** with vendored assets and no Node build, embedded in the binary |
| H5 | Accessibility and i18n | WCAG 2.2 AA. English only. |

---

#### 4. Proposed milestones (provisional, finalised at the end of M0)
Each milestone ships its own UI, docs and audit events. There is no separate "UI milestone" or "docs milestone".

| M | Scope |
|---|---|
| **M0** | Foundation: process scaffolding, discovery, ADRs (this plan) |
| M1 | Service skeleton: config registry, slog, metrics, health, DB and migrations, OpenAPI pipeline, audit core, binary and image, dev compose lab |
| M2 | Identity: local users, OIDC/SAML/proxy, RBAC, API tokens, sessions |
| M3 | Read path: NetBox and PDNS clients, server groups, **read-only drift report** (the first safe value) |
| M4 | Write path: plan/apply, guards and approvals, post-apply verification, webhook plus reconcile, brownfield import |
| M5 | SIEM: sinks, hash chain, outbox, event catalogue |
| M6 | Production hardening: HA, Helm, backup and restore, threat model review, load test |
| M7 | Terraform/OpenTofu provider (separate repo) |
| M8 | Ansible collection (separate repo), then v1.0 |

---

#### 5. What happens on approval (M0)

**M0a: process scaffolding.** No Go code yet, because it needs the A1 module path.
1. `CLAUDE.md`, and a front-door `README.md` stub.
2. `project/brief.md`: the brief verbatim plus the 2026-09-25 answers.
3. `project/requirements.md`: REQs from the brief, and **Q-001…** for every gap in §3, with its default.
4. `project/milestones/M00-foundation.md` holding this plan, and `project/items/` for the M0 tasks.
5. `docs/adr/`:
   - a template;
   - ADR-0001: record decisions;
   - ADR-0002: in-repo tracking;
   - ADR-0003: self-contained, forge-neutral toolchain;
   - ADR-0004: NetBox as source of truth, PowerDNS Auth scope.
6. `docs/` Diátaxis skeleton with index pages.
7. `.claude/settings.json` and the skills `new-item`, `new-adr` and `close-milestone`. Also `.gitignore`, `.editorconfig`, `CHANGELOG.md`.
8. Save project memories: workflow, commit model, self-contained rule.

**M0b: discovery (plan mode).** Work through the Q list section by section. Each answer updates `requirements.md` or becomes an ADR, starting with the **B** items.

**M0c: toolchain.** Needs A1.
- `go.mod`, and `tools/go.mod` with the pinned tools;
- `Makefile` with `check`, `ci`, `docs`, `project`, `test-integration`;
- `tools/projctl`;
- `.golangci.yml`, the Vale config (Google style), the vacuum ruleset (Zalando);
- the Hugo site with the theme spike;
- thin `.gitlab-ci.yml` and `.github/workflows/ci.yml` files;
- ADRs for the docs platform, the API standard, and persistence and HA.

#### 6. Verification
- `make check` and `make ci` pass locally with only Go, Docker and make installed, and `.gitlab-ci.yml` passes on the GitLab runner.
- `projctl lint` fails on a deliberately broken item (duplicate ID, unknown milestone, stale index) and passes after `projctl index`.
- The docs site builds. A page using an alert and a Mermaid block renders on the site **and** raw on GitLab.
- **Cold-start test:** a fresh Claude session, told only "read CLAUDE.md, then give me the current status and the next item", answers correctly without reading more than about 5 files. This checks that the setup can be repeated.
- The user reviews and commits M0.

Sources: [Zensical announcement](https://squidfunk.github.io/mkdocs-material/blog/2025/11/05/zensical/) · [ArnesSI/netbox-powerdns-sync](https://github.com/ArnesSI/netbox-powerdns-sync)
