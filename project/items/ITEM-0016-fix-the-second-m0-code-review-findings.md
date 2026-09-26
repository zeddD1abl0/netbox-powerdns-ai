---
id: ITEM-0016
title: Fix the second M0 code-review findings
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M00
requirements: [REQ-022, REQ-026]
depends_on: []
created: 2026-09-26
closed: 2026-09-26
---

# ITEM-0016: Fix the second M0 code-review findings

## Goal

The first `/code-review high` (ITEM-0013) came before ITEM-0014 and ITEM-0015
added the pinned-binary toolchain and the staged CI. Review that code, and fix
what's found before M00 closes, so M01 starts on a clean base.

## Acceptance criteria

- [x] `/code-review high` has run on the code added by ITEM-0014 and ITEM-0015. Each finding is fixed, or recorded here with the reason it isn't.
- [x] The goimports edit hook groups imports the way `.golangci.yml` requires, so a file it formats passes `make lint`. Pipe-tested.
- [x] `make tools-audit` never writes `tools/tools.mk`. It fails with a diff when a pin differs from the published checksums. Tested in a scratch clone.
- [x] A cached tool binary is fetched and verified again whenever `tools/tools.mk` changes.
- [x] The main-branch guard also catches path-prefixed `git`, options with separate or quoted arguments, and merge, cherry-pick, revert, am, pull and rebase. Pipe-tested.
- [x] `projctl lint` checks each CI job's own image, and rejects keys and variables that let a job pass or be skipped while its make target fails. Covered by tests.
- [x] ADR-0011's stale toolchain text is corrected by a superseding ADR.
- [x] `make ci` is green.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-26: `/code-review high` ran on `tools/fetch.sh`, `tools/update.sh`,
  `tools/tools.mk`, the `Makefile`, the CI rules in `tools/projctl`, both CI
  files, `.claude/settings.json` and the commit guard. It reported 10
  findings. Four matched the ones found before the review: the hook,
  `tools-audit`, the cache and the GitHub triggers. Each was checked against
  the code:
  1. **Commit guard bypasses (fixed).** Reproduced on a scratch repo on `main`:
     `/usr/bin/git commit`, `git --git-dir .git commit`,
     `git -c 'user.name=A B' commit`, `git merge` and `git cherry-pick` were
     all allowed. The pattern now accepts a path before `git` and options with
     a separate, possibly quoted argument. It also covers every subcommand
     that creates commits: commit, merge, cherry-pick, revert, am, pull and
     rebase. Pipe-tested in 32 cases, including a run without `jq`: 18
     denials and 14 allows, among them `git log --grep merge` and
     `git mergetool`. The guard still reads the command's text, so it's a
     safety net against mistakes, not a sandbox. Its header says so.
  2. **`tools-audit` wrote `tools.mk` (fixed).** `tools/update.sh --check`
     replaces `--current`. It edits a temporary copy and compares it with
     `git diff --no-index`. Tested in a scratch clone:
     - a clean clone passes;
     - an uncommitted but correct edit passes;
     - a tampered Hugo hash fails, prints the diff, and leaves `tools.mk`
       byte-for-byte unchanged.
  3. **Cached binaries weren't tied to their pins (fixed).** Each binary now
     depends on `tools/tools.mk`. Testing found two bugs in the first attempt:
     - `tar` keeps the archive's mtime, so every binary looked older than
       `tools.mk` and every run refetched. `fetch.sh` now touches the binary.
     - A failed fetch left the previous binary behind. `fetch.sh` now
       removes it first.

     Tested in a scratch clone: a cold fetch gets 6 and the next run 0, a
     touched `tools.mk` gets 6 and the next run 0, and a tampered pin fails
     with no binary left.
  4. **The format hook disagreed with lint (fixed).** The hook runs goimports
     with `-local github.com/zeddD1abl0/netbox-powerdns-ai`. Pipe-tested: a
     file that mixes third-party and internal imports comes out with the
     internal ones in their own group, and `golangci-lint fmt --diff` then
     reports nothing.
  5. **`update.sh` could leave `tools.mk` half-updated (fixed).** It now
     writes the whole file only after every tool succeeds. In a scratch clone,
     latest mode with a broken Vale asset name failed at Vale and left
     `tools.mk` unchanged. Two earlier tools had already been derived by then.
  6. **`update.sh` didn't validate versions or hashes (fixed).** A version
     must match `[0-9A-Za-z.+-]+`. A hash must be exactly 64 lowercase hex
     digits on a single line. Checked under `dash`: `1/e`, `1&x`, an empty
     value, two hashes, 63 or 65 digits and uppercase are all rejected.
  7. **`make shell` pulled the host's architecture (fixed).** The CI image's
     digest is a multi-arch index (checked with
     `docker buildx imagetools inspect`), and only linux-amd64 is pinned.
     `make shell` now passes `--platform linux/amd64`. Checked on this amd64
     host, where `go env` inside gives `linux-amd64`. An arm64 host wasn't
     available to test.
  8. **A CI job with no image passed lint (fixed).** `projctl` now resolves
     each job's own image:
     - on GitLab, through `extends` (a later entry wins), YAML merge keys,
       `default` and the top-level `image`;
     - on GitHub, from `container`.

     A job that names no image, or another image, is reported.
  9. **Keys that neutralise a job passed lint (fixed).** `projctl` rejects:
     - on GitLab jobs and their templates: `allow_failure`, `when`, `rules`,
       `only` and `except`;
     - on GitHub jobs and steps: `if` and `continue-on-error`;
     - `MAKEFLAGS`, `MFLAGS`, `GNUMAKEFLAGS` and `MAKEFILES` in any
       `variables` or `env`.

     `workflow: rules` stays allowed. Tests: 10 new catch cases and 7 cases
     that must pass. Run against the old code, 9 of the catch cases failed
     outright. The other two differ only in the message, which now names the
     job.
  10. **The GitHub workflow runs twice for a same-repo PR (not changed).**
      GitHub is a push mirror with no PR workflow, so today this costs only
      runner time. Revisit if the project starts taking PRs on GitHub. The
      same finding noted that `lint.go`'s comment still said CI keeps its
      caches in `.cache/`. That comment was corrected.
- 2026-09-26: ADR-0017 restates ADR-0011's Hugo decision with the toolchain
  from ADR-0014: a release binary pinned in `tools/tools.mk`, not
  `tools/go.mod`. The decision is unchanged. ADR-0011's status is now
  `superseded by ADR-0017`, which `make project-lint` accepts as a pair. The
  comments in `site/hugo.toml` and `render-heading.html`, and M00's
  superseded-rules callout, now point to ADR-0017. Historical references
  (Q-044, Q-046, the CHANGELOG, ITEM-0008, M00's ADR criterion) still name
  ADR-0011, the ADR those answers produced at the time.
- 2026-09-26: Done. `make ci` is green. CHANGELOG unchanged: its lines on the
  toolchain, CI and the commit hook are still accurate.
