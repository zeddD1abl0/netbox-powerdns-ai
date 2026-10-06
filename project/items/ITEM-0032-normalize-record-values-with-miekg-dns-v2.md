---
id: ITEM-0032
title: Normalize record values with miekg/dns v2
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-024, REQ-028]
depends_on: [ITEM-0031]
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0032: Normalize record values with miekg/dns v2

## Goal

M03 compares NetBox's values with PowerDNS's. M01 parses only a few record
types, so the rest would show false drift wherever the two sources write the
same data differently. Parse every type with miekg/dns v2
(`codeberg.org/miekg/dns`), as ADR-0025 decides, keeping ADR-0023's rules.

## Acceptance criteria

- [x] `dns.Value` parses each type's RDATA with miekg/dns v2, with the zone as origin, and prints its canonical text with domain names lowercased. NetBox's unquoted TXT keeps M01's rule.
- [x] A table test gives NetBox's form and PowerDNS's form of the same data, for CAA, TLSA, SSHFP, DS, HTTPS, SVCB, NAPTR, LOC, TXT, MX, SRV and SOA, one value each.
- [x] M01's normalization tests, recorded responses and lab fixtures pass. Any change in an existing value is listed here, with why.
- [x] Every module added to `go.mod` is listed here with its license.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M02's approved design.
- 2026-10-06: Done.
  - **Modules added**, each under an allowed license:
    `codeberg.org/miekg/dns` v0.6.118 (BSD-3-Clause), and through it
    `golang.org/x/crypto` v0.48.0 and `golang.org/x/net` v0.51.0 (BSD-3-Clause,
    the Go authors'). The module's own `go.mod` lists more (certmagic, zap,
    sqlite and others), but they serve its commands, and graph pruning keeps
    them out of nbpdns's `go.mod` and build.
  - `dns.Value` parses every type the library knows with
    `mdns.NewData(type, value, origin)`, then prints it. Before printing,
    the fields tagged as domain names (`name`, `cname`, `mname`) are made
    lowercase, through reflection on the RDATA struct.
  - **Hex is uppercase, not lowercase** as the design said. miekg/dns prints
    SSHFP's and DS's hex in uppercase whatever case it was given, but
    TLSA's as given, so uppercase is the one form all three can share. Hex
    compares without regard to case, so either would do; base64 and text
    keep their case.
  - **Found in the library:** a number too big for its field is kept modulo
    the field's size, not refused: an SRV port of 70000 became 4464, a CAA
    flag of 300 became 44, and an SOA serial of 99999999999 became
    1215752191. nbpdns would then have turned invalid data into different,
    valid-looking data. `changedNumber` compares each plain decimal in the
    input with the one in the same place in the output, and a difference is
    an error, so the value is kept as given and reported as a problem.
    `TestValueOutOfRange` covers MX, SRV, CAA, TLSA, DS, HTTPS and SOA. This
    is worth reporting upstream; nbpdns doesn't depend on a fix.
  - TXT and SPF keep M01's own parser, since the library reads an unquoted
    value as several strings, while NetBox's plugin keeps one.
  - **No existing value changed:** M01's `TestValue` table, the recorded
    NetBox responses and the lab fixture's expectations pass unchanged.
    Error messages for unparseable values now quote the library's reason.
  - `TestValueAcrossSources` gives NetBox's form and PowerDNS's form for
    CAA, TLSA, SSHFP, DS, HTTPS, SVCB, NAPTR, LOC, TXT, MX, SRV, SOA and
    AAAA, and expects one value each. The PowerDNS forms here are written
    by hand; ITEM-0035's recorded responses check them against PowerDNS.
  - Docs: the normalization table in "How nbpdns reads NetBox", the
    `internal/dns` package comment, and a CHANGELOG line.
