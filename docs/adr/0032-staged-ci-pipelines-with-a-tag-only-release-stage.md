---
title: "0032: Staged CI pipelines with a tag-only release stage"
status: accepted
date: 2026-10-07
decision-makers: [jordan]
requirements: [REQ-026, REQ-039, REQ-045]
questions: [Q-049]
supersedes: ADR-0016
---

# 0032: Staged CI pipelines with a tag-only release stage

## Context and problem statement

ADR-0016 split CI into stages of jobs that only run make targets, and had
`projctl` keep the jobs in step with `make ci`. The user asked for stages on
2026-09-25:
> The whole concept of the CI pipeline is that it should consist of multiple
> stages, through linting, building, testing, etc. Having a single command is
> a bit odd. I don't mind if the Makefile itself has a default "Just do
> everything". But the CI pipelines should definitely have stages, especially
> when we eventually build in security scanning, point testing, SBOM
> regression, etc.

Releases (ADR-0030) add a step that must run in some pipelines and not
others: publishing, only for a version tag. ADR-0016's rule, that every job
runs exactly `make ci`'s targets and nothing can skip a job, has no room for
it. This ADR restates ADR-0016, and adds the one exception.

## Decision drivers

- Stages that show where a pipeline failed, with room for security
  scanning, SBOMs and more.
- CI logic stays in the Makefile (ADR-0022). Forge files only run make
  targets.
- CI and the local `make ci` must never drift apart.
- Publishing happens only for a deliberate version tag, and can't be
  slipped into another job.

## Considered options

1. A tag-only `release` job, which `projctl` allows as the one exception.
2. A `make release` that every pipeline runs, and that publishes only when
   a tag variable is set.
3. Publishing by hand, outside CI.

## Decision outcome

Chosen option: **staged jobs that run make targets, kept in step with
`make ci` by `projctl lint`, plus a tag-only release stage.**

- **Stages:** `lint`, then `test`, then `build`, then `security`, then, on
  GitLab, `release`. GitLab uses `stages:`; GitHub mirrors the first four
  with `needs:`.

  | Stage | Jobs |
  |---|---|
  | lint | `go-lint` (`make vet lint generate-check`), `docs-lint`, `api-lint`, `project-lint` |
  | test | `unit-test` (`make test`), `integration-test` (`make test-integration`) |
  | build | `build` (`make build`), `docs-site` (`make docs-links`), `release-check` (`make release-check`) |
  | security | `vuln`, `secrets` |
  | release | `release` (`make release`), on GitLab, for version tags only |

  Later jobs, such as container scanning, slot into this structure.
- **`make ci`** stays as the local "run everything" target, and `make check`
  as its subset without the docs build, the release check and the
  integration tests.
- **`make project-lint` enforces the mirror**, for each CI file:
  - The make targets its jobs run, plus their prerequisites, must equal
    those of `make ci`. Only targets with a recipe count, so running the
    parts of an aggregate separately counts the same as running the
    aggregate.
  - Every job's image must equal the Makefile's `CI_IMAGE`.
  - No job may use a key that can let it pass, or not run, while its target
    fails: `allow_failure`, `when`, `rules`, `only` or `except` on GitLab,
    and `if` or `continue-on-error` on GitHub.
  - The make-only rule still applies: no flags, variables, other actions,
    `include:` or shell overrides.
- **The one exception: the `release` job.** On GitLab only, a job named
  `release` may run `make release`, which isn't in `make ci`. Its only skip
  key may be one `rules:` entry, an `if` that matches a version tag. Any
  other job running `make release`, a `release` job anywhere else, or a
  `release` job with other rules fails lint. GitLab's `workflow:` runs
  pipelines for tags, so a tag's pipeline runs every check, then publishes.
- **No CI cache.** Jobs fetch pinned binaries in seconds (ADR-0022).
  Caching would depend on a shared runner cache, and cost pod disk without
  one.

### Consequences

- Good: failures are reported by stage. New checks slot in, and forgetting
  a forge or `make ci` fails lint.
- Good: publishing is one job, visibly tag-only, after every check has
  passed in the same pipeline.
- Bad: each job starts cold, so tools and modules are fetched once per job.
- Bad: the GitHub file repeats the image and checkout per job. The image
  rule catches any drift.
- Bad: the exception is GitLab's alone. Publishing from another forge would
  need its own rule in `projctl`.

### Confirmation

- Tested in `tools/projctl`: missing and extra targets, a mismatched image,
  no image, a skip key, and the release job: allowed only as named, on
  GitLab, with its one tag rule.
- Removing the `vuln` job from `.gitlab-ci.yml` makes `make project-lint`
  fail, as it did under ADR-0016.

## Pros and cons of the options

### A make release that publishes only with a tag variable

- Good: no exception in `projctl`; every pipeline runs the same jobs.
- Bad: whether a pipeline publishes depends on a variable, not on anything
  the CI file shows.

### Publishing by hand

- Good: no credentials in CI.
- Bad: what's published wasn't necessarily built by the pipeline that
  checked it.

## More information

- Supersedes ADR-0016, whose decision this restates.
- ADR-0022 (the make-only rule), ADR-0030 (the release).
- Recorded in M05's design, 2026-10-07.
