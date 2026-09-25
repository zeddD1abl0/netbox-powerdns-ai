---
id: ITEM-0008
title: Documentation site build and theme spike
type: task
status: open
milestone: M00
requirements: [REQ-018, REQ-020, REQ-021, REQ-026]
depends_on: [Q-044, ITEM-0004]
created: 2026-09-25
closed:
---

# ITEM-0008: Documentation site build and theme spike

## Goal

Build `docs/` into a static site with a pinned, Go-native toolchain. Pages must
render well on the site **and** raw on GitHub and GitLab.

## Acceptance criteria

- [ ] Platform chosen and recorded in an ADR (Q-044, ITEM-0009).
- [ ] Spike comparing the two candidate themes (Hextra and hugo-book, if Hugo is chosen). Criteria: no Node build, GitHub alert rendering, Mermaid, offline search, accessibility. The result goes in the ADR.
- [ ] The docs tool is pinned (ADR-0003).
- [ ] Render hooks for GitHub alerts and Mermaid code blocks.
- [ ] `make docs` builds the site with no network access after the first tool download.
- [ ] A link checker runs over the built site in `make check`.
- [ ] A test page with an alert and a Mermaid diagram renders on the site and raw on GitLab.

## Notes
