---
id: ITEM-0107
title: Fix the findings of M08's code review
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-049, REQ-050]
depends_on: []
created: 2026-10-17
closed: 2026-10-17
---

# ITEM-0107: Fix the findings of M08's code review

## Goal

`/code-review high` at M08's close, on `main...m08-sqlite-persistence`,
found ten things. Seven are fixed here: a data race between serve's
refreshes and its watcher, NetBox shown as up by a refresh that never read
it, a failed reload that's never retried, a group's state handed to a new
group of the same name, background work outliving the database it uses,
an audit prune that can delete an event younger than its retention, and
three readers of a secret file. The other three are left, each with its
reason in the notes and in M08's verification log. `/security-review`
found nothing at its bar.

## Acceptance criteria

- [x] serve's watcher never reads the session's configuration, which each
  refresh changes; it opens stored secrets with the master key read at
  serve's start.
- [x] A full refresh that compared no server group leaves NetBox's state as
  it was: `/status` shows it as not read yet, and `nbpdns_netbox_up` isn't
  set, until a refresh reads NetBox.
- [x] A settings or groups reload that fails is tried again at the next
  poll, until one succeeds.
- [x] A group removed and a new one of the same name added before serve
  applies them don't share state: the new one starts with none.
- [x] serve waits for its prune and its watcher to stop before it records
  its stop and closes the database.
- [x] The audit prune deletes only the events before the first one at or
  after its cutoff, so no event younger than `audit.retention_days` goes.
- [x] `nbpdns settings set --from-file`, `groups --api-key-file` and the
  `_FILE` keys read a secret file with one function;
  `secrets rotate-master-key --new-key-file` uses it too.
- [x] Each fix has a test that fails without it, where `-race` or a
  table-driven test can show it.
- [x] `make check` is green.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-17: The findings left unfixed, and why:
  - A refresh that `RefreshNow` brought forward, after a group change, is
    stored with cause `schedule`. That's by design: `RefreshNow` brings the
    scheduled refresh forward, and the schedule counts from it. A cause of
    its own would be a new value of the API's closed `cause` enum, an
    incompatible change (ADR-0012). The `group.*` audit event, seconds
    before the refresh, says why it ran.
  - `diffGroups` in `internal/cli` and `Service.SetGroups` both match
    groups by ID. They can't share one implementation: the service sees
    only `service.Group`, which lacks the connection settings that decide
    whether a group changed. After this item's fix, both treat a reused
    name the same way, as a new group.
  - Commit 3ffda43, "Update .gitignore", has no Conventional Commit type or
    `Refs` trailer. It's the user's own commit, already pushed, so the
    history stays as it is.
- 2026-10-17: Fixed, each with a test that fails without its fix, but two:
  - The race: serve's watcher has its own copy of the session, so
    `reconfigure`'s write of `s.cfg` on the refresh goroutine and the
    watcher's read of `database.master_key` no longer share a field. No
    test: the race needs a reload during a refresh's reads, after its
    `reconfigure`, and a test that triggers it in-process orders the two
    through SQLite's own mutexes, so `-race` can't see it.
  - NetBox's state: the service keeps whether a refresh has read NetBox,
    or tried to, beside whether it's up. A full refresh with no groups,
    and a zone refresh of zones that no group serves, which `drift.Run`
    returns before reading NetBox, leave it as it was, as the restore
    does with no reports. `TestRefreshWithNoGroups`. The second case was
    so since v0.7.0, so CHANGELOG has a Fixed line.
  - The retry: `reloadSettings` and `reloadGroups` report whether they
    applied, and the watcher moves its version on only then.
    `TestServeRetriesAFailedReload`, which stores a value that doesn't
    parse, sees the failure, mends it without moving the version on, and
    sees it applied; it times out without the fix.
  - A reused name: `SetGroups` keeps a group's state only for a name whose
    ID is the same. `TestSetGroups` gains the case.
  - Background work: serve waits for the prune and the watcher, through a
    `sync.WaitGroup`, before `closeStore`. No test: cancellation stops
    both at once, so a check that they've stopped once serve returns
    passes without the fix too.
  - The prune: `LastAuditEventBefore` takes the last event before the
    first one at or after the cutoff, by seq, with `NOT EXISTS`, which
    sqlc's PostgreSQL engine compiles. `TestPruneAuditKeepsAYoungerEvent`;
    the old query deleted the younger event.
  - Secret files: `config.ReadSecretFile`, exported, reads `--from-file`,
    `--api-key-file`, the `_FILE` keys and `--new-key-file`; standard input
    keeps its own read, capped at 64 KiB.
  `nbpdns_netbox_up`'s help, the API's `netbox.up`, "Service endpoints"
  and "How nbpdns stores its state" say what changed, and the references
  are regenerated. `make check` is green, and `make test-integration` and
  `make release-check` passed, with `GOFLAGS=-count=1`.
