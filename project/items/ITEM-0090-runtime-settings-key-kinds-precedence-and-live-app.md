---
id: ITEM-0090
title: 'Runtime settings: key kinds, precedence and live application'
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-050]
depends_on: [ITEM-0087]
created: 2026-10-09
closed: 2026-10-10
---

# ITEM-0090: Runtime settings: key kinds, precedence and live application

## Goal

Runtime settings (ADR-0040): each key a bootstrap or a runtime key, shown
in the generated reference; the database as a source between the defaults
and the file, with pinning; `nbpdns settings list|get|set|unset`, audited
with the OS user; and `serve` applying changed runtime keys within
seconds.

## Acceptance criteria

- [x] Table tests cover the precedence, pinning, and refusing a bootstrap key.
- [x] A running `serve` uses each changed runtime key within seconds, as tests show.
- [x] `nbpdns config show` names the database as a source.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
- 2026-10-10: Config: `Key.Runtime`, set in each runtime key's
  declaration (ADR-0040's list; `TestRuntimeKeys` pins it), shown as each
  key's kind in the generated reference. `Loader.Load` keeps each key's
  raw value and source from the flags, environment and file;
  `Loader.WithStored` layers the database's values between them and the
  defaults, so a runtime key set higher is pinned, its stored value kept in
  `Setting.Stored` but unused, and refuses a stored bootstrap key, an
  unknown key or an invalid value. Caching the first load's layers, rather
  than reading the file again at each reload, keeps a running serve's
  bootstrap keys and its reloads consistent. `CheckSetting` checks a value
  as the file's would be, and its errors never hold a secret.
- 2026-10-10: Store: migration 4, `settings` and the `settings_version`
  sequence; `ChangeSetting` sets or unsets a value, bumps the version and
  records `setting.set` or `setting.unset`, with the values before and
  after (`[redacted]` for a secret), in one transaction, and changes
  nothing when there's nothing to change.
- 2026-10-10: CLI: `nbpdns settings list|get|set|unset`, with `--reason`,
  and `nbpdns config show`, which applies the database's settings if the
  database exists. Secret keys are refused until ITEM-0092 encrypts them.
  The one-shot commands don't read the database (ADR-0038).
- 2026-10-10: serve: applies the stored settings before it records its
  start, so the configuration's fingerprint covers them; watches the
  version every 5 s; on a change, re-reads the settings, sets the log's
  `slog.LevelVar` and the service's new `Timing` (interval, timeout,
  webhook delay; `Run` works the next start out from the last start and
  the current interval, and `SetTiming` wakes it), and publishes the
  configuration through an `atomic.Pointer`. Each refresh applies it to
  the session first: refreshes never overlap, so that's the one goroutine
  that changes `s.cfg`, and it makes new NetBox or PowerDNS clients only if
  their settings changed. The prune loop reads the live configuration too.
  An invalid stored value is logged, and serve keeps what it has;
  `nbpdns_settings_reloads_total{outcome}` counts reloads. The how-to
  "Change nbpdns's settings while it runs".
- 2026-10-10: Done. `make check` passed; `make test-integration` passed
  (internal/cli 85.1 s). ITEM-0093's integration test changes
  `drift.interval` on a serve that reads the lab.
