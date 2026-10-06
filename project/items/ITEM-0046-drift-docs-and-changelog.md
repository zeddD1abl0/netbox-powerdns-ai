---
id: ITEM-0046
title: Drift docs and CHANGELOG
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M03
requirements: [REQ-031]
depends_on: [ITEM-0044]
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0046: Drift docs and CHANGELOG

## Goal

The M03 docs: the tutorial "Find drift between NetBox and PowerDNS", the
how-to "Set a zone's drift policy", the explanation "How nbpdns finds drift",
the generated references, and the CHANGELOG.

## Acceptance criteria

- [x] The tutorial, how-to and explanation exist, each checked against the lab.
- [x] The generated references are current, and `CHANGELOG.md` has lines under Unreleased.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M03's approved design.
- 2026-10-06: Done. New pages: the tutorial "Find drift between NetBox and
  PowerDNS" (a zone missing on the primary, each kind of RRset change, the
  fix by hand, an unmanaged zone, `--zone`, and the exit statuses), the
  how-to "Set a zone's drift policy", and the explanation "How nbpdns finds
  drift" (served data, zone states as a Mermaid flowchart, RRset changes,
  the SOA serial, unmanaged and inactive zones, policies, problems,
  failures and exit statuses, scale with the benchmark figures, and what
  isn't done yet). The section indexes list them, and the CHANGELOG has a
  documentation line beside ITEM-0044's command line. The references were
  regenerated with ITEM-0042, ITEM-0044 and ITEM-0047. Every output in the
  tutorial and the how-to comes from the lab: the tutorial's own shell and
  YAML blocks were extracted and run in order on an empty lab, and gave
  the output the page shows. Writing the docs against ADR-0027 found that
  the table didn't mark `enforce` zones, which became ITEM-0047. `make
  docs-links` passes.
