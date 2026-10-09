---
id: ITEM-0062
title: Keep the image's latest tag on the newest release
type: debt # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M18
requirements: [REQ-045]
depends_on: []
created: 2026-10-07
closed:
---

# ITEM-0062: Keep the image's latest tag on the newest release

## Goal

Every release pushes the image's `latest` and `<major>.<minor>` tags,
whatever its version. Releases are made from `main`, in order, so each
release is the newest, and the tags are right. But a release of an older
version, such as 0.1.2 after 0.2.0, would move `latest` back to 0.1.2.
This item makes `make release` push the moving tags only for the newest
release, or refuse an older one, before anyone needs to release one.

## Acceptance criteria

- [ ] A release of a version older than the newest release doesn't move `latest`, or doesn't publish, and a test or dry run shows it.
- [ ] "Make a release" and "Release artifacts" say what happens.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M05's `/code-review high` (finding 4).
  - **Why not in M05:** `make release` can only tell which release is the
    newest from every version tag. A GitLab tag pipeline's clone is
    shallow, and fetches only the tags in its history, so a newer tag on
    another line can be missing. The fix needs a `git fetch --tags` in the
    release job, or the registry's or GitLab's list of releases.
  - **Until then:** "Make a release" says to release from `main`, in
    version order.
