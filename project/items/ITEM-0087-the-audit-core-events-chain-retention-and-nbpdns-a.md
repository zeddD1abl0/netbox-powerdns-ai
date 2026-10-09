---
id: ITEM-0087
title: 'The audit core: events, chain, retention and nbpdns audit'
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-011, REQ-049, REQ-053]
depends_on: [ITEM-0086]
created: 2026-10-09
closed:
---

# ITEM-0087: The audit core: events, chain, retention and nbpdns audit

## Goal

The audit trail (ADR-0039): the event model; the action registry and its
generated reference; append, in the same transaction as each change; the
SHA-384 chain; `nbpdns audit list` and `nbpdns audit verify`; and the
daily prune, by `audit.retention_days`, with its anchor.

## Acceptance criteria

- [ ] Every action is declared once, and `docs/reference/audit-events.md` is generated from the registry.
- [ ] `nbpdns audit verify` passes on a good trail, fails on an edited, deleted or inserted event, and passes after a prune.
- [ ] The service's start and stop are events, with the configuration's fingerprint.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
