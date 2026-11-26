---
id: ITEM-0109
title: 'Design M09: PostgreSQL and HA'
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-032, REQ-033, REQ-054]
depends_on: [ITEM-0095]
created: 2026-10-18
closed: 2026-10-18
---

# ITEM-0109: Design M09: PostgreSQL and HA

## Goal

M09's design, made in plan mode at M09's start (ADR-0037), and its
record: the approved plan, verbatim, under **Approved design** in M09's
file; ADR-0046 to ADR-0048, accepted with the plan; the brief's dated
answers of 2026-10-18; M09's items, ITEM-0110 to ITEM-0117; and M09 in
progress, with its GitLab milestone in step.

## Acceptance criteria

- [x] M09's file holds the approved plan, verbatim, with its phases and acceptance criteria.
- [x] ADR-0046 to ADR-0048 are accepted, and the brief records the answers of 2026-10-18.
- [x] M09 is in progress, and its GitLab milestone describes its design.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Designed with the user. What the code showed: sqlc's
  PostgreSQL package, generated from today's SQL, differs from SQLite's
  only in its placeholders and in `LIMIT`'s parameters (`int32`); writes
  rely on SQLite's immediate transactions, which the audit chain needs;
  the API and `/status` are built from `internal/service`'s memory, and
  NetBox's zones aren't stored; webhooks' zones wait in memory. No GitLab
  issue was open. The user chose libpq's `sslmode=prefer` as the default,
  the URL as a secret with a `passfile` for the password, and said the
  runner's memory values (ITEM-0100) are deployed.
