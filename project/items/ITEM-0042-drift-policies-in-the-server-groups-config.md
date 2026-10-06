---
id: ITEM-0042
title: Drift policies in the server groups' config
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M03
requirements: [REQ-031]
depends_on: []
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0042: Drift policies in the server groups' config

## Goal

Each server group's drift policy, and its overrides per zone, are set in the
config file (ADR-0027): `drift_policy`, `report` if unset, and
`zone_policies`, from zone name to `enforce`, `report` or `ignore`.

## Acceptance criteria

- [x] `drift_policy` and `zone_policies` are fields of a server group, decoded strictly: a policy must be one of the three, zone names follow the NetBox zone name rules, and a name listed twice is an error.
- [x] `config show` lists them, and the configuration reference documents them.
- [x] Table-driven tests cover them.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M03's approved design.
- 2026-10-06: Done. `config.Group` gains `DriftPolicy` (`report` unless the
  entry sets it) and `ZonePolicies`, keyed by absolute lowercase zone
  names, so that they match the model's zone names; `Group.Policy(zone)`
  returns a zone's policy. The policy names are constants with
  `config.Policies`. Zone names follow the NetBox zone name rules (ASCII,
  with or without a final dot), and a name that two keys normalize to, such
  as `example.com` and `example.com.`, is an error. `config show` lists the
  overrides as `example.com=enforce,legacy.example.com=report`. The
  reference's example shows both fields, and the CHANGELOG's server groups
  line names them.
