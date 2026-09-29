---
id: ITEM-0023
title: NetBox client and normalized DNS model
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-024, REQ-027, REQ-040]
depends_on: [ITEM-0020, ITEM-0021, ITEM-0022]
created: 2026-09-27
closed:
---

# ITEM-0023: NetBox client and normalized DNS model

## Goal

A read-only client for the NetBox DNS plugin's REST API, and the normalized
DNS model that M02 and M03 share.

## Acceptance criteria

- [x] ADR-0020 records REST only, the supported versions, and v2 tokens. Q-007 is answered, producing REQ-040.
- [ ] The client reads status, views, zones, nameservers and records. It pages through results, fetches records per zone with bounded concurrency, and has timeouts and retries (honoring `Retry-After`). It uses TLS with an optional CA file, and its errors are typed.
- [ ] The normalization rules are documented and covered by table-driven tests: names, relative targets, TXT, effective TTL, status, and the managed flag.
- [ ] Integration tests pass against both lab NetBox versions, with a least-privilege v2 token. A token without permission, and an unsupported version, are covered.

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
