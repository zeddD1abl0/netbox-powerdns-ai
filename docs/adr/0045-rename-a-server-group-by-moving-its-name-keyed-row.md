---
title: '0045: Rename a server group by moving its name-keyed rows'
status: accepted # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-10-14
decision-makers: [jordan]
requirements: [REQ-050, REQ-052] # REQ-nnn this decision serves
questions: [] # Q-nnn this decision answers
supersedes: # ADR-NNNN this replaces, if any
---

# 0045: Rename a server group by moving its name-keyed rows

## Context and problem statement

From M08, server groups live in the database, and a group added with
`nbpdns groups add` is changed with the CLI (ADR-0040). On 2026-10-14 the
user asked for groups to be renamed too, and for whatever a rename would
break to be found and fixed. ADR-0040 doesn't cover renames, and until now
the API has called a group's name "its stable ID".

A group's name is the key of everything nbpdns keeps about it: its rows in
the drift history and its last report (ADR-0042), the targets of its audit
events (ADR-0039), its metrics' `group` label, and the URLs of its API
resources. Its API key, sealed with the master key (ADR-0041), is bound to
the place it's stored, which a name would make a rename change.

## Decision drivers

- A renamed group keeps its drift history, and the next refresh compares
  its zones with their last rows, so that a rename makes no false changes.
- The audit trail is hash-chained, so no event can be rewritten.
- A rename shouldn't need the master key.
- A group moved from the config file to the CLI, or back, keeps its history,
  as the user expects of a name.
- No API change that breaks a client.

## Considered options

1. Keep the history keyed by name, and move a group's rows when it's
   renamed.
2. Key the history by a stable group ID, and resolve names to IDs.
3. Don't support renames: remove the group and add it again.

## Decision outcome

Chosen: **option 1**, keyed by name, with the rows moved at a rename.

- **An internal ID:** each group in `server_groups` has an ID that nbpdns
  assigns from a sequence and never reuses. A CLI group's sealed API key is
  bound to the place `server-group:<id>`, so a rename re-seals nothing, and
  needs no master key. The ID isn't shown anywhere yet.
- **The history follows the name:** `nbpdns groups rename OLD NEW` changes,
  in one transaction, the group's row, the name in `drift_refresh_groups`,
  `drift_zone_history` and `drift_group_reports`, records a
  `group.renamed` event, and bumps the groups' version.
- **The audit trail isn't rewritten:** events before the rename keep the old
  name, and `group.renamed` links the two names.
- **A running `serve`** matches the group by its ID, moves its state to the
  new name, drops the old name's metric series and sets them under the new
  one, and makes a new client, whose request metrics are labelled by name.
  The rows of a refresh are written only for groups that `server_groups`
  holds, in the transaction that writes them, so a refresh that crosses a
  rename can't write under the old name.
- **Refused:** renaming a file-managed group, which is renamed in the file;
  a new name that another group has, or that the config file declares; and
  a new name that a removed group's kept history still uses, since two
  histories would merge.
- **A name's history:** a group removed keeps its history until
  `drift.history_retention_days` prunes it. A group added under that name,
  with the CLI or in the file, continues it.
- **Renaming in the config file** is a removal and an addition: the history
  stays under the old name until it's pruned.
- **The API** still addresses a group by its name, its identifier, which a
  rename changes; the spec no longer calls it stable. A page cursor that
  names the old name gets the 400 it gets for a removed group.

### Consequences

- Good: a renamed group's history, last report and drift states carry on,
  and the history API and `nbpdns drift history` read them under its new
  name.
- Good: moving a group between the config file and the CLI keeps its
  history.
- Good: no new field in the API, and no migration of the history tables.
- Bad: a rename rewrites the history's group names, so the history shows the
  new name for refreshes from before it; the audit trail keeps the old one.
- Bad: dashboards and alerts on the old `group` label need updating.
- Bad: a rename can't take a name that a removed group's history uses,
  until that history is pruned.
- Bad: Terraform (M19) will want an ID that doesn't change; this ID can be
  exposed then.

### Confirmation

- Store tests rename a group and read its history, last report and zone
  rows under the new name, and check that a refresh after the rename writes
  no false changes.
- Store tests refuse a rename into a name with kept history, and check that
  the rows of a refresh for a group that isn't in `server_groups` aren't
  written.
- Service tests check that a rename moves the group's state and metrics.

## Pros and cons of the options

### Key the history by a stable group ID

- Good: a rename changes one row.
- Bad: the history tables, from ADR-0042, would have to be rebuilt with an
  ID column, and their rows mapped to IDs before any group is in the
  database.
- Bad: a group removed and added again, such as one moved from the config
  file to the CLI, would get a new ID, and lose its history.
- Bad: a removed group's history could be read only by an ID nobody sees.

### Don't support renames

- Good: nothing to build.
- Bad: the user asked for renames, and a removal and an addition lose the
  group's continuity: its history restarts, and every drifted zone is a new
  `zone.drift_changed` event.

## More information

- ADR-0039 (audit), ADR-0040 (managed resources), ADR-0041 (secrets),
  ADR-0042 (drift history).
- The user's answers of 2026-10-14, in `project/brief.md`. Implemented by
  ITEM-0091.
