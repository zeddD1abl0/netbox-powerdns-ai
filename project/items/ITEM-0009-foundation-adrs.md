---
id: ITEM-0009
title: "ADRs: docs platform, API standard, persistence and HA"
type: task
status: open
milestone: M00
requirements: [REQ-013, REQ-019, REQ-020]
depends_on: [Q-044]
created: 2026-09-25
closed:
---

# ITEM-0009: ADRs: docs platform, API standard, persistence and HA

## Goal

Record the foundation decisions M1 builds on, once the questions behind them
are answered.

## Acceptance criteria

- [ ] ADR: documentation platform and theme (Q-044, informed by the ITEM-0008 spike).
- [ ] ADR: API standard. Covers OpenAPI 3.1 spec-first, the Zalando guidelines, RFC 9457, versioning, pagination, concurrency control, idempotency and deprecation.
- [x] ADR: persistence and HA (Q-019, Q-020): [ADR-0009](../../docs/adr/0009-persistence-and-high-availability.md).
- [ ] Each ADR is accepted by the user and linked from `requirements.md`.

## Notes
- 2026-09-25: ADR-0009 written and accepted from the Q-019/Q-020 answers.
