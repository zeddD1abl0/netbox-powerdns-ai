---
id: ITEM-0032
title: Normalize record values with miekg/dns v2
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-024, REQ-028]
depends_on: [ITEM-0031]
created: 2026-10-06
closed:
---

# ITEM-0032: Normalize record values with miekg/dns v2

## Goal

M03 compares NetBox's values with PowerDNS's. M01 parses only a few record
types, so the rest would show false drift wherever the two sources write the
same data differently. Parse every type with miekg/dns v2
(`codeberg.org/miekg/dns`), as ADR-0025 decides, keeping ADR-0023's rules.

## Acceptance criteria

- [ ] `dns.Value` parses each type's RDATA with miekg/dns v2, with the zone as origin, and prints its canonical text with domain names lowercased. NetBox's unquoted TXT keeps M01's rule.
- [ ] A table test gives NetBox's form and PowerDNS's form of the same data, for CAA, TLSA, SSHFP, DS, HTTPS, SVCB, NAPTR, LOC, TXT, MX, SRV and SOA, one value each.
- [ ] M01's normalization tests, recorded responses and lab fixtures pass. Any change in an existing value is listed here, with why.
- [ ] Every module added to `go.mod` is listed here with its license.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M02's approved design.
