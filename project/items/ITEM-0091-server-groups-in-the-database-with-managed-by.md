---
id: ITEM-0091
title: Server groups in the database, with managed_by
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-050]
depends_on: [ITEM-0090, ITEM-0092]
created: 2026-10-09
closed:
---

# ITEM-0091: Server groups in the database, with managed_by

## Goal

Server groups as managed resources (ADR-0040): the file's groups mirrored
at start as `managed_by=file`, audited; `nbpdns groups list|add|set|remove`
for `managed_by=cli`; a name both claim stops `serve`; `serve` applies group
changes within seconds; and `managed_by` in the API.

## Acceptance criteria

- [ ] The file's groups are mirrored, read-only to the CLI, and removed when the file drops them.
- [ ] A group added with the CLI is compared by a running `serve` within seconds, and one removed leaves its state and metrics.
- [ ] `managed_by` is in the API's server groups.
- [ ] A removed group's drift history and refreshes are kept until their retention.
- [ ] A CLI group's API key comes from `--api-key-file` or an environment variable, never the command line, and is sealed; the master key's check and rotation cover it.
- [ ] `groups add` and `groups set` check that the group's servers answer when the change touches how nbpdns reaches them, and not otherwise.
- [ ] A group can be renamed, and whatever a rename would break is found and fixed.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
- 2026-10-10: Now after ITEM-0092, whose sealing a CLI-managed group's API
  key needs; ITEM-0092 no longer depends on this item.
- 2026-10-14: The user answered the questions this item's design left
  open (the brief's answers of 2026-10-14). A removed group's history is
  retained, until `drift.history_retention_days` prunes it. A CLI group's
  API key comes from `--api-key-file` or an environment variable; which
  variable is this item's to choose. `add` and `set` check that the
  servers answer when the change touches them, such as their URL or API
  key, and not for other changes. Renaming must not break anything
  critical: find what a rename would break, such as history and audit
  targets keyed by the group's name, and fix it. ITEM-0092's
  `checkMasterKey` and `RotateMasterKey` read only the settings' sealed
  values: extend both to the groups' API keys.
