---
id: ITEM-0121
title: Bump the API spec's version for M09's additions
type: debt # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-013, REQ-019]
depends_on: []
created: 2026-11-05
closed:
---

# ITEM-0121: Bump the API spec's version for M09's additions

## Goal

`info.version` in `api/openapi.yaml` is still 1.2.0, the version that
v0.8.0 released, though M09 has added fields to `/api/status`:
`database`'s `backend`, `url` and `tls` (ITEM-0111), and `replica` and
`leader` (ITEM-0112). [How the API is
designed](../../docs/explanation/how-nbpdns-api-is-designed.md) says the
spec's own semantic version grows with each addition, so that a client can
tell what a server's API has. Bump it once for M09's additions, before
v0.9.0, and say so in the CHANGELOG.

## Acceptance criteria

- [ ] `info.version` is 1.3.0, and `docs/reference/api.md`, regenerated, says so.
- [ ] The CHANGELOG's `Unreleased` says that the API's spec is at 1.3.0.
- [ ] `make check` passes.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-05: Created by ITEM-0114, which found it while it changed the
  spec's descriptions (no new fields, so it didn't bump the version itself).
  v0.8.0's spec is at 1.2.0, as checked with `git show v0.8.0:api/openapi.yaml`.
