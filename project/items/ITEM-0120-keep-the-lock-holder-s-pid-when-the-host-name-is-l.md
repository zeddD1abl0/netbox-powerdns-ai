---
id: ITEM-0120
title: Keep the lock holder's PID when the host name is long
type: bug # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033]
depends_on: []
created: 2026-11-04
closed:
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

- [ ] A lock holder's name fits in 63 bytes and keeps its PID, with a host name of any length; a host name that's cut short says so.
- [ ] A test names a holder with a host name of 64 characters or more, on PostgreSQL.
- [ ] `make test-integration` passes, and the branch's next pipeline passes its integration job.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-04: Found in pipeline 845 (f8e728a), while ITEM-0113 checked it
  for ITEM-0110. Its job got its 2Gi request and ran to the end, without
  an OOM kill or a retry; only these two tests failed.
