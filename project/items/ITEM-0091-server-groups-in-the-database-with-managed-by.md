---
id: ITEM-0091
title: Server groups in the database, with managed_by
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-050]
depends_on: [ITEM-0090]
created: 2026-10-09
closed:
---

# ITEM-0091: Server groups in the database, with managed_by

## Goal

Server groups as managed resources (ADR-0040): the file's groups mirrored
at start as `managed_by=file`, audited; `nbpdns groups list|add|set|remove`
for `managed_by=cli`; a name both claim stops `serve`; `serve` applies group
changes within seconds; and `managed_by` in the API.

## Acceptance criteria

- [ ] The file's groups are mirrored, read-only to the CLI, and removed when the file drops them.
- [ ] A group added with the CLI is compared by a running `serve` within seconds, and one removed leaves its state and metrics.
- [ ] `managed_by` is in the API's server groups.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
