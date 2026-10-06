---
id: ITEM-0034
title: PowerDNS lab with 5.1 and 5.0 primaries
type: task # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-036, REQ-041]
depends_on: []
created: 2026-10-06
closed:
---

# ITEM-0034: PowerDNS lab with 5.1 and 5.0 primaries

## Goal

M02's integration tests read from real PowerDNS servers. Add two server
groups to the lab, each with a primary only: `lab-a` on PowerDNS 5.1 and
`lab-b` on 5.0, as ADR-0024 decides, within the runners' memory.

## Acceptance criteria

- [ ] `make lab-up` starts PowerDNS 5.1 on port 8151 and 5.0 on port 8150, from the official images pinned by digest, with the SQLite backend, a published lab-only API key and no bind mounts, and waits until they're healthy.
- [ ] `internal/lab` lists them, with a smoke test of each one's version, and `TestSupportedMatchesLab` covers PowerDNS.
- [ ] Fixtures create a zone per test through the API, with varied types and a disabled record, and remove it afterwards.
- [ ] The emulated CI job's memory, before and after, is recorded here, and the GitLab job's comment and the lab how-to are updated.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M02's approved design.
