---
id: ITEM-0091
title: Server groups in the database, with managed_by
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-050]
depends_on: [ITEM-0090, ITEM-0092]
created: 2026-10-09
closed: 2026-10-15
---

# ITEM-0091: Server groups in the database, with managed_by

## Goal

Server groups as managed resources (ADR-0040): the file's groups mirrored
at start as `managed_by=file`, audited; `nbpdns groups list|add|set|remove`
for `managed_by=cli`; a name both claim stops `serve`; `serve` applies group
changes within seconds; and `managed_by` in the API.

## Acceptance criteria

- [x] The file's groups are mirrored, read-only to the CLI, and removed when the file drops them.
- [x] A group added with the CLI is compared by a running `serve` within seconds, and one removed leaves its state and metrics.
- [x] `managed_by` is in the API's server groups.
- [x] A removed group's drift history and refreshes are kept until their retention.
- [x] A CLI group's API key comes from `--api-key-file` or an environment variable, never the command line, and is sealed; the master key's check and rotation cover it.
- [x] `groups add` and `groups set` check that the group's servers answer when the change touches how nbpdns reaches them, and not otherwise.
- [x] A group can be renamed, and whatever a rename would break is found and fixed.

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
- 2026-10-14: Started. The plan, agreed with the user: a `server_groups`
  table (migration 00005) with an internal ID, `managed_by`, the fields as
  JSON and a CLI group's sealed key; `nbpdns groups list|add|set|remove|rename`,
  the key from `--api-key-file` or `NBPDNS_GROUP_API_KEY`, and `--no-check`;
  serve's watcher reads the groups' version and applies them live; a rename
  moves the group's name-keyed history rows (ADR-0045). The brief's answers
  of 2026-10-14, "ITEM-0091's plan", settle the plan's questions.
- 2026-10-15: Done. Migration 00005 adds `server_groups`; the store mirrors
  the file's groups, and adds, sets, removes and renames CLI groups, each
  with a `group.*` event and a bump of `groups_version`; `checkMasterKey`
  and `RotateMasterKey` cover the groups' sealed keys. `RecordRefresh`
  writes only the groups that the database stores, and the prune ages out
  a removed group's last zone rows. serve's watcher applies the groups
  live (`Service.SetGroups`, `RefreshNow`), and `serve` starts with no
  groups, when `drift.Run` reads nothing. A rename moves the group's
  history rows (ADR-0045). The integration test of a CLI group against the
  lab is ITEM-0093's.
