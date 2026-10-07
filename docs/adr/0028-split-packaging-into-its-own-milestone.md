---
title: "0028: Split packaging into its own milestone"
status: accepted
date: 2026-10-07
decision-makers: [jordan]
requirements: [REQ-002, REQ-003]
questions: []
supersedes:
---

# 0028: Split packaging into its own milestone

## Context and problem statement

ADR-0019 planned M04, "Service", as `nbpdns serve` with health and
readiness, Prometheus metrics and OTLP trace export, plus the container
image and GoReleaser. Those are two logical steps: running nbpdns
continuously, and packaging it for release. The user wants each merge to be
one useful, logical step. In M04's design session on 2026-10-07, the user
chose to split packaging out.

ADR-0019's own rule allows this: a planned milestone stub with no work yet
may be rewritten or renamed by a re-plan recorded in an ADR. M05 to M18 are
such stubs.

## Decision drivers

- Each merge request is one useful, logical step.
- The service is useful before it's packaged: it runs from the static
  binary that `make build` already makes.
- Records stay as they were written: done milestones, the brief, accepted
  ADRs and closed items aren't rewritten.

## Considered options

1. Keep packaging in M04, as ADR-0019 planned.
2. Split packaging into a new milestone after M04, and renumber the stubs
   after it.
3. Move packaging into M16, "Production hardening", beside the Helm chart
   and the systemd unit, without renumbering.

## Decision outcome

Chosen option: **2, a new milestone, M05 "Packaging"**, chosen by the user.
It keeps both merges small, and the image comes right after the service
that needs it, not at the end.

- **M04, "Service":** `nbpdns serve`, health and readiness, the status
  page, Prometheus metrics and OTLP trace export (ADR-0029).
- **M05, "Packaging":** GoReleaser, static Linux amd64 and arm64 binaries,
  checksums, and the container image (Q-025). The image is reviewed
  against ADR-0015. Where images and binaries are published, and whether
  they're signed, is decided in M05's design.
- **The stubs after it shift up one number:**

  | Before | After | Title |
  |---|---|---|
  | M05 | M06 | REST API |
  | M06 | M07 | NetBox webhooks |
  | M07 | M08 | SQLite persistence |
  | M08 | M09 | PostgreSQL and HA |
  | M09 | M10 | Authentication |
  | M10 | M11 | SSO and RBAC |
  | M11 | M12 | Web UI |
  | M12 | M13 | Write path: plan and apply |
  | M13 | M14 | Change safety |
  | M14 | M15 | Brownfield import |
  | M15 | M16 | SIEM export |
  | M16 | M17 | Production hardening |
  | M17 | M18 | Terraform/OpenTofu provider |
  | M18 | M19 | Ansible collection and v1.0 |

  Each stub's file, ID, title line and branch name change with it.
- **Live references use the new numbers:**
  - open items;
  - the "needed by" column of the open questions;
  - the docs, the CHANGELOG and `CLAUDE.md`;
  - text that nbpdns prints, such as `enforce (from M13)`.
- **Records keep the numbers they were written with:** done milestones,
  the brief, accepted ADRs (ADR-0019, ADR-0027 and others), and closed
  items' notes. Read them with the table above.

### Consequences

- Good: M04 and M05 are each one logical step, and the image follows the
  service closely.
- Good: renumbering now, while the stubs have no work, is cheap.
- Bad: older records name milestones by their old numbers, so a reader
  must use the table above. An "M12" in ADR-0027 means today's M13.

### Confirmation

- `make project-lint` passes with the renamed stubs, and the board lists
  M05 "Packaging".
- No live document or code text names a milestone by its old number.

## More information

- ADR-0019 (the milestone list, and the rule that allows a re-plan).
- Recorded in M04's design, 2026-10-07. Implemented by ITEM-0050.
