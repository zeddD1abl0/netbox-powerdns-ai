---
id: ITEM-0070
title: DNS records from NetBox in the API
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-046, REQ-047]
depends_on: [ITEM-0069]
created: 2026-10-08
closed:
---

# ITEM-0070: DNS records from NetBox in the API

## Goal

Each zone's RRsets as NetBox defines them, for IaC to read
(REQ-047). `serve` reads every active NetBox zone in the groups' views,
keeps them, and `…/zones/{zone}/rrsets` serves them, as of NetBox's last
successful read. `nbpdns drift` reads no more than before.

## Acceptance criteria

- [ ] `drift.Options.ReadNetBox` reads every active zone in the groups' views, and `serve` sets it; a test shows `nbpdns drift` doesn't.
- [ ] `…/rrsets` pages a zone's active RRsets, marks NetBox's managed records, carries `as_of`, and is 404 for a zone that isn't active in NetBox.
- [ ] An integration test matches a zone's `rrsets` with NetBox's records.
- [ ] The scale check (REQ-043) records each refresh's time and the resident memory with NetBox's zones kept.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's approved design.
