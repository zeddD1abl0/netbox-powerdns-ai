---
id: ITEM-0038
title: Fix the M02 code review findings
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-028, REQ-041, REQ-042]
depends_on: [ITEM-0031, ITEM-0032, ITEM-0033, ITEM-0035, ITEM-0036]
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0038: Fix the M02 code review findings

## Goal

Fix the findings of the `/code-review high` run at M02's close, on the
branch's whole diff against `main`. It found ten: a crash, three
correctness problems, two misleading messages, and four cleanups. All ten
are fixed here.

## Acceptance criteria

- [x] An error page whose title holds characters that grow when lowercased, such as Ⱥ, gives its title, and doesn't crash nbpdns. Covered by a test.
- [x] `powerdns check` reports a group it can't build a client for, such as one with a missing CA file, as a failed check, and checks the other groups. Covered by a test.
- [x] A DS, TLSA or SSHFP value whose digest is given in chunks of digits isn't reported as out of range, and is normalized. Covered by a test.
- [x] The root zone's ID, `.`, stays in its URL. Covered by a test.
- [x] A 404 from the server endpoint names both the server ID and the URL as what to check.
- [x] The credentials-in-URL error fits both NetBox's URL and a PowerDNS group's.
- [x] `Connect` and `powerdns check` use one release check, with the client's own supported list.
- [x] Zone-file text is split into fields by one tokenizer.
- [x] `netbox zones --group` reads a group's views in one query.
- [x] A secret that YAML reads as a number, such as `1e10`, is an error, for the NetBox token and for API keys. Covered by tests.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: The review ran on `origin/main...m02-powerdns-read-path` at
  `c5b4bea`. Findings and what became of each:
  1. `TextDetail` searched `strings.ToLower(s)` for the title, then sliced
     `s` with its offsets, but some characters take more bytes in
     lowercase, so the slice could run past the end: a panic, reproduced by
     the reviewer. Fixed: only ASCII letters are lowercased for the search,
     so the offsets match.
  2. `powerdns check` returned when it couldn't build one group's client,
     so no group was reported. Fixed: that group fails its `connection`
     check with the reason, and the others are checked.
  3. `changedNumber` compared fields by position, but miekg/dns prints hex
     or base64 given in chunks as one field, so a DS digest `1234 5678` was
     reported as out of range. Fixed: the comparison stops at a printed
     field that starts with an input field and the next one, which a
     number kept modulo its size can't do. (A first try stopped at any
     printed field that starts with its input, which a relative name made
     absolute also does, and so hid an SOA serial out of range; the tests
     caught it.)
  4. `ZoneData` built the zone's URL with `JoinPath`, which cleans the path
     and so dropped the root zone's ID, `.`. Fixed: the ID is appended as
     an escaped segment.
  5. A 404 from the server endpoint always blamed `server_id`. Fixed: the
     error names the URL asked for, and says to check both `server_id` and
     that `url` is the address in front of `/api/v1`.
  6. The credentials-in-URL error said "set the token instead", which a
     PowerDNS group doesn't have. Fixed: "set the token or API key in its
     own key".
  7. `Server.Check` used the global `Supported` while `Connect` used the
     client's list. Fixed: `Client.Check` does both jobs, with the client's
     list, and `Server.Supported` is gone.
  8. `fields` was a second hand-written tokenizer beside `unescape`. Fixed:
     `fields` finds each field's end with `unescape`.
  9. `netbox zones --group` listed each view in turn. Fixed:
     `ZoneFilter.Views` takes a list, sent as repeated `view=` parameters,
     which the plugin's filter accepts; checked against the lab.
  10. A secret that YAML reads as a number was turned into Go's text for it,
      so `1e10` became `1e+10` and stopped matching. Fixed for the NetBox
      token and for API keys: a secret must be a string, and the error says
      to quote it.
  `make check` and `make test-integration` pass.
