---
id: ITEM-0117
title: 'The M09 docs: several replicas on PostgreSQL, and how the leader is chosen'
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033, REQ-054]
depends_on: [ITEM-0116]
created: 2026-10-18
closed: 2026-11-25
---

# ITEM-0117: The M09 docs: several replicas on PostgreSQL, and how the leader is chosen

## Goal

M09's docs: the how-to "Run several replicas of nbpdns", with a
Kubernetes and a Compose example, `database.listen`, and Prometheus
aggregation; the explanation "How nbpdns replicas choose a leader", with
the timings and the fence; and "How nbpdns stores its state", updated for
PostgreSQL and the shared state. The how-to is checked step by step in the
lab, which is M09's manual verification.

## Acceptance criteria

- [x] The pages exist, each in one Diátaxis section, and pass the docs lint.
- [x] The how-to's steps are checked in the lab, and M09's manual verification is recorded in its file.
- [x] The CHANGELOG's `Unreleased` section describes M09 as a whole.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
- 2026-11-25: Done.
  - New how-to, "Run several replicas of nbpdns"
    (`docs/how-to/run-several-replicas-of-nbpdns.md`): the configuration
    every replica shares, `database.listen` among it; a Kubernetes example
    (3 replicas, a rolling update, spread across nodes, no volume, the URL
    from a secret, and a Service) and a Compose one (three services on one
    anchor, each with its own host name and port); which replica leads,
    with `/status` and the audit trail; a `sum(nbpdns_leader) != 1` alert
    beside the drift alerts of "Monitor drift with Prometheus"; and how
    replicas stop and are replaced.
  - New explanation, "How nbpdns replicas choose a leader": the lock and its
    row, the followers' checks, the session's timeouts, the step-down, the
    fence, the write lock's bound (ADR-0049), and a table of how long each
    loss takes, with the lab's times.
  - "Run nbpdns on PostgreSQL" loses its stale "come in M09" note and its
    replica sections, which moved to the how-to, keeping a short pointer
    under its old heading; its migration examples show all eight.
    "How nbpdns stores its state" keeps the serve lock and the write lock,
    and points to the explanation for the leader; its "doesn't do yet"
    has the move from SQLite in place of more than one instance. "How
    nbpdns runs as a service" says only the leader refreshes, and drops its
    "comes in M09" line. The links to the old sections, from the container,
    monitoring and webhook how-tos, point to the new page.
  - The CHANGELOG's Unreleased opens with a summary of M09, as 0.7.0's
    section does.
  - M09's manual verification, steps 1 to 10, passed and is in its
    Verification log, with a second entry for the how-to's examples: the
    Compose example ran on the snapshot image in the lab, and the
    Kubernetes manifest was parsed, not applied, since `kubectl` reaches
    only the user's own cluster.
  - `make docs-lint` and `make docs-links` pass.
