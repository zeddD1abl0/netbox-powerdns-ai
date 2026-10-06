---
id: ITEM-0044
title: nbpdns drift command
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M03
requirements: [REQ-031, REQ-043]
depends_on: [ITEM-0043]
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0044: nbpdns drift command

## Goal

`nbpdns drift` runs the comparison for every group, or one with `--group`,
or one zone with `--zone`, and reports it as a table or JSON, with the exit
codes of ADR-0027.

## Acceptance criteria

- [x] The command reads through adapters for the NetBox and PowerDNS clients, and `--zone` reads only that zone on both sides.
- [x] The table gives a summary per group, the changes of each zone that drifted, then the unmanaged zones, ignored zones and problems; JSON gives the whole report.
- [x] It exits 0 with no drift, 3 with drift, 1 when something couldn't be read, and 2 on a usage error, and the command-line reference lists every command's exit codes.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M03's approved design.
- 2026-10-06: Done. `internal/cli/drift.go` adds the command and the
  adapters `netboxSource` and `primarySource`; `cli.go` gains `exitDrift`
  and `exitCode`, and the reference's exit status table lists 3. A group
  whose client or `Connect` fails is passed to `drift.Run` as failed, so the
  others are still compared. Smoke-tested against the lab: a changed A
  record exits 3, with an unmanaged zone listed and not counted; in sync
  exits 0; an unreachable second group exits 1 with lab-a still reported;
  an unknown flag exits 2. The smoke test showed that `--zone` with a
  mistyped name reported an empty "in sync" and exited 0, so `drift.Run`
  now returns a `ZoneNotFoundError` when every group was read and neither
  side has the zone, as `powerdns records --zone` does; with a failed
  group, the run is incomplete instead. ITEM-0045's fixture covers every
  case in the lab.
