---
id: ITEM-0116
title: 'The takeover tests: stopped, killed and cut-off leaders'
type: task # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033]
depends_on: [ITEM-0115]
created: 2026-10-18
closed:
---

# ITEM-0116: The takeover tests: stopped, killed and cut-off leaders

## Goal

The takeover tests, ADR-0009's and ADR-0047's confirmation: three
replicas as child processes, the test binary running itself as `nbpdns
serve`, on a fresh lab database, with a 1-second poll. A leader is
stopped, killed, or cut off by a TCP proxy in the test that stops
forwarding but keeps its sockets open.

## Acceptance criteria

- [ ] Exactly one replica refreshes.
- [ ] A leader stopped with SIGTERM, or killed with SIGKILL, is replaced within 5 seconds.
- [ ] A cut-off leader logs its step-down before another replica takes over, within 40 seconds.
- [ ] A webhook sent to a follower, and a setting changed with the CLI, reach every replica.
- [ ] The stopped and webhook cases also pass with `database.listen` on.
- [ ] The integration job's peak memory is recorded, and fits the runner.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
