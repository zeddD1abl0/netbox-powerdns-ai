---
title: "0019: Re-slice the milestones into smaller steps"
status: accepted
date: 2026-09-29
decision-makers: [jordan]
requirements: [REQ-022]
questions: []
supersedes:
---

# 0019: Re-slice the milestones into smaller steps

## Context and problem statement

M00 finalized eight milestones, M1 to M8. Each was a large step:
- M1 built every cross-cutting foundation first: the database, the OpenAPI
  pipeline, the audit core and the container image, with no DNS feature.
- M2 added identity before the first useful read-only result, which only
  arrived in M3.

In the M01 design session on 2026-09-27, the user asked for smaller
milestones: each merge should be a useful, logical step. Read-only and
unauthenticated features come first, and authentication arrives before
anything can change state from outside.

Accepted ADRs refer to the old milestone numbers, for example "decided in M3
design". They can't be edited (ADR-0001), so the old numbers must stay
resolvable.

## Decision drivers

- Each merge is a useful step that can be reviewed on its own.
- Read-only value comes early: first NetBox, then PowerDNS, then the
  comparison.
- Authentication is in place before anything can change state from outside.
- Later milestones build on earlier ones without rework.
- References to old milestones in accepted ADRs stay resolvable.

## Considered options

1. Keep the eight milestones.
2. Keep the eight milestones, and split each into phases.
3. Re-slice into smaller milestones, in the order they deliver value.

## Decision outcome

Chosen option: **re-slice into eighteen smaller milestones** (option 3). Each
one delivers something usable, and the order follows the drivers: read NetBox,
read PowerDNS, compare, serve, persist, then authenticate before any write.
The user approved the list in the M01 design on 2026-09-27.

| Milestone | Title | Delivers |
|---|---|---|
| M01 | NetBox read path | The `nbpdns` binary, config, logging, the NetBox DNS plugin client, the NetBox lab, and the `nbpdns netbox` commands |
| M02 | PowerDNS read path | The PowerDNS API client, server groups declared in the config file, a PowerDNS lab (two groups, each a primary and a secondary), and `nbpdns powerdns` |
| M03 | Drift report | Compare NetBox with PowerDNS for each group, in memory. Text and JSON reports, and exit codes for scripts. Read-only. |
| M04 | Service | `nbpdns serve` refreshes drift on a schedule. Health and readiness, Prometheus metrics, OTLP trace export, the container image and GoReleaser. |
| M05 | REST API | The OpenAPI 3.1 pipeline, and read-only, unauthenticated drift endpoints |
| M06 | NetBox webhooks | Receive NetBox event-rule webhooks, check their HMAC signature, refresh the affected zones, and carry NetBox's request and user into the trace |
| M07 | SQLite persistence | Migrations, the audit core, runtime settings, drift history, the single-instance lock, secrets encrypted at rest |
| M08 | PostgreSQL and HA | The second dialect, leader election, replica tests |
| M09 | Authentication | The break-glass token, local users with MFA, API tokens and service accounts, sessions. The API is locked down. |
| M10 | SSO and RBAC | OIDC, SAML, proxy headers, roles, the permission reference |
| M11 | Web UI | Login, dashboard, drift, settings, the audit viewer |
| M12 | Write path: plan and apply | Plans, applying to primaries, catalog zones, the `enforce` policy |
| M13 | Change safety | Change limits, approvals, verification after apply |
| M14 | Brownfield import | Import existing PowerDNS zones into NetBox |
| M15 | SIEM export | Sinks, the outbox, hash-chain verification, the event catalogue |
| M16 | Production hardening | Helm, systemd, backup and restore, export and import, threat model, load test |
| M17 | Terraform/OpenTofu provider | A separate repository |
| M18 | Ansible collection and v1.0 | A separate repository, then the v1.0 release |

- **Each milestone is still designed in its own plan-mode session,** which may
  split it further. The list above is provisional beyond M01.
- **The milestone files.** M01 to M08 were stubs with no work, apart from
  M01's approved design. They're rewritten to this list, and renamed where
  the title changed. New stubs are created for M09 to M18. This is the one
  exception to "files never move", and `CLAUDE.md` says so: a planned
  milestone stub with no work yet may be rewritten or renamed by a re-plan
  recorded in an ADR.
- **Open questions** in `project/requirements.md` move to the milestone that
  now needs them.

### Where the old milestones went

| Old milestone | Its scope now lives in |
|---|---|
| M1 Service skeleton | Config and logging: M01. Metrics, health, trace export, binary and image: M04. OpenAPI pipeline: M05. Database, migrations and the audit core: M07; PostgreSQL and leader election: M08. The container lab: M01 (NetBox) and M02 (PowerDNS). |
| M2 Identity | Local users, MFA, break-glass token, API tokens, sessions: M09. SSO and RBAC: M10. |
| M3 Read path and drift report | NetBox client: M01. PowerDNS client and server groups: M02. Drift report: M03. Trace context from NetBox's changelog: M06. |
| M4 Write path | Plan and apply, catalog zones, `enforce`: M12. Change limits, approvals and verification: M13. Sync triggers: M06. Brownfield import: M14. |
| M5 SIEM export | M15 |
| M6 Production hardening | HA tests: M08. Helm, systemd, backup and restore, threat model, load test: M16. |
| M7 Terraform/OpenTofu provider | M17 |
| M8 Ansible collection and v1.0 | M18 |
| (none) | The web UI, which the old plan spread across milestones: M11 |

### References in accepted ADRs

| ADR | Says | Now |
|---|---|---|
| ADR-0004 | Design reviews for M1–M4 check that every write path starts from NetBox data | Every milestone that reads or writes DNS data: M01–M03, M06 and M12–M14 |
| ADR-0005 | The Terraform provider in M7 | M17 |
| ADR-0006 | GraphQL is an option to evaluate in M3 | Evaluated in M01: REST only (ADR-0020) |
| ADR-0006 | Design review in M3/M4: no database driver or file access for PowerDNS data | M02 and M12 |
| ADR-0007 | How zones are assigned to a server group is designed in M3 | M02 |
| ADR-0008 | Where the drift policy is set is decided in M3 design | M03 |
| ADR-0009 | The leader election mechanism is confirmed in M1 design | M08 |
| ADR-0009 | The query layer library is an M1 decision | M07 |
| ADR-0012 | oapi-codegen, `api/openapi.yaml`, `make api-lint` and contract tests in M1 | M05 |
| ADR-0015 | The runtime image is chosen in M1 (Q-025), and M1's image design is reviewed against ADR-0015 | M04 |

### Consequences

- Good: each merge is small and useful. NetBox, PowerDNS and the drift report
  arrive in M01 to M03, before any database or authentication.
- Good: every milestone that can change state from outside comes after M09,
  so authentication is in place first.
- Bad: more milestones means more closing work: reviews, verification and a
  merge request each time.
- Bad: from M05 to M09 the API is read-only but unauthenticated, so it must
  only be exposed on a trusted network. Each of those milestones' docs says
  so.
- Bad: an old milestone number in an accepted ADR needs the table above to be
  read correctly.

### Confirmation

- `project/milestones/` holds M01 to M18, matching the table, and
  `make project-lint` checks their front matter.
- Every open question's "Needed by" names a new milestone.

## More information

- The plan, with the reasons for each boundary, is under "Approved design" in
  `project/milestones/M01-netbox-read-path.md`.
- Recorded by ITEM-0019.
