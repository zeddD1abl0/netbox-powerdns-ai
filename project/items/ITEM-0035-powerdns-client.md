---
id: ITEM-0035
title: PowerDNS client
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-028, REQ-041]
depends_on: [ITEM-0031, ITEM-0032, ITEM-0034]
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0035: PowerDNS client

## Goal

Read each group's primary through the PowerDNS API into the shared model
(ADR-0024): the server's type and release, its zones, and each zone's RRsets,
with typed errors, on the shared HTTP client.

## Acceptance criteria

- [x] `internal/powerdns` reads the server (authoritative, release checked against `Supported`, 5.1 and 5.0), the zones list, each zone's RRsets with disabled records, and many zones with bounded concurrency.
- [x] Errors are typed: unreachable; key rejected; not authoritative or no such `server_id`; zone not found; unsupported release; and API errors with PowerDNS's own text.
- [x] Zones map into the model as ADR-0024 says. Unit tests use responses recorded from the lab; integration tests read fixtures from both primaries.
- [x] The key is sent only as `X-API-Key`, never logged or printed, and requests carry `traceparent`.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M02's approved design.
- 2026-10-06: Done.
  - **`internal/powerdns`**, on `internal/httpclient`: `Server` and
    `Connect` (a non-authoritative server is an error; an unsupported
    release logs a warning), `Zones` (`dnssec=false`), `FindZone` (the
    API's `zone` filter, with the name normalized first), `ZoneData` (the
    zone with its RRsets, disabled records included) and `ReadZones`, which
    reads zones with at most `powerdns.concurrency` requests in flight and
    stops at the first error. `Supported` is 5.1 and 5.0, and
    `TestSupportedMatchesLab` checks the lab runs exactly them.
  - **Errors:** `AuthError` for 401 and 403; `ServerNotFoundError` and
    `ZoneNotFoundError` for a 404 where each is meant; `NotAuthoritativeError`;
    `VersionError`; and `APIError`, with PowerDNS's `error` field, or the
    start of its text, since 5.0 answers some errors in plain text. A
    redirect names its target and the key to fix. `UnreachableError` is
    the shared one.
  - **Into the model:** every record of every RRset becomes a raw record
    with the RRset's TTL, `active` or `disabled`, never managed; the zone's
    name servers are its apex NS RRset's active values. `dns.Zone`'s
    NetBox-only fields (`view`, `status`, `default_ttl`) are now left out of
    JSON when empty, which changes nothing for NetBox's zones, where they're
    always set.
  - **Found by the tests:** `FindZone` passed the name as typed to the
    filter, and a base URL without a path gave request paths without their
    leading slash, which showed in errors. Both are fixed.
  - **ADR-0025 checked against PowerDNS:** the recorded responses and the
    integration tests normalize PowerDNS's own text, such as the MX name's
    case, TLSA's lowercase hex and HTTPS's unquoted `alpn`, to the values
    `TestValueAcrossSources` expects from NetBox.
  - **Tests:** responses recorded from both lab servers (`-record`) for the
    model and the release check; local servers for the errors, the request
    path and headers (`X-API-Key`, `traceparent`), the key never in logs or
    errors, the `http://` warning, missing settings, error text trimming,
    and bounded concurrency; integration tests against both primaries for
    reading, a missing zone, a wrong key, an unknown `server_id`, and an
    unsupported release.
  - The supported versions reference now has a PowerDNS section, from
    `Supported`.
