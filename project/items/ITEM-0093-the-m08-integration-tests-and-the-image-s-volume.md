---
id: ITEM-0093
title: The M08 integration tests and the image's volume
type: task # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-036, REQ-049, REQ-050, REQ-052]
depends_on: [ITEM-0089, ITEM-0091, ITEM-0092]
created: 2026-10-09
closed:
---

# ITEM-0093: The M08 integration tests and the image's volume

## Goal

The integration tests on the lab: a restart that keeps the reports and
history; a second `serve` refused; settings and groups changed with the CLI
reaching a running `serve`; `nbpdns audit verify` passing, then failing
after an edit. And the image: the release check runs it with a volume owned
by 65532 at `/var/lib/nbpdns`, and writes the database.

## Acceptance criteria

- [ ] The integration tests pass in CI.
- [ ] The release check writes the database in the image, on a volume.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
