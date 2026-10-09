---
id: ITEM-0064
title: Publish the CHANGELOG's section as the release notes
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-045]
depends_on: []
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0064: Publish the CHANGELOG's section as the release notes

## Goal

The v0.1.0 release on GitLab was published with empty notes. `make release`
passes the CHANGELOG's section to GoReleaser with `--release-notes`, but
`.goreleaser.yaml` set `changelog.disable: true`, which, as GoReleaser's
docs warn, "will also ignore any changelog files passed via
`--release-notes`, and will render an empty changelog". The dry runs didn't
show it, because without `GITLAB_TOKEN` no release is made.

## Acceptance criteria

- [x] `.goreleaser.yaml` doesn't disable the changelog, and a tag build loads the notes file.
- [x] "Make a release" checks the notes, and says how to fix empty ones.
- [x] The CHANGELOG records the fix.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Done.
  - **Found** while checking v0.1.0 with the read-only project token: the
    release's description was `"\n"`, and job 4235's log has no sign of
    the notes being read.
  - **Fixed:** the `changelog:` block is gone. A comment says never to set
    `changelog.disable`, and why. Snapshots skip GoReleaser's changelog
    step anyway, so `release-check` is unchanged, and can't test this.
  - **Checked**, in a scratch clone (deleted after) at a throwaway `v0.1.1`
    tag, with `--skip=publish,ko,announce` and `--verbose`. Without
    `disable`, GoReleaser logged `loading file …/notes-test.md` and
    `read 40 bytes`. With it, it never loaded the file.
  - **"Make a release"** now checks the release's notes, and says to fix
    wrong or empty ones by editing the release in GitLab.
  - **v0.1.0's notes** still need that: the read-only token can't edit a
    release, so the user does.
- 2026-10-08: The user chose not to fill in v0.1.0's notes by hand, so its GitLab release keeps empty notes. Later releases get theirs from the CHANGELOG.
