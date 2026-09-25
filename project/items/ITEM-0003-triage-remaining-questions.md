---
id: ITEM-0003
title: Settle the non-blocking questions that M0 needs
type: task
status: done
milestone: M00
requirements: []
depends_on: [Q-018, Q-044, Q-045, Q-046, Q-049]
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0003: Settle the non-blocking questions that M0 needs

## Goal

Resolve the questions whose **Needed by** is M0 but that don't block M1 design.
Confirm with the user that every other question can wait for the milestone
named in its **Needed by** column.

## Acceptance criteria

- [x] Q-018: differentiators from existing tools recorded in `brief.md`.
- [x] Q-044: docs platform chosen (feeds ITEM-0008 and ITEM-0009).
- [x] Q-045: documentation audiences confirmed.
- [x] Q-046: docs hosting and versioning confirmed.
- [x] Q-049: CI runners confirmed.
- [x] The user has reviewed the **Needed by** column for the other questions.

## Notes

- 2026-09-25: All answered. Q-044: Hugo (ADR-0011). Q-045, Q-046 and Q-049:
  defaults accepted. Q-018: three differentiators recorded in the brief. The
  user reviewed the **Needed by** column along with the question batches. The
  API style question was added and answered as Q-056 (ADR-0012).
