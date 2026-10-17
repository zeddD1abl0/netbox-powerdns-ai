---
id: ITEM-0094
title: The M08 docs and CHANGELOG
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-049, REQ-050, REQ-051, REQ-052]
depends_on: [ITEM-0093]
created: 2026-10-09
closed: 2026-10-15
---

# ITEM-0094: The M08 docs and CHANGELOG

## Goal

M08's docs: the explanation "How nbpdns stores its state"; the how-tos for
settings, groups, the master key, the audit trail and drift history; the
container and install guides' database volume; the regenerated
references; and the CHANGELOG, with its `[0.8.0]` section at close.

## Acceptance criteria

- [x] The explanation and the how-tos exist, and the references are regenerated.
- [x] The CHANGELOG has M08's changes.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
- 2026-10-14: ADR-0044 tries Sonnet here once. A Sonnet 5.5 subagent drafts
  the docs and the CHANGELOG lines from the M08 diff, against the style
  guide, and Opus reviews the draft. Record how it compared: what Opus
  changed, and whether a draft saved work.
- 2026-10-15: Done. The five how-tos, the container and install guides'
  database, and the CHANGELOG's lines came with their items; the references
  were current. This item added:
  - **The explanation** "How nbpdns stores its state": the database and its
    lock, migrations, settings (linking "Configuration sources and
    precedence" for the order and pinning), groups' owners, secrets at rest,
    the audit chain, drift history, failed writes, and copies.
  - **A how-to,** "Back up nbpdns's database". M08's non-goals promised
    that its docs say how to copy the SQLite file safely, but no page did.
    The steps were run: `sqlite3 .backup` while `serve` ran, as the user
    and as root (in a container, with `setpriv` as UID 65532); a copy
    after a clean stop, which leaves no `-wal`; `audit verify` on the copy,
    on the host and through the image; and a restore, after which `serve`
    starts, the settings come back, and `audit verify` passes. Found on the
    way: a running database's main file can be 4 KB with everything in its
    `-wal`; `.backup`'s copy is 0644, so the copies' directory is 0700; and
    root's `sqlite3` gives the files it makes the database's owner.
  - **Stale pages fixed:** "Configuration sources and precedence" (four
    sources, and a future-tense section on runtime settings); "How nbpdns
    runs as a service" (any config change took a restart; "remember across
    restarts" still to come); "Check the audit trail" (retention set in the
    file, with a restart); "Run nbpdns in a container" (no warning against
    a named volume, which Docker makes root's; the errors without a volume;
    NetBox's URL and token needed with no groups; the master key);
    "Configure nbpdns" and "Connect nbpdns to PowerDNS" (no pointer to the
    settings and groups commands); the tutorial "Run nbpdns as a service",
    whose output, run again on the lab, gained four migrations, the
    groups' mirror, and the status page's database and webhooks lines.
  - Links from the M08 how-tos, the install guide and the indexes.
  - The CHANGELOG's Unreleased already covered every user-facing item
    (ITEM-0086 to ITEM-0093, ITEM-0106), so it's unchanged. Its `[0.8.0]`
    section waits for close-milestone.
- 2026-10-15: **The Sonnet trial (ADR-0044).** A Sonnet 5.5 subagent drafted
  all the prose above except the tutorial: 3 minutes, 34 tool calls,
  126k tokens, from a brief of about 1,400 words. It added 374 lines. Opus
  then checked each claim against the code and the ADRs, and changed:
  - **The explanation,** 40 of 235 lines. Every fact it took from the code
    held: the 5 s busy timeout and poll, which commands take the lock, the
    downgrade refusal, what a reload applies when, GCM's additional data,
    the verifier, the prune's anchor, the fingerprint. Opus fixed an
    overclaim (a failed history write loses "nothing for good"), gave the
    reason `netbox.url` and `netbox.ca_file` are bootstrap keys, which it
    had flagged as unsure, added why each group has one owner and what
    renaming in the file does (ADR-0045), rewrote a muddled GCM sentence,
    and replaced a weak diagram with one of who writes the database.
  - **The backup how-to,** 65 of 119 lines, rewritten after running it.
    Its untested steps would have failed or leaked: `sudo -u '#65532'`,
    which sudo refuses for a UID with no user by default; copies left
    0644; a copy made as the service's user into a directory it can't
    write; and a restore that owned the file by 65532 on a host install.
    It had listed each of these as unsure.
  - **The precedence page,** 32 of its 45 new lines: "runtime setting"
    where the reference says "runtime key", a bootstrap definition that
    didn't fit `netbox.url`, and pinning that ignored the restart.
  - Small fixes elsewhere: three paragraphs wrapped past 80 columns, a
    link to the backup guide, and wording. Five link edits were kept as
    they were.
  - **Did it save work?** For the explanation and the stale pages, yes:
    the structure, most of the prose and the code lookups held, and
    reviewing took less than writing would have, though Opus still read
    the code behind every claim. For the backup how-to, no: it had to be
    run anyway, and half of it was rewritten. Its report was accurate, and
    named every claim it couldn't confirm, which made the review faster.
    The draft was good enough to keep for prose that rests on code Opus
    must read anyway; it doesn't replace running the steps.
