---
id: ITEM-0088
title: Drift history and restore at start
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-044, REQ-052]
depends_on: [ITEM-0087]
created: 2026-10-09
closed:
---

# ITEM-0088: Drift history and restore at start

## Goal

The drift history in the database (ADR-0042): each refresh and each
group's result; a zone's row when its state or changes differ from its
last; each group's last report, restored at start, with `/readyz` and
`/status` saying so; a zone's change of state as an audit event; and the
prune by `drift.history_retention_days`.

## Acceptance criteria

- [ ] A zone driven through drift and back has a row for each change, and a refresh with no change writes none.
- [ ] After a restart, `serve` serves its last reports at once, and says how old they are.
- [ ] A failed history write is counted, logged and shown, and the refresh is still served.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
