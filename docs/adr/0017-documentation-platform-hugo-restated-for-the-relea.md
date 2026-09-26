---
title: "0017: Documentation platform — Hugo, restated for the release-binary toolchain"
status: accepted
date: 2026-09-26
decision-makers: [jordan]
requirements: [REQ-018, REQ-020, REQ-021, REQ-026]
questions: [Q-044, Q-046]
supersedes: ADR-0011
---

# 0017: Documentation platform — Hugo, restated for the release-binary toolchain

## Context and problem statement

ADR-0011 chose Hugo for the documentation site. Parts of its text describe the
toolchain as it was then (ADR-0003, later ADR-0013):
- Hugo "pinned in `tools/go.mod`" and compiled from source on the first
  `make docs`;
- Go, Docker and make as the only prerequisites.

ADR-0014 replaced that toolchain. Hugo is now a release binary pinned by
SHA-256 in `tools/tools.mk`, and the prerequisites include curl, tar and
sha256sum. ADR-0014 superseded only ADR-0013, so ADR-0011 was never updated.
Accepted ADRs aren't edited, so this ADR restates the decision with the
toolchain corrected. **The decision itself is unchanged.**

## Decision drivers

- The docs must read cleanly as raw Markdown on GitHub and GitLab, and also use
  a real site's features (REQ-021).
- The toolchain is self-contained and pinned in the repository (ADR-0014).
- Builds work offline and in air-gapped environments.

## Considered options

As in ADR-0011:
1. Hugo, a single binary.
2. Zensical, run from a pinned Python container image.
3. Docusaurus, which needs Node.

## Decision outcome

Chosen option: **Hugo** (Q-044), as in ADR-0011.

- **Pinned** as a release binary by SHA-256 in `tools/tools.mk` (ADR-0014),
  fetched once into `.cache/tools/`. `make docs` builds the site, and
  `make docs-links` checks its links. Nothing is installed or compiled.
- **Site project in `site/`**, which mounts `../docs` as its content. The docs
  stay where they are, and `site/` holds only configuration, layouts and the
  theme.
- **Theme: Hextra**, chosen by the ITEM-0008 spike over hugo-book.
  - It needs no Node build.
  - It's vendored with `hugo mod vendor` into `site/_vendor/`, so builds need
    no network. `make docs-theme-update` refreshes it.
  - Its MIT notice is kept in `site/Hextra.LICENSE`.
- **Markdown features** that also work raw on GitHub and GitLab:
  - GitHub alerts (`> [!NOTE]`) and Mermaid code blocks, both rendered by
    Hextra's own hooks;
  - relative links written for the repository, resolved to site pages or to
    source files by the link hook in `site/layouts/_markup/`.
- **Titles:** pages keep their own `# H1` so the raw file reads well. The
  heading hook drops level-one headings, since the theme already shows the
  title.
- **Output:** the built site is a release artifact that can be hosted anywhere
  (Q-046). The API reference is also served by the binary (ADR-0012).

### Consequences

- Good: one pinned binary, fetched in seconds. Builds work offline.
- Good: the raw Markdown and the site read the same.
- Bad: shortcodes don't render raw. The style guide allows none until one is
  justified (see `docs/contributing/documentation-style.md`).
- Bad: the vendored theme is updated by hand, with `make docs-theme-update`.

### Confirmation

- `make docs` builds with no network access once the tools are fetched.
- htmltest checks the built site's internal links and anchors in `make ci`.
- An alert and a Mermaid diagram render on the site, and raw on GitLab and
  GitHub (M00 verification log, 2026-09-26).

## More information

- Supersedes [ADR-0011](0011-documentation-platform-hugo.md). Only the
  toolchain text changed.
- The theme spike and the overridden hooks are recorded in ITEM-0008.
- The toolchain is described in [ADR-0014](0014-toolchain-pinned-release-binaries-on-glibc-linux.md).
