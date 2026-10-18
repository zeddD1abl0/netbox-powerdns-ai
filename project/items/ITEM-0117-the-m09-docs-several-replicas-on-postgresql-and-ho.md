---
id: ITEM-0117
title: 'The M09 docs: several replicas on PostgreSQL, and how the leader is chosen'
type: task # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033, REQ-054]
depends_on: [ITEM-0116]
created: 2026-10-18
closed:
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

- [ ] The pages exist, each in one Diátaxis section, and pass the docs lint.
- [ ] The how-to's steps are checked in the lab, and M09's manual verification is recorded in its file.
- [ ] The CHANGELOG's `Unreleased` section describes M09 as a whole.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
