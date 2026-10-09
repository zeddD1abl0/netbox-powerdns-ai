---
id: ITEM-0090
title: 'Runtime settings: key kinds, precedence and live application'
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-050]
depends_on: [ITEM-0087]
created: 2026-10-09
closed:
---

# ITEM-0090: Runtime settings: key kinds, precedence and live application

## Goal

Runtime settings (ADR-0040): each key a bootstrap or a runtime key, shown
in the generated reference; the database as a source between the defaults
and the file, with pinning; `nbpdns settings list|get|set|unset`, audited
with the OS user; and `serve` applying changed runtime keys within
seconds.

## Acceptance criteria

- [ ] Table tests cover the precedence, pinning, and refusing a bootstrap key.
- [ ] A running `serve` uses each changed runtime key within seconds, as tests show.
- [ ] `nbpdns config show` names the database as a source.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
