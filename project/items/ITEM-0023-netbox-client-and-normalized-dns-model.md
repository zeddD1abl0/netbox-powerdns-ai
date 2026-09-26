---
id: ITEM-0023
title: NetBox client and normalized DNS model
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-024, REQ-027]
depends_on: [ITEM-0020, ITEM-0021, ITEM-0022]
created: 2026-09-27
closed:
---

# ITEM-0023: NetBox client and normalized DNS model

## Goal

A read-only client for the NetBox DNS plugin's REST API, and the normalized
DNS model that M02 and M03 share.

## Acceptance criteria

- [ ] ADR-0020 records REST only, the supported versions, and v2 tokens. Q-007 is answered, producing REQ-040.
- [ ] The client reads status, views, zones, nameservers and records. It pages through results, fetches records per zone with bounded concurrency, and has timeouts and retries (honoring `Retry-After`). It uses TLS with an optional CA file, and its errors are typed.
- [ ] The normalization rules are documented and covered by table-driven tests: names, relative targets, TXT, effective TTL, status, and the managed flag.
- [ ] Integration tests pass against both lab NetBox versions, with a least-privilege v2 token. A token without permission, and an unsupported version, are covered.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-27: Created from M01's approved design, before implementation
  started.
