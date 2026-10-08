---
id: ITEM-0074
title: Refuse requests whose Host isn't server.public_url's
type: debt # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M10
requirements: [REQ-046]
depends_on: []
created: 2026-10-08
closed:
---

# ITEM-0074: Refuse requests whose Host isn't server.public_url's

## Goal

The listener has no authentication until M10, and nothing checks a
request's Host. So a page that an operator on the trusted network visits
could use DNS rebinding to read the API from their browser: the groups,
zones, changes and records. `/status` has been open to the same attack
since M04. When `server.public_url` is set, a request whose Host isn't its
host could be refused, which closes the rebinding path for deployments that
set it. M10's authentication closes it for every deployment.

## Acceptance criteria

- [ ] With `server.public_url` set, a request to the listener whose Host isn't its host is refused, with a test, or M10's design records why authentication alone is enough.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's `/security-review`, which found it below its bar, at about 0.4 confidence: it's the accepted "trusted network until M10" exposure, now with records in it.
