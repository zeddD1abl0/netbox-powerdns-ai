---
id: ITEM-0067
title: The API's middleware
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-037, REQ-046]
depends_on: [ITEM-0066]
created: 2026-10-08
closed:
---

# ITEM-0067: The API's middleware

## Goal

The behaviour every API request shares (ADR-0033): problem details
for every error under `/api`, unknown paths and methods included;
`X-Flow-ID` taken or made, returned, and used as the request ID; a span
per request that continues an incoming `traceparent`; a log line per
request; request metrics; and absolute links from `server.public_url`.

## Acceptance criteria

- [ ] Every error under `/api`, from 400 to 500, unknown paths and methods included, is `application/problem+json`, validated against the spec.
- [ ] A request's `X-Flow-ID` is returned, or a new one is made; it's the request ID in the logs; an invalid one is replaced.
- [ ] An incoming `traceparent` is continued by the request's span.
- [ ] `nbpdns_api_requests_total` and `nbpdns_api_request_duration_seconds` are declared, and their reference regenerated.
- [ ] `server.public_url` is declared, and links are absolute with and without it.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's approved design.
