---
title: "0016: Staged CI pipelines that mirror `make ci`"
status: accepted
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-026, REQ-039]
questions: [Q-049]
supersedes:
---

# 0016: Staged CI pipelines that mirror `make ci`

## Context and problem statement

M0's first CI ran one job, `make ci`. The user asked for stages, 2026-09-25:
> The whole concept of the CI pipeline is that it should consist of multiple
> stages, through linting, building, testing, etc. Having a single command is
> a bit odd. I don't mind if the Makefile itself has a default "Just do
> everything". But the CI pipelines should definitely have stages, especially
> when we eventually build in security scanning, point testing, SBOM
> regression, etc.

Splitting CI into jobs risks drift: a check that runs locally but in no CI
job, a CI job that runs a different image, or two forges that disagree.

## Decision drivers

- Stages that show where a pipeline failed, with room for security scanning,
  SBOMs and more.
- CI logic stays in the Makefile (ADR-0014). Forge files only run make
  targets.
- CI and the local `make ci` must never drift apart.

## Considered options

1. One job running `make ci` (the first design).
2. Staged jobs, each running make targets, kept in step with `make ci` by a
   lint rule.
3. Stage logic in the forge files: scripts, templates, `include:`.

## Decision outcome

Chosen option: **staged jobs that run make targets, kept in step with
`make ci` by `projctl lint`.**

- **Stages:** `lint`, then `test`, then `build`, then `security`. GitLab uses
  `stages:`; GitHub mirrors them with `needs:`.

  | Stage | Jobs |
  |---|---|
  | lint | `go-lint` (`make vet lint`), `docs-lint`, `api-lint`, `project-lint` |
  | test | `unit-test` (`make test`) |
  | build | `docs-site` (`make docs-links`) |
  | security | `vuln`, `secrets` |

  Later stages and jobs (integration tests, SAST, container scanning, SBOM
  regression) slot into this structure.
- **`make ci`** stays as the local "run everything" target, and `make check`
  as its subset without the docs build.
- **`make project-lint` enforces the mirror**, for each CI file:
  - The make targets its jobs run, plus their prerequisites, must equal those
    of `make ci`. Only targets with a recipe count, so running the parts of an
    aggregate separately counts the same as running the aggregate.
  - Every job's image must equal the Makefile's `CI_IMAGE`.
  - The make-only rule from ADR-0014 still applies: no flags, variables,
    other actions, `include:` or shell overrides.
- **No CI cache.** Jobs fetch pinned binaries in seconds (ADR-0014). Caching
  would depend on a shared runner cache and would cost pod disk without one.

### Consequences

- Good: failures are reported by stage. New checks slot in, and forgetting a
  forge or `make ci` fails lint.
- Bad: each job starts cold, so tools and modules are fetched once per job.
  That's cheap now: about 76 MB of binaries for the whole pipeline.
- Bad: the GitHub file repeats the image and checkout per job. The image rule
  catches any drift.

### Confirmation

- Tested in `tools/projctl`: missing and extra targets, a mismatched image,
  and no image at all.
- Removing the `vuln` job from `.gitlab-ci.yml` makes `make project-lint` fail
  (verified 2026-09-25).
