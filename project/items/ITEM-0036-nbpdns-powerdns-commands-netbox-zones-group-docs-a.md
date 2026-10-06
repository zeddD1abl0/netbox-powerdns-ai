---
id: ITEM-0036
title: nbpdns powerdns commands, netbox zones --group, docs and CHANGELOG
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-041, REQ-042]
depends_on: [ITEM-0033, ITEM-0035]
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0036: nbpdns powerdns commands, netbox zones --group, docs and CHANGELOG

## Goal

Make M02 usable end to end: `nbpdns powerdns check`, `zones` and `records`,
`nbpdns netbox zones --group`, and the docs from M02's design, with the
CHANGELOG.

## Acceptance criteria

- [x] The commands work as in M02's design, as tables and as JSON, with exit statuses 0, 1 and 2 and clear messages.
- [x] `nbpdns netbox zones --group G` lists only the zones in G's views, and reports a zone name that's in two of them.
- [x] The docs exist: the tutorial "Read your PowerDNS zones with nbpdns"; the how-to guides "Connect nbpdns to PowerDNS" and "Put the PowerDNS API behind a TLS proxy", the latter checked by hand with a client certificate; "How nbpdns reads PowerDNS"; and the lab how-to and supported versions reference, updated.
- [x] `CHANGELOG.md` has lines under Unreleased.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M02's approved design.
- 2026-10-06: Done.
  - **`nbpdns powerdns check`** prints one row per check per group
    (connection, server, key, zones), as a table with a GROUP column or as
    JSON, with an overall and a per-group `ok`. Any failed row exits 1; an
    `http://` URL only warns. A group's checks stop early when its API can't
    be reached, rejects the key, or isn't an authoritative server; an
    unsupported release is a failed row, and the zones are still listed.
  - **`zones`** lists every group's zones, or one group's with `--group`,
    sorted by group and canonical name. **`records --zone Z [--group G]`**
    needs `--group` when more than one group is declared, and shows the
    RRsets with each record's status; JSON adds the group and `problems`
    (never null). No groups, or an unknown group, is an error naming the
    groups.
  - **`netbox zones --group G`** reads the zones of each of G's views, and
    logs a warning for each zone name in more than one of them. `--view`
    and `--group` together are a usage error, since a group chooses its
    views. Without `--group`, the command is unchanged.
  - **Found while checking the TLS proxy how-to:** an error answer from a
    proxy is an HTML page, which made a multi-line detail. Both clients
    now take an HTML page's title, or the text on one line
    (`httpclient.TextDetail`), so nginx's refusal reads "400 No required
    SSL certificate was sent".
  - **Docs, each checked by running it:**
    - the tutorial "Read your PowerDNS zones with nbpdns", run step by step
      against the lab, its outputs pasted from that run;
    - "Connect nbpdns to PowerDNS"; its `pdnsutil hash-password` step was
      checked on PowerDNS 5.1: the server keeps the hash, a client sends
      the plain key, and the hash itself is refused;
    - "Put the PowerDNS API behind a TLS proxy", checked with a temporary
      nginx 1.29 in front of lab-a's primary, a test CA made with openssl,
      and a client certificate: with every file, `powerdns check` passes
      over `https://`; without the client certificate, it fails with the
      proxy's 400; without the CA file, with nbpdns's certificate error;
      `nginx -t` and the reload work as written;
    - "How nbpdns reads PowerDNS", and the section indexes and reference
      table.
  - **Tests:** unit tests for the exit codes and messages, the check of an
    unreachable primary, the records table and the shared-name problem;
    integration tests run every command against both lab primaries, with a
    wrong key and a missing zone, and `netbox zones --group` against the
    lab's NetBox with one and with two views.
