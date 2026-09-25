---
id: ITEM-0002
title: Answer the questions that block M1
type: task
status: done
milestone: M00
requirements: []
depends_on: [Q-005, Q-006, Q-008, Q-009, Q-010, Q-019, Q-020, Q-047, Q-048]
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0002: Answer the questions that block M1

## Goal

Get the user's answers to every question marked **Blocking** in
[requirements.md](../requirements.md#open-questions). Work through them in plan
mode, one section at a time.

## Acceptance criteria

- [x] Q-005 (product identity, module path, license).
- [x] Q-006 (NetBox DNS data model).
- [x] Q-008 (PowerDNS versions and backends).
- [x] Q-009 (topologies).
- [x] Q-010 (sync semantics).
- [x] Q-019 (persistence).
- [x] Q-020 (HA).
- [x] Q-047 (commit model).
- [x] Q-048 (test lab).
- [x] Each answer is moved to **Answered**, with a REQ or ADR linked.

## Notes

- 2026-09-25: Answered in the M0b discovery session. The Q-005 answer left the
  GitHub owner blank, so it was split out as **Q-055** (blocking) and added to
  ITEM-0004's `depends_on`. The parts of Q-008 and Q-010 that weren't answered
  became Q-053 (PowerDNS versions) and Q-054 (sync trigger and brownfield
  import), both non-blocking. Recorded in ADR-0005 to ADR-0010.
