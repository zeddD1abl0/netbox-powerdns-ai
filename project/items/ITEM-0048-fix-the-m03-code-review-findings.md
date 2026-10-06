---
id: ITEM-0048
title: Fix the M03 code review findings
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M03
requirements: [REQ-031, REQ-043]
depends_on: [ITEM-0047]
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0048: Fix the M03 code review findings

## Goal

`/code-review high` on `origin/main...m03-drift-report`, at `089121c`,
found eight things. This item fixes seven; the eighth, comparing groups
concurrently, is ITEM-0049.

## Acceptance criteria

- [x] `--zone` doesn't warn about the `zone_policies` of other zones.
- [x] Two `zone_policies` keys that differ only in case are an error, not one silently chosen.
- [x] A group's report lists only the problems of the zones it compares, by view as well as name.
- [x] The help text and the CHANGELOG say that nothing is reported when NetBox can't be read.
- [x] The table lists the report's warnings, as the JSON does.
- [x] The exit status follows the report's `complete` and `drift`, with drifted zones counted in one place.
- [x] `--zone` and `zone_policies` check zone names with one function.
- [x] NetBox's records aren't read for a group that couldn't be listed, or for a zone its primary doesn't have.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Done. The findings, and what changed:
  1. **`--zone` warned about every other zone's policy.** `Compare` saw only
     the named zone, so every other `zone_policies` entry looked unserved.
     `Run` now narrows each group's policies to the named zone
     (`forZone`). `TestRun` and the integration test's `--zone` cases check
     that there are no warnings, and that the zone's own policy still
     applies.
  2. **Case-only duplicates in `zone_policies` were merged silently.**
     Viper lowercases map keys, so `Example.com` and `example.com` reached
     `decodeGroup` as one, with either policy. `zonePolicyCollisions`
     reads the config file's own YAML, matching keys without regard to
     case as Viper does, and makes such keys an error. It uses
     `go.yaml.in/yaml/v3` directly, which was already in the build as
     Viper's YAML library (MIT and Apache-2.0), so no dependency is new.
  3. **A group's problems included other views' zones of the same name.**
     `dns.Problem` gains `View`, which NetBox's problems carry, and
     `Compare` keeps a NetBox problem only if it's of the zone compared, in
     that view. The JSON of `netbox records` gains `view` on its problems.
  4. **The help text and CHANGELOG said the readable groups are reported
     when NetBox can't be read.** Nothing is, and both now say so; the
     explanation page already did.
  5. **Warnings were only in the logs and the JSON.** The table now ends
     with "Warnings about the configuration or NetBox's zones", and the
     how-to and explanation say so. They're still logged.
  6. **The drift rule was in two places.** `Counts.DriftedZones` counts
     drifted zones, `Drifted` uses it, and `driftResult` follows the
     report's `complete` and `drift`.
  7. **`zoneKey` copied `netbox.ZoneName`.** The function is now
     `dns.ZoneName`, used by `--zone` in every command and by
     `zone_policies`; the `netbox` package and `config` no longer have
     their own.
  8. **NetBox's records were read for zones that couldn't be compared.**
     `Run` now lists every primary before reading NetBox's records, which
     it reads only for zones on a primary that listed. `TestRun` checks
     that a zone missing on the primary isn't read, and that nothing is
     read when every group fails. Comparing the groups concurrently, the
     rest of the finding, is ITEM-0049, for M04: M03's approved design
     reads groups in turn.
  Also fixed, while here: NetBox's `normalize` returned `out,
  out.SetRecords(raw)`, which relies on an evaluation order Go doesn't
  specify.
