---
id: ITEM-0072
title: The API docs and CHANGELOG
type: task # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-018, REQ-046, REQ-047]
depends_on: [ITEM-0070, ITEM-0071]
created: 2026-10-08
closed:
---

# ITEM-0072: The API docs and CHANGELOG

## Goal

The API's documentation: the reference generated from the spec,
"Service endpoints" updated, a how-to on reading drift and records
through the API, an explanation of the API's design, and the CHANGELOG.

## Acceptance criteria

- [ ] `docs/reference/api.md` is generated from the spec by `gendocs`, and `generate-check` covers it.
- [ ] The how-to and the explanation exist, and "Service endpoints" lists `/api`, `/api/openapi.yaml` and `/api/docs`.
- [ ] The CHANGELOG has the API under Unreleased.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's approved design.
