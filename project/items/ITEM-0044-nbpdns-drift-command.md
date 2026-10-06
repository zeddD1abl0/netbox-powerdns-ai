---
id: ITEM-0044
title: nbpdns drift command
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M03
requirements: [REQ-031, REQ-043]
depends_on: [ITEM-0043]
created: 2026-10-06
closed:
---

# ITEM-0044: nbpdns drift command

## Goal

`nbpdns drift` runs the comparison for every group, or one with `--group`,
or one zone with `--zone`, and reports it as a table or JSON, with the exit
codes of ADR-0027.

## Acceptance criteria

- [ ] The command reads through adapters for the NetBox and PowerDNS clients, and `--zone` reads only that zone on both sides.
- [ ] The table gives a summary per group, the changes of each zone that drifted, then the unmanaged zones, ignored zones and problems; JSON gives the whole report.
- [ ] It exits 0 with no drift, 3 with drift, 1 when something couldn't be read, and 2 on a usage error, and the command-line reference lists every command's exit codes.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M03's approved design.
