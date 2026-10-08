---
id: ITEM-0071
title: Scalar at /api/docs, vendored and locked down
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-018, REQ-046]
depends_on: [ITEM-0066]
created: 2026-10-08
closed:
---

# ITEM-0071: Scalar at /api/docs, vendored and locked down

## Goal

Scalar's API reference at `/api/docs`, vendored and embedded, with
its fonts and Agent Scalar off, and a strict same-origin
Content-Security-Policy, so the page reaches no other host (ADR-0034).

## Acceptance criteria

- [ ] `make vendor-scalar` fetches a pinned version, checks its SHA-256, and writes it and its licence to `internal/api/docs/`.
- [ ] `/api/docs` and its assets are served with the CSP, and the embedded page references only same-origin URLs, as tests show.
- [ ] The page renders the reference in headless Firefox.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's approved design.
