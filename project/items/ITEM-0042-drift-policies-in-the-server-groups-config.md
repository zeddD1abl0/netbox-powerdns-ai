---
id: ITEM-0042
title: Drift policies in the server groups' config
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M03
requirements: [REQ-031]
depends_on: []
created: 2026-10-06
closed:
---

# ITEM-0042: Drift policies in the server groups' config

## Goal

Each server group's drift policy, and its overrides per zone, are set in the
config file (ADR-0027): `drift_policy`, `report` if unset, and
`zone_policies`, from zone name to `enforce`, `report` or `ignore`.

## Acceptance criteria

- [ ] `drift_policy` and `zone_policies` are fields of a server group, decoded strictly: a policy must be one of the three, zone names follow the NetBox zone name rules, and a name listed twice is an error.
- [ ] `config show` lists them, and the configuration reference documents them.
- [ ] Table-driven tests cover them.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M03's approved design.
