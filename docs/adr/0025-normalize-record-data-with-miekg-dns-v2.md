---
title: "0025: Normalize record data with miekg/dns v2"
status: accepted
date: 2026-10-06
decision-makers: [jordan]
requirements: [REQ-024, REQ-028]
questions: []
supersedes:
---

# 0025: Normalize record data with miekg/dns v2

## Context and problem statement

M03 compares NetBox's records with PowerDNS's, value by value, in the model
of ADR-0023. The two write the same data differently. NetBox's DNS plugin
keeps a value much as it was entered, often with names relative to the zone;
PowerDNS keeps its own text form, with absolute names. M01's normalization
parses only A, AAAA, CNAME, DNAME, MX, NS, PTR, SRV, SOA, TXT and SPF, and
compares every other type as text. CAA, TLSA, SSHFP, DS, HTTPS, SVCB, NAPTR,
LOC and the rest would show drift wherever the two texts differ, though the
data is the same.

## Decision drivers

- No false drift: the same record data gives the same value, whichever
  source it came from.
- Every type NetBox or PowerDNS can hold, not a list that falls behind.
- An established library over code of our own (the user's preference),
  under an allowed license (MIT, BSD, Apache-2.0, MPL-2.0).
- M13 verifies the secondaries with DNS queries (ADR-0007), so a DNS library
  will be needed anyway.

## Considered options

1. `codeberg.org/miekg/dns`, miekg/dns v2.
2. `github.com/miekg/dns`, miekg/dns v1.
3. Parsers of our own for a fixed list of types.

## Decision outcome

Chosen option: **miekg/dns v2** (`codeberg.org/miekg/dns`, BSD-3-Clause),
because it parses and prints every record type, is actively developed, and
will also send M13's queries. The user chose it on 2026-10-06.

- `dns.Value` parses each value as its type's RDATA, with the zone as the
  origin for relative names, and prints the library's canonical text, with
  domain names made lowercase.
- ADR-0023's rules all still hold: absolute lowercase names, canonical TXT,
  effective TTLs, RRsets. NetBox's unquoted TXT values keep M01's rule: one
  string, as the plugin takes it.
- A value that doesn't parse is kept as given, with its whitespace collapsed,
  and reported as a problem, as in M01.
- The module is pinned in `go.mod`. It's before 1.0, and its author expects
  to keep changing its API, so an upgrade is a change of its own, with its
  tests.

### Consequences

- Good: one parser for every type, the same on both sides, so values compare
  as data rather than as text.
- Good: M13 has its DNS client already.
- Bad: a dependency before 1.0, whose API may change between releases. The
  pin means that only costs anything when nbpdns chooses to upgrade.
- Bad: nbpdns's normalized text is the library's, so a change in how it
  prints a type changes nbpdns's output; the table tests catch it.

### Confirmation

- A table test feeds NetBox's form and PowerDNS's form of the same data, for
  CAA, TLSA, SSHFP, DS, HTTPS, SVCB, NAPTR, LOC, TXT, MX, SRV and SOA, and
  expects one value.
- M01's normalization tests and the lab fixtures pass unchanged.
- `make vuln` checks the module like every other.

## Pros and cons of the options

### miekg/dns v1

- Good: a stable API, used across the Go ecosystem for years.
- Bad: in maintenance mode, with fixes only, and due to be archived; its
  author's work goes into v2.

### Parsers of our own

- Good: no dependency.
- Bad: more code to write and keep correct, and every type left off the list
  shows false drift.

## More information

- miekg/dns v2: <https://codeberg.org/miekg/dns>.
- ADR-0023, the model these values belong to.
- Recorded in M02's design, 2026-10-06. Implemented by ITEM-0032.
