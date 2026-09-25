---
id: ITEM-0006
title: Makefile and thin CI wrappers for GitLab and GitHub
type: task
status: blocked
milestone: M00
requirements: [REQ-026]
depends_on: [ITEM-0004]
created: 2026-09-25
closed:
---

# ITEM-0006: Makefile and thin CI wrappers for GitLab and GitHub

## Goal

Make `make` the single entry point, so CI on any forge, and any developer, runs
exactly the same steps (ADR-0003).

## Acceptance criteria

- [ ] Targets: `check`, `ci`, `fmt`, `lint`, `test`, `test-integration`, `vuln`, `docs`, `project`, `build`, `image`, `release-dry`, `help`.
- [ ] `make help` lists every target with a one-line description.
- [ ] `.gitlab-ci.yml` calls only make targets. It runs on the existing self-hosted runners (Q-049).
- [ ] `.github/workflows/ci.yml` calls the same targets.
- [ ] A check fails if either CI file runs anything other than make targets.
- [ ] `CLAUDE.md` command table updated to match.

## Notes
