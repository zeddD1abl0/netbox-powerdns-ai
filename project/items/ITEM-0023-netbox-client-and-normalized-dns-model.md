---
id: ITEM-0023
title: NetBox client and normalized DNS model
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-024, REQ-027, REQ-040]
depends_on: [ITEM-0020, ITEM-0021, ITEM-0022]
created: 2026-09-27
closed: 2026-10-06
---

# ITEM-0023: NetBox client and normalized DNS model

## Goal

A read-only client for the NetBox DNS plugin's REST API, and the normalized
DNS model that M02 and M03 share.

## Acceptance criteria

- [x] ADR-0020 records REST only, the supported versions, and v2 tokens. Q-007 is answered, producing REQ-040.
- [x] The client reads status, views, zones, nameservers and records. It pages through results, fetches records per zone with bounded concurrency, and has timeouts and retries (honoring `Retry-After`). It uses TLS with an optional CA file, and its errors are typed.
- [x] The normalization rules are documented and covered by table-driven tests: names, relative targets, TXT, effective TTL, status, and the managed flag.
- [x] Integration tests pass against both lab NetBox versions, with a least-privilege v2 token. A token without permission, and an unsupported version, are covered.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-27: Created from M01's approved design, before implementation
  started.
- 2026-09-29: Decided with the user, and recorded in ADR-0020:
  - `netbox.url` may use `http://`. The key's description in the
    configuration reference warns that the token is then sent unencrypted,
    and the client logs a warning.
  - The normalized model groups records into RRsets. When NetBox records in
    one RRset have different TTLs, the RRset takes the lowest.
  - Retries, timeouts and TLS are tested against a local HTTP test server
    that returns only status codes, headers and delays. NetBox's API
    responses are tested only through the lab and recorded responses.
  ADR-0020 is written before ADR-0021 (ITEM-0020) to keep the numbers in the
  approved design, as a checkpoint commit for this item.
- 2026-09-29: Checkpoint: ADR-0020 is written and accepted, Q-007 is
  answered, and REQ-040 exists. The ADR also records the decisions above
  (plain HTTP, RRsets, test servers). Two refinements it adds:
  - an RRset's TTL is the lowest among its *active* records, so an inactive
    record can't cause TTL drift;
  - an unsupported version only warns in most commands, and
    `nbpdns netbox check` reports it as a failure.
  Still to check against the lab: how the plugin stores IDN names and TXT
  values.
- 2026-10-06: State at the start of this session: `internal/dns` was written
  and tested, and `internal/netbox` had its transport, retries and typed
  errors but didn't compile (no status, version check or readers). Neither was
  committed, and the board was stale. Checked against the lab (NetBox 4.7.1
  with plugin 1.7.2, and 4.6.10 with 1.6.1):
  - **IDN names** are stored in their ASCII (punycode) form, such as
    `xn--bcher-kva.example`; only `display` shows Unicode. Record names keep
    the case they were entered in (`Mixed`), so lowercasing is needed.
  - **TXT values** are stored as entered: unquoted (`v=spf1 -all`, or
    `he said "hi"`), or quoted (`"quoted" "two"`). A value longer than 255
    bytes is split by the plugin into quoted 255-byte strings. Non-ASCII TXT
    values are rejected. `dns.Value`'s rule (quoted means zone-file
    strings, anything else is one string) fits all of these.
  - A valid token without permissions gets 200 from `/api/status/` and 403
    from the plugin's endpoints; a bad token gets 403 from both, which is how
    the client tells them apart. A v2 token is `nbt_<key>.<token>`; the
    tokens API returns only the `<token>` part, once, on creation.
  - The zone `name` filter is exact and case-sensitive, and takes the ASCII
    form only, so `--zone` input is lowercased, loses its final dot, and
    non-ASCII input is refused with a hint (no IDNA dependency).
  - Records carry the plugin's own `active` flag (record and zone both
    active), which the model uses as is.
- 2026-10-06: Done.
  - **`internal/netbox`:** `Status`, `Connect` (a missing plugin is an
    error; an unsupported release logs a warning), `Views`, `Nameservers`,
    `Zones` (by name, view and status), `Records`, `Count` (for `check`),
    `FindZone` (an `AmbiguousZoneError` names the views), `ZoneName`, and
    `ReadZones`, which reads each zone's records with at most
    `netbox.concurrency` requests in flight and stops at the first error.
    Lists follow NetBox's `next` links, rebased onto the configured URL.
    `Supported` declares the releases once; a unit test checks that the lab
    runs exactly them.
  - **Retries** now cover only network failures known to pass (timeouts,
    refused or reset connections, EOFs, temporary DNS failures), not a list
    of failures to skip: a TLS alert from the server arrives as a
    `*net.OpError` and plain HTTP from an `https://` URL as an untyped error,
    so a list of exclusions missed both, and retried them.
  - **`Client.Close`** drops idle connections. Without it, the lab's
    NetBox 4.6 (two Granian workers) held a thread per idle keep-alive
    connection from earlier test clients, and a new connection waited about
    30 s. `tracing.Transport` now passes `CloseIdleConnections` through.
  - **The plugin keeps an RRset's active records on one TTL** by default
    (`enforce_unique_rrset_ttl`): it refuses a conflicting record when it's
    added, and copies a changed TTL to the rest of the RRset. So the lowest-
    TTL rule (ADR-0020) only matters where that setting is off. The lab stays
    on the plugin's defaults; the fixture has an inactive record with its own
    TTL instead, and the rule stays covered by `internal/dns`'s unit tests.
  - **The NetBox 4.6 plugin** answers a validation error in a bulk create
    with 500, not 400. Nothing in nbpdns writes, so it only affects fixtures.
  - **Tests:**
    - `internal/lab/fixture.go` creates a fixture per test: two views with a
      zone of the same name, 37 varied records (39 with the plugin's SOA and
      NS), a least-privilege v2 token and a token with no permissions.
    - The integration tests read it from both NetBoxes at a page size of 10
      (four pages), and cover a token without permission, a bad v1 and v2
      token, an ambiguous zone, and an unsupported release.
    - `TestRecord -record` recorded the responses in `testdata/`, which the
      unit tests map into the model.
    - The transport tests use local servers that send only status codes,
      headers, delays and recorded responses: retries and backoff,
      `Retry-After`, timeouts, TLS (untrusted, CA file, TLS 1.1, plain HTTP),
      redirects, 401/403, headers, warnings, token redaction, cancellation
      and bounded concurrency.
  - The record sort in `dns.SetRecords` now breaks ties on status and TTL,
    so equal values come out in the same order every run.
