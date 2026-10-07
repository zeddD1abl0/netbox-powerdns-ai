---
id: ITEM-0040
title: Write record values to PowerDNS in its own text form
type: task # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M13
requirements: [REQ-028]
depends_on: []
created: 2026-10-06
closed:
---

# ITEM-0040: Write record values to PowerDNS in its own text form

## Goal

PowerDNS's API accepts a record's content only in PowerDNS's own canonical
text, and refuses anything else with 422 and the form it wanted: for an
HTTPS record, `1 . alpn="h3,h2" ipv4hint=192.0.2.10` is refused as "Not in
expected format (parsed as '1 . alpn=h3,h2 ipv4hint=192.0.2.10')".
nbpdns's normalized values (ADR-0025) are for comparing, and follow
miekg/dns's text, which quotes `alpn` and `ipv4hint`. So when M12 writes
NetBox's data to PowerDNS, it must turn each value into PowerDNS's form,
not send the normalized one.

## Acceptance criteria

- [ ] Every value nbpdns writes is in PowerDNS's own form, for every record type the lab's fixtures cover, and PowerDNS accepts it.
- [ ] After a write, reading the zone back gives the same normalized values as NetBox's data, so the write leaves no drift.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Found during M02's manual verification, creating an HTTPS
  record through PowerDNS 5.1.4's API with `alpn="h3,h2"`. PowerDNS takes
  `alpn=h3,h2`, and nbpdns's normalization reads both the same, so reading
  is fine; only writing is affected.
- 2026-10-07: M12, "Write path: plan and apply", is now M13 (ADR-0028).
