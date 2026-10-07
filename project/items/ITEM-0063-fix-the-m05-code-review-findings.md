---
id: ITEM-0063
title: Fix the M05 code review findings
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M05
requirements: [REQ-045]
depends_on: [ITEM-0061]
created: 2026-10-07
closed: 2026-10-07
---

# ITEM-0063: Fix the M05 code review findings

## Goal

`/code-review high` on `origin/main...m05-packaging`, at `876963f`, found
eight things, and `/security-review` found one wrong claim in the docs.
This item fixes them, or records why not.

## Acceptance criteria

- [x] A snapshot passes the release tests before the first tag, at a tag, and after it.
- [x] The compiled `projctl` binary is gone from the branch's history, and ignored.
- [x] `projctl` refuses a release job with `needs:`, or with a stage that isn't after every other job's.
- [x] `make release` takes the one `v` tag at HEAD, refuses more than one, and hands it to GoReleaser.
- [x] `make release` refuses a `RELEASE_REGISTRY` that isn't a host or host:port.
- [x] The docs say who can publish, and which GitLab settings limit it.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Done. The findings, and what changed:
  1. **The snapshot test would fail at and after the first tag:**
     `checkVersion` wanted a snapshot's version to start with `v0.0.0-` or
     the tag and a `-`. At `v0.1.0`, Go stamps `v0.1.0`; after it,
     `v0.1.1-0.<time>-<hash>`. So `release-check` would have failed in
     `v0.1.0`'s own pipeline, before the release stage, and in every
     pipeline after it. A snapshot now passes if it reports the tag, or a
     pseudo-version that ends with the commit's time and hash, with or
     without `+dirty`.
  2. **A 4 MB `tools/projctl/projctl` binary was committed** in "feat:
     publish releases to GitLab on version tags". None of M05's commits
     had been pushed, so that commit was rebuilt without it, by
     cherry-picking, and the commits after it replayed on top: no
     force-push is needed. `.gitignore` now ignores the binaries that
     `go build` leaves in `tools/projctl` and `tools/hooktest`.
  3. **`projctl` didn't stop the release job running early:** `needs: []`,
     an earlier stage, or a check job in `.post` would let it publish
     before every check passed. `gitlabRelease` now refuses `needs:` in
     the job or what it extends, and any other job whose stage isn't
     before the release job's, in GitLab's order: `.pre`, `stages:`,
     `.post`. Four new tests show each refusal. The tests' base CI file
     gains a `release` stage.
  4. **`latest` and `<major>.<minor>` can move back** if an older version
     is released after a newer one. Not fixed here: `make release` can't
     tell which release is newest from a shallow CI clone. ITEM-0062
     (M17) records it. "Make a release" now says to release from `main`,
     in version order, and "Release artifacts" says what the moving tags
     follow.
  5. **`make release` and GoReleaser could pick different tags:** it took
     `git describe --exact-match --tags HEAD`, and GoReleaser picked its
     own. It now takes `git tag --points-at HEAD --list 'v*'`, which must
     be exactly one tag, `vMAJOR.MINOR.PATCH`, and passes it to GoReleaser
     as `GORELEASER_CURRENT_TAG`. With one `v` tag, Go stamps that one
     too.
  6. **`config.json` wasn't escaped:** `RELEASE_REGISTRY` must now be a
     host, or host:port, which can't break the JSON.
  7. **The tests' `releaseIf` copies `releaseRule`:** kept, on purpose,
     so that loosening the rule fails the tests until they're changed too.
     Its comment now says so.
  8. **`checkStatic` copied each binary into a string:** it reads it with
     `bytes.NewReader`.

  `/security-review` found nothing at its bar. It noted that the
  explanation said only the release job has the publishing credentials,
  but GitLab gives every job the registry's credentials and a job token.
  The explanation now says so. "Make a release" adds two settings, to
  protect the image's release tags and refuse duplicate generic packages.

  **Checked, in a scratch clone, deleted afterwards:**
  - `make release-check` passed with no tag (`v0.0.0-20261007131253-…`),
    at a `v0.1.0` tag (`v0.1.0`), and a commit after it
    (`v0.1.1-0.20261007131355-…`).
  - At `v0.1.0`, `make release` published to a temporary `registry:3`:
    the tags `0.1.0`, `0.1` and `latest`, and three SBOMs. The image
    reported `v0.1.0`, unmodified. Without `GITLAB_TOKEN`, GoReleaser
    skipped the GitLab release.
  - `make release` refused HEAD with no `v` tag, HEAD with `v0.1.0` and
    `v0.1.0-rc.1`, and `RELEASE_REGISTRY='bad"host'`, each with its
    message.
