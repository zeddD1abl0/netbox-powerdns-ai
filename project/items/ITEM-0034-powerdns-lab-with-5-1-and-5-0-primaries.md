---
id: ITEM-0034
title: PowerDNS lab with 5.1 and 5.0 primaries
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-036, REQ-041]
depends_on: []
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0034: PowerDNS lab with 5.1 and 5.0 primaries

## Goal

M02's integration tests read from real PowerDNS servers. Add two server
groups to the lab, each with a primary only: `lab-a` on PowerDNS 5.1 and
`lab-b` on 5.0, as ADR-0024 decides, within the runners' memory.

## Acceptance criteria

- [x] `make lab-up` starts PowerDNS 5.1 on port 8151 and 5.0 on port 8150, from the official images pinned by digest, with the SQLite backend, a published lab-only API key and no bind mounts, and waits until they're healthy.
- [x] `internal/lab` lists them, with a smoke test of each one's version, and `TestSupportedMatchesLab` covers PowerDNS.
- [x] Fixtures create a zone per test through the API, with varied types and a disabled record, and remove it afterwards.
- [x] The emulated CI job's memory, before and after, is recorded here, and the GitLab job's comment and the lab how-to are updated.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M02's approved design.
- 2026-10-06: Done.
  - **The lab** gains `powerdns-51` (PowerDNS 5.1.4, port 8151, group
    `lab-a`) and `powerdns-50` (5.0.7, port 8150, group `lab-b`), from the
    official `powerdns/pdns-auth-51` and `-50` images (Debian trixie),
    pinned by digest. `PDNS_AUTH_API_KEY` alone makes the image's startup
    script turn on the web server and API, listening on every interface and
    allowed from anywhere, so there are no flags and no bind mounts. The
    image ships its SQLite database, which lives in the container, so
    `lab-down` removes the zones. The image has no curl, so the healthcheck
    asks the API through its Python. Both are healthy about a second after
    they start.
  - **Checked by hand** against both images before writing the lab: the
    server reports `daemon_type` and `version`; a wrong key gets 401; an
    unknown server ID gets 404, as JSON on 5.1 and as plain text on 5.0;
    the API refuses relative names, keeps a name's case, keeps TLSA hex in
    lowercase and HTTPS's `alpn` unquoted, and returns disabled records with
    a `disabled` flag; a new zone without an SOA gets
    `a.misconfigured.dns.server.invalid.`, so fixtures set their own.
  - **`internal/lab`:** `PowerDNSes` and `PowerDNSAPIKey`, the lab's
    published key, which `.gitleaks.toml` now allows besides the NetBox
    token. PowerDNS has one key per server, so fixtures write with the key
    nbpdns reads with. `NewPowerDNSFixture` creates a zone per test with
    every type the normalization covers, mixed case, a disabled record in
    an active RRset, a disabled-only RRset and a 300-byte TXT, plus two
    small zones, and deletes all three afterwards. `TestLabPowerDNSes`
    checks each server's type and release, and that a fixture can be
    created. `TestSupportedMatchesLab` for PowerDNS is in
    `internal/powerdns`, which ITEM-0035 adds.
  - **Measured** in the emulated CI job (ITEM-0028's method), on the full
    M02 tree with the PowerDNS integration tests:

    | | NetBox only (ITEM-0028) | With PowerDNS 5.1 and 5.0 |
    |---|---|---|
    | Docker-in-Docker and the lab, resident | 1,119 MiB | 1,279 MiB |
    | Docker-in-Docker and the lab, with cache | 3,326 MiB | 4,131 MiB |
    | Job container, resident | 323 MiB | 230 MiB |
    | Duration | 299 s | 297 s |

    Each PowerDNS server holds about 48 MiB. With hard limits of 1.5 GiB on
    the service and 768 MiB on the job, as before, the job passed in 307 s,
    with a resident peak of 1,263 MiB, so about 270 MiB of headroom is left.
    The two images add about 0.5 GB of disk, so Docker-in-Docker stores
    about 2.5 GB. The GitLab job's comment and the lab how-to say so.
