---
id: ITEM-0116
title: 'The takeover tests: stopped, killed and cut-off leaders'
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033]
depends_on: [ITEM-0115]
created: 2026-10-18
closed: 2026-11-25
---

# ITEM-0116: The takeover tests: stopped, killed and cut-off leaders

## Goal

The takeover tests, ADR-0009's and ADR-0047's confirmation: three
replicas as child processes, the test binary running itself as `nbpdns
serve`, on a fresh lab database, with a 1-second poll. A leader is
stopped, killed, or cut off by a TCP proxy in the test that stops
forwarding but keeps its sockets open.

## Acceptance criteria

- [x] Exactly one replica refreshes.
- [x] A leader stopped with SIGTERM, or killed with SIGKILL, is replaced within 5 seconds.
- [x] A cut-off leader logs its step-down before another replica takes over, within 40 seconds.
- [x] A webhook sent to a follower, and a setting changed with the CLI, reach every replica.
- [x] The stopped and webhook cases also pass with `database.listen` on.
- [x] The integration job's peak memory is recorded, and fits the runner.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
- 2026-11-25: Done. `TestTakeovers`, in
  `internal/cli/takeover_integration_test.go`, runs each replica as a
  child process: with `CLI_TEST_REPLICA` set (not `NBPDNS_`, which config
  refuses), the package's new `TestMain` runs `Main` with the signals that
  `cmd/nbpdns` takes, instead of the tests. Each replica gets the test's
  environment without its `NBPDNS_` variables, as `run` does, plus its
  own, and reaches the lab database through a TCP proxy of its own. The
  three start at once, so the start covers concurrent migrations too.
  Cut off, a proxy forwards nothing more, either way, and answers no new
  connection, but keeps every socket open, as a hung host's kernel does,
  so only PostgreSQL's `idle_session_timeout` ends the leader's session.
  - **Polling** (`database.poll_interval` 1s): one leader, whose refresh
    the others serve, and none of the others refreshes; `settings set
    drift.interval 2m` reaches all three; a webhook posted to a follower
    is refreshed by the leader, and all three serve the zone's change;
    the leader stopped with SIGTERM, then the next killed with SIGKILL,
    each replaced by a follower, whose takeover names it as the leader
    before, and a new replica started each time to keep three; then the
    next one cut off.
  - **Listening** (`database.listen` on): the setting, the webhook and
    the stopped leader again, with the longest poll, 30s, rather than the
    design's 1s. With a 1s poll, a listening run can't show that
    notifications did the work; with 30s, only they can bring the changes
    within the checks' 5 and 15 seconds.
  - Times, from five runs, three on 2 CPUs (`taskset -c 0,1`): a stopped
    leader replaced within 25 to 550 ms, a killed one within 610 to 985
    ms, and a cut-off one within 28.8 to 29.0 s; the cut-off leader logs
    its step-down 8.8 s after the cut, about 20 s before the takeover.
    The test takes about 52 s: 42 polling, 9 listening.
  - Two mutations each failed it: the listener ignoring notifications
    (the listening run, at the setting), and a 60 s heartbeat deadline
    (the polling run: no step-down logged before the takeover).
  - The test sends SIGTERM once: a second one, after `Main` has returned
    and `stop` restored SIGTERM's default action, but before the
    race-instrumented process exits, ended a cleanly stopped replica with
    the signal (exit -1). The replica's cancel is a `sync.OnceFunc`, as a
    context's cancel can be called twice.
  - Memory, measured as ITEM-0110 did, in a user scope sampled every 100
    ms, on 2 CPUs: `make test-integration` with an empty build cache, 342
    s, peaks at 1.29 GiB, 66 s in, while compiling (ITEM-0110: 1.68 GiB on
    16 CPUs); with a warm cache, 152 s, at 0.38 GiB; `TestTakeovers` alone
    at 0.36 GiB, its test binary and three race-instrumented replicas. The
    lab's PostgreSQL, anonymous and shared, goes from 0.11 to 0.14 GiB.
    So the job's peak is still the build's, within `.heavy`'s 2Gi; the
    first pipeline with this commit shows CI's run.
  - Helpers: `served.listening`, from `startServe`, which the replicas
    share; `inSyncChanged`, the webhook event that three tests built.
  - Found: ITEM-0126. A leader cut off, or frozen, inside a write
    transaction holds the store's write lock on a pool connection, which
    has none of the session's timeouts, so no replica can take over or
    write for hours. This test cuts the leader off once its refresh is
    recorded, when only the feed's read transactions run, so it doesn't
    meet it.
  - `make check` passes; its first run failed once on ITEM-0123's flake.
    `make test-integration` passes, with an empty build cache and a warm
    one.
- 2026-11-25: ITEM-0126 moved the proxy to `internal/lab/proxy.go`
  (`lab.Proxied`), for its store test too, and gave it `Restore`.
- 2026-11-25: Pipeline 1040 (7d5f4d6, this item's commit) passed every job
  without a retry. Its integration job, 5722, ran on `talos-control-2` in
  1089 s, with `TestTakeovers` among `internal/cli`'s tests, 212 s in all
  (1039: 150 s), without an OOM kill; its pod first waited for memory, as
  `Insufficient memory`, as before. Pipeline 1041 (2e2da9f, ITEM-0126)
  passed its integration job too, 5733, on the same node, in 1101 s.
