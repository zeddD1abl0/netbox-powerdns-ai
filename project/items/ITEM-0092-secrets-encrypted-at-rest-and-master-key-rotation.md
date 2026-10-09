---
id: ITEM-0092
title: Secrets encrypted at rest, and master key rotation
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-051, REQ-053]
depends_on: [ITEM-0090, ITEM-0091]
created: 2026-10-09
closed:
---

# ITEM-0092: Secrets encrypted at rest, and master key rotation

## Goal

Envelope encryption (ADR-0041): AES-256-GCM data keys wrapped by the
master key, `database.master_key` with its `_FILE` form; secret runtime
settings and CLI groups' API keys sealed; `serve` refusing secrets it can't
open; and `nbpdns secrets rotate-master-key`, audited.

## Acceptance criteria

- [ ] No secret is written to the database unsealed, as a test checks.
- [ ] A wrong key and a changed sealed value are refused.
- [ ] Rotating the master key leaves every secret readable.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
