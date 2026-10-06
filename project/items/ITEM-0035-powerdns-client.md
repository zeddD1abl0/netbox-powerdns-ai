---
id: ITEM-0035
title: PowerDNS client
type: feature # feature | bug | debt | task
status: in-progress # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-028, REQ-041]
depends_on: [ITEM-0031, ITEM-0032, ITEM-0034]
created: 2026-10-06
closed:
---

# ITEM-0035: PowerDNS client

## Goal

Read each group's primary through the PowerDNS API into the shared model
(ADR-0024): the server's type and release, its zones, and each zone's RRsets,
with typed errors, on the shared HTTP client.

## Acceptance criteria

- [ ] `internal/powerdns` reads the server (authoritative, release checked against `Supported`, 5.1 and 5.0), the zones list, each zone's RRsets with disabled records, and many zones with bounded concurrency.
- [ ] Errors are typed: unreachable; key rejected; not authoritative or no such `server_id`; zone not found; unsupported release; and API errors with PowerDNS's own text.
- [ ] Zones map into the model as ADR-0024 says. Unit tests use responses recorded from the lab; integration tests read fixtures from both primaries.
- [ ] The key is sent only as `X-API-Key`, never logged or printed, and requests carry `traceparent`.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M02's approved design.
