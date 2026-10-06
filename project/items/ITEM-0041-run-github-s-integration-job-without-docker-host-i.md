---
id: ITEM-0041
title: Run GitHub's integration job without DOCKER_HOST in its environment
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-026, REQ-036, REQ-039]
depends_on: [ITEM-0030]
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0041: Run GitHub's integration job without DOCKER_HOST in its environment

## Goal

With the mirror restored (ITEM-0030), GitHub ran the workflow for the
first time since M00, on `main` (`7934f8b`) and on `m02-powerdns-read-path`
(`c5b4bea`). Every job passed but `integration-test`, which has never run
on GitHub before: its `actions/checkout` step and that step's post step
failed, before any of nbpdns's own commands ran. The job is a container job,
and GitHub's runner runs each of its steps with `docker exec`, handing the
job's environment to that docker command too. The job set
`DOCKER_HOST=tcp://docker:2375`, the Docker-in-Docker service's address,
which only the job's own network can reach, so the runner's `docker exec`
had nothing to talk to. Run the job without `DOCKER_HOST` in its
environment.

## Acceptance criteria

- [x] The Makefile takes the lab's Docker host from `LAB_DOCKER_HOST` when it's set, exporting it as `DOCKER_HOST` for compose and the tests; `DOCKER_HOST` alone works as before, so GitLab's job is unchanged.
- [x] GitHub's `integration-test` job sets `LAB_DOCKER_HOST`, not `DOCKER_HOST`, and `make project-lint` passes.
- [x] The emulated CI job passes with only `LAB_DOCKER_HOST` set.
- [x] GitHub's workflow passes on the M02 branch, `integration-test` included.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Diagnosed from GitHub's public API: the jobs list shows
  `integration-test` failing at "Run actions/checkout" and "Post Run
  actions/checkout", and every other job passing. The job log needs a
  signed-in user, so it wasn't read; the diagnosis is the runner's known
  behavior for container jobs, which matches both failed steps, and the only
  difference between this job and the ones that pass.
- 2026-10-06: Fixed, but not yet seen on GitHub. The Makefile exports
  `LAB_DOCKER_HOST` as `DOCKER_HOST` when it's set, and GitHub's job sets
  `LAB_DOCKER_HOST` instead of `DOCKER_HOST`; GitLab's job, which works,
  still sets `DOCKER_HOST`. `make -n` shows both forms give the lab the
  same host and bind address, and `make project-lint` accepts the workflow.
  The emulated CI job, with only `LAB_DOCKER_HOST` set, passed in 313 s,
  every package. The last criterion waits for GitHub's run after the next
  push.
- 2026-10-06: Done. After the user pushed `44623bc`, GitHub Actions run 37426268234 on
  `m02-powerdns-read-path` passed every job, from GitHub's public API:
  project-lint, docs-lint, api-lint, go-lint, unit-test, integration-test
  (7m25s), build, docs-site, secrets and vuln. It's the first time the
  integration job has passed on GitHub.
