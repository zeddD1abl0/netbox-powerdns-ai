---
id: ITEM-0056
title: Fix the M04 code review findings
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: [REQ-014, REQ-044]
depends_on: [ITEM-0054]
created: 2026-10-07
closed: 2026-10-07
---

# ITEM-0056: Fix the M04 code review findings

## Goal

`/code-review high` on `origin/main...m04-service`, at `98e8bf7`, found
eight things. This item fixes them.

## Acceptance criteria

- [x] A refresh stopped by `drift.timeout` is failed for that reason, and blames neither NetBox nor a primary.
- [x] Bad `otlp.headers` are a config error, and `config show` works whatever the OTLP settings.
- [x] `serve` tries again, each refresh, to make a client for a group that has none.
- [x] `nbpdns_netbox_up` and the refresh timestamps have no series before the first refresh.
- [x] NetBox and each primary are checked once, until a check succeeds, not on every refresh.
- [x] Every log line carries the IDs, the OTel error handler's, the last flush's, and the HTTP server's included.
- [x] The zone states and change kinds are listed once, in `internal/drift`.
- [x] `drift.group_concurrency`'s description says NetBox is read once for all groups.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Done. The findings, and what changed:
  1. **A refresh stopped by `drift.timeout` blamed what it was reading:**
     NetBox down if it was reading NetBox, the groups failed if it was
     reading them. A refresh whose context ran out of time, and that didn't
     finish complete, now gets a `TimeoutError`: it's failed, NetBox's and
     every group's state stay as they were, the log says so, and `/status`
     gives the reason in the new field `schedule.last_refresh.error`, which
     also holds NetBox's error for a refresh that couldn't read it. The
     reference page documents the field.
  2. **Bad OTLP settings stopped `config show`, and headers were only
     checked when exporting.** The header parser is now
     `config.ParseHeaders`, and the loader checks `otlp.headers` with it,
     through a new check argument of `secretKey`, so a bad header is a
     config error like any other, naming the pair, not its value.
     `config show` is annotated to make no exporter, so a missing
     `otlp.ca_file` can't stop it either. A short command's wait for its
     last spans, up to `otlp.timeout` with an unreachable collector, is
     documented behavior and stays.
  3. **A group whose client failed at startup stayed failed.**
     `retryClients` tries again before each refresh of `serve`, and logs
     when a group has a client.
  4. **`nbpdns_netbox_up` and the refresh times read 0 before the first
     refresh**, which looked like NetBox down and a refresh in 1970. They're
     gauge vectors with no labels now, which have no series until set.
  5. **Every refresh checked NetBox and each primary again**, and repeated
     an unsupported-release warning each time. `netboxConn` and
     `groupClient` remember a successful check; a failed one is tried again
     next time.
  6. **Three log lines lacked the IDs.** The session makes the provider and
     the root span first, then the exporter, so its `http://` warning, the
     batcher's error handler and the last flush all log with the command's
     context; a span reaches the processors registered when it ends, so
     the root span is still exported. The HTTP server's error log goes
     through `logging.Bound`, which handles every record with the serve
     command's context.
  7. **The zone states were listed in four places.** `drift.States`,
     `drift.DriftedStates`, `drift.ChangeKinds`, `drift.IsDrifted` and
     `Counts.ByState` are the one list; the metrics, the service and the
     status page use them, and `drifted(g)` is computed once per group.
  8. **`drift.group_concurrency`'s description** says NetBox is read once
     for all the groups together.
  New tests: two timeouts in a row keeping NetBox and the group up, with
  the reason on `/status`; a bad `otlp.headers` at load, without its value;
  the unlabelled gauges absent before a refresh; a group whose CA file
  appears after startup getting a client; a primary checked once over two
  listings; a command's `http://` warning carrying its IDs; `config show`
  exporting nothing, even with a missing CA file; and `logging.Bound`
  through a `log.Logger`.
