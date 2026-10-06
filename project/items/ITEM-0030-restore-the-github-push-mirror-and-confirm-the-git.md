---
id: ITEM-0030
title: Restore the GitHub push mirror and confirm the GitHub pipeline
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-026, REQ-039]
depends_on: []
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0030: Restore the GitHub push mirror and confirm the GitHub pipeline

## Goal

GitHub is a push mirror of every branch, configured in GitLab (ADR-0018), and
its workflow is how the project shows it isn't tied to one forge (ADR-0003,
REQ-026). The mirror stopped after M00: on 2026-10-06, GitHub's `main` was
still at `fcbdaeb`, the M01 branch was never mirrored, and the last Actions
run was on 2026-09-26. So the GitHub workflow, including its
`integration-test` job, has never run M01's code, and M01's criterion
"Integration tests run in every GitLab and GitHub pipeline" is open. Restore
the mirror, then confirm the GitHub workflow passes on the M01 merge.

## Acceptance criteria

- [x] The GitLab push mirror to GitHub works again: GitHub's `main` is at GitLab's `main` (`7934f8b` or later).
- [x] The GitHub workflow passes on `main`, including `integration-test`, or each failure has an item.
- [x] M01's open GitHub criterion is ticked, with the run, in its milestone file.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Found when recording M01's merge. `git ls-remote` on the
  GitHub repository listed only `refs/heads/main` at `fcbdaeb`. GitHub's
  public API listed four Actions runs, the last on 2026-09-26 for `main` at
  `fcbdaeb`, all successful. Blocked on the user: the mirror's settings, and
  any error it reports, are in GitLab under Settings, then Repository, then
  Mirroring repositories. A common cause is an expired GitHub token.
- 2026-10-06: The user restored the mirror. GitHub's `main` is at `7934f8b`
  and `m02-powerdns-read-path` at `c5b4bea`, as on GitLab. GitHub then ran
  the workflow on both: every job passed but `integration-test`, which
  failed at its checkout step, because the job's `DOCKER_HOST` redirected
  the runner's own docker commands. ITEM-0041 fixes that; this item's last
  criteria wait for GitHub's run of the fix.
- 2026-10-06: Done. On `main` (`7934f8b`), GitHub's run failed only at
  `integration-test`, for the reason ITEM-0041 fixed. With the fix,
  GitHub Actions run 37426268234 on `m02-powerdns-read-path` (`44623bc`), which holds all of `main`,
  passed every job, `integration-test` included; `main` runs it once M02 is
  merged. M01's GitHub criterion is ticked with that run.
- 2026-10-06: After M02's merge, the mirror carried `main` (`d96f5ca`) to
  GitHub, and GitHub Actions run 37438635140 on `main` passed every job,
  `integration-test` included.
