---
id: ITEM-0022
title: NetBox lab and integration tests in CI
type: task # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-036, REQ-039]
depends_on: []
created: 2026-09-27
closed:
---

# ITEM-0022: NetBox lab and integration tests in CI

## Goal

A container lab with NetBox 4.7 and 4.6 and the DNS plugin. Integration
tests run against it locally and in every pipeline on both forges.

## Acceptance criteria

- [ ] `deploy/dev/compose.yaml` runs PostgreSQL, Valkey, NetBox 4.7 with plugin 1.7.x (port 8047), and NetBox 4.6 with plugin 1.6.x (port 8046). Every image is pinned by digest, and the plugin by version.
- [ ] docker-compose, and buildx if compose needs it, are pinned in `tools/tools.mk`. `tools/fetch.sh` can fetch an asset that is itself the binary.
- [ ] `make lab-up` and `make lab-down` work. `make test-integration` depends on `lab-up`, and takes the lab host from `DOCKER_HOST`.
- [ ] An `integration-test` job with a digest-pinned `docker:dind` service runs in every GitLab and GitHub pipeline. `make ci` includes `test-integration`, and `make project-lint` passes.
- [ ] The job's peak ephemeral storage is measured and reported to the user.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-27: Created from M01's approved design, before implementation
  started.
- 2026-09-29: The user hasn't yet confirmed runner capacity for the
  Docker-in-Docker job (estimated 4–6 GB of ephemeral storage). Measure the
  real figure and report it. Also agreed in the review: golangci-lint and
  `go vet` must check files with the `integration` build tag, which they skip
  by default.
