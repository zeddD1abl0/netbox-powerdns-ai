---
id: ITEM-0120
title: Keep the lock holder's PID when the host name is long
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033]
depends_on: []
created: 2026-11-04
closed: 2026-11-05
---

# ITEM-0120: Keep the lock holder's PID when the host name is long

## Goal

PostgreSQL truncates `application_name` to 63 bytes, and a replica's
session, and each lock's, is named `nbpdns: host <name>, PID <pid>`
(`holderName`, `internal/store/lock.go`). Where the host name is long, as
a Kubernetes pod's or a GitLab runner's is, the PID is cut off, and the
error for a held lock names `host <name>,` with no PID. Pipeline 845's
integration job, on the runner's 50-character host name, failed
`TestServeOnPostgres` and `TestStoreLock/postgres` on it. The name should
keep the PID however long the host name is: shorten the host name, not
the PID.

## Acceptance criteria

- [x] A lock holder's name fits in 63 bytes and keeps its PID, with a host name of any length; a host name that's cut short says so.
- [x] A test names a holder with a host name of 64 characters or more, on PostgreSQL.
- [ ] `make test-integration` passes, and the branch's next pipeline passes its integration job.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-04: Found in pipeline 845 (f8e728a), while ITEM-0113 checked it
  for ITEM-0110. Its job got its 2Gi request and ran to the end, without
  an OOM kill or a retry; only these two tests failed.
- 2026-11-05: Pipeline 950 (9573c69) failed its integration job the same
  way, now in five tests, as ITEM-0112's and ITEM-0113's tests read the
  same name: TestServeOnPostgres, TestListenerOutlastsIdleTimeout,
  TestSessionSettings, TestReplicasShareTheServeLock and
  TestStoreLock/postgres. Each shows `host runner-…-6pxrh5d4,` with the PID
  cut off. Every pipeline fails on it until it's fixed.
- 2026-11-05: Done. `applicationName(host, pid)` (`internal/store/lock.go`)
  names a session `nbpdns: host <name>, PID <pid>` in at most 63 bytes: a
  host name too long to fit beside the PID keeps both ends, cut in its
  middle at `...` (a pod's or runner's name differs at its end, a domain
  name at its start), and any character but printable ASCII is `?`, as
  PostgreSQL would otherwise escape it and lengthen the name. The serve
  lock, the replica's session and the listener all use `holderName`; the
  SQLite lock file keeps the whole host name, having no limit. A
  `hostname` variable lets a test set the host's name. Tests:
  `TestApplicationName` (the runner's 50 characters, a 254-character name
  with a 7-digit PID, exactly fitting, one over, non-ASCII), and
  `TestLongHostName` on both databases, a 73-character host name, which
  fails with the old format on PostgreSQL (checked). The five tests that
  failed in pipeline 950 now take their expected name from `holderName`,
  and the CLI's checks for the PID and the host's start. The PostgreSQL
  how-to says that a long host name is cut short. No CHANGELOG line: the
  bug was in PostgreSQL support, which isn't released yet. `make check`
  passes; `make test-integration` passes in 2 min 32 s, its largest
  process at 1.41 GiB. The last criterion's pipeline half waits for the
  branch's next push, which ITEM-0100's notes record.
- 2026-11-06: Pipeline 953 (26f2217) didn't reach the integration job: its
  `go-lint` job failed when golangci-lint ran past its 5-minute timeout
  (ITEM-0122), and the test stage was skipped. The last criterion's
  pipeline half still waits, now for the first pipeline after ITEM-0122.
- 2026-11-06: Pipeline 964 (dc80fc6) didn't reach the integration job
  either: its `go-lint` job failed on golangci-lint's timeout again
  (ITEM-0122). The last criterion's pipeline half still waits for the
  first pipeline after ITEM-0122.
