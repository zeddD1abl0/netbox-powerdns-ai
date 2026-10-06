---
id: ITEM-0036
title: nbpdns powerdns commands, netbox zones --group, docs and CHANGELOG
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-041, REQ-042]
depends_on: [ITEM-0033, ITEM-0035]
created: 2026-10-06
closed:
---

# ITEM-0036: nbpdns powerdns commands, netbox zones --group, docs and CHANGELOG

## Goal

Make M02 usable end to end: `nbpdns powerdns check`, `zones` and `records`,
`nbpdns netbox zones --group`, and the docs from M02's design, with the
CHANGELOG.

## Acceptance criteria

- [ ] The commands work as in M02's design, as tables and as JSON, with exit statuses 0, 1 and 2 and clear messages.
- [ ] `nbpdns netbox zones --group G` lists only the zones in G's views, and reports a zone name that's in two of them.
- [ ] The docs exist: the tutorial "Read your PowerDNS zones with nbpdns"; the how-to guides "Connect nbpdns to PowerDNS" and "Put the PowerDNS API behind a TLS proxy", the latter checked by hand with a client certificate; "How nbpdns reads PowerDNS"; and the lab how-to and supported versions reference, updated.
- [ ] `CHANGELOG.md` has lines under Unreleased.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M02's approved design.
