---
id: ITEM-0009
title: "ADRs: docs platform, API standard, persistence and HA"
type: task
status: done
milestone: M00
requirements: [REQ-013, REQ-019, REQ-020]
depends_on: [Q-044, Q-056]
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0009: ADRs: docs platform, API standard, persistence and HA

## Goal

Record the foundation decisions M1 builds on, once the questions behind them
are answered.

## Acceptance criteria

- [x] ADR: documentation platform (Q-044): [ADR-0011](../../docs/adr/0011-documentation-platform-hugo.md). The theme pick is recorded in ITEM-0008.
- [x] ADR: API standard ([ADR-0012](../../docs/adr/0012-api-standard.md)). Covers OpenAPI 3.1 spec-first, the Zalando guidelines, RFC 9457, versioning, pagination, concurrency control, idempotency and deprecation.
- [x] ADR: persistence and HA (Q-019, Q-020): [ADR-0009](../../docs/adr/0009-persistence-and-high-availability.md).
- [x] Each ADR is accepted by the user and linked from `requirements.md`.

## Notes
- 2026-09-25: ADR-0009 written and accepted from the Q-019/Q-020 answers.
- 2026-09-25: ADR-0011 (Hugo) and ADR-0012 (Zalando in full, no URI
  versioning) written and accepted. The theme is no longer an ADR decision: it's
  reversible, so the ITEM-0008 spike records it.
