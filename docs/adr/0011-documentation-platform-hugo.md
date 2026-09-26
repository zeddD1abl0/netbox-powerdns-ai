---
title: "0011: Documentation platform — Hugo"
status: superseded by ADR-0017
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-018, REQ-020, REQ-021, REQ-026]
questions: [Q-044, Q-046]
---

# 0011: Documentation platform — Hugo

## Context and problem statement

The docs must read cleanly as raw Markdown on any forge, and also use the
features of a real documentation site (REQ-021). The toolchain must be
self-contained and pinned in the repo (ADR-0003). Material for MkDocs, the
obvious default, is in maintenance mode, and its successor Zensical needs
Python.

## Decision drivers

- Only Go, Docker and make as prerequisites (ADR-0003).
- Raw Markdown stays readable on GitHub and GitLab.
- Builds work offline and in air-gapped environments.

## Considered options

1. Hugo: a Go binary, pinned as a Go tool.
2. Zensical, run from a pinned Python container image.
3. Docusaurus: needs Node.

## Decision outcome

Chosen option: **Hugo** (Q-044).

- **Pinned** in `tools/go.mod` and run through `make docs`. No separate
  install.
- **Site project in `site/`**, which mounts `../docs` as its content. The docs
  stay where they are, and `site/` holds only configuration, layouts and the
  theme.
- **Theme:**
  - must not need a Node build;
  - is vendored with `hugo mod vendor` into `site/_vendor/`, so builds need no
    network;
  - is chosen by a spike between Hextra and hugo-book (ITEM-0008). The theme is
    easy to change, so the pick is recorded in ITEM-0008, not in a new ADR.
- **Markdown features** that also work raw on GitHub and GitLab:
  - GitHub alerts (`> [!NOTE]`), rendered through a blockquote render hook;
  - Mermaid code blocks, rendered through a code-block render hook.
- **Titles:** pages keep their own `# H1` so the raw file reads well. The
  layout suppresses the theme's own title heading, so it isn't shown twice.
- **Output:** the built site is a release artifact that can be hosted anywhere
  (Q-046). The API reference is also served by the binary (ADR-0012).

### Consequences

- Good: one more Go tool, nothing new to install. Offline builds.
- Bad: Hugo's module is large, so the first `make docs` takes a while to
  compile. It's cached after that.
- Bad: shortcodes don't render raw. The style guide allows none until one is
  justified (see `docs/contributing/documentation-style.md`).

### Confirmation

- `make docs` builds with no network access once the tools are cached.
- htmltest checks the built site's links in `make ci`.
- A test page with an alert and a Mermaid diagram renders on the site and raw
  on GitLab (ITEM-0008).
