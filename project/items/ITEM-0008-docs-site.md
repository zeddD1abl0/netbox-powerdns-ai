---
id: ITEM-0008
title: Documentation site build and theme spike
type: task
status: done
milestone: M00
requirements: [REQ-018, REQ-020, REQ-021, REQ-026]
depends_on: [Q-044, ITEM-0004]
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0008: Documentation site build and theme spike

## Goal

Build `docs/` into a static site with a pinned, Go-native toolchain. Pages must
render well on the site **and** raw on GitHub and GitLab.

## Acceptance criteria

- [x] Platform chosen and recorded in an ADR: Hugo, [ADR-0011](../../docs/adr/0011-documentation-platform-hugo.md).
- [x] A spike comparing Hextra and hugo-book against the criteria: no Node build, alert rendering, Mermaid, offline search, accessibility. Result in the notes.
- [x] The docs tool is pinned (`tools/hugo`, ADR-0013), and so is the theme (`site/go.mod`, vendored in `site/_vendor`).
- [x] GitHub alerts and Mermaid code blocks render on the site. Hextra's own render hooks handle both; the site overrides only the heading and link hooks.
- [x] `make docs` builds with no network access: verified with `GOPROXY=off` and an empty Hugo cache.
- [x] A link checker runs over the built site: `make docs-links` (htmltest), part of `make ci`.
- [x] A page with an alert (`docs/_index.md`) and one with a Mermaid diagram (ADR-0007) render on the site. Raw rendering on GitHub and GitLab is checked at M0 close, once the branch is pushed (M00 acceptance).

## Notes

- 2026-09-25: Spike result: **Hextra v0.12.3.**
  - It builds with the pinned standard (non-extended) Hugo 0.166 and needs no
    Node.
  - It renders GitHub alerts as callouts, and Mermaid from a local,
    fingerprinted script.
  - It has built-in FlexSearch offline search, and loads no external scripts
    or stylesheets.
  - Accessibility basics look sound: an ARIA-labelled search, a dark-mode
    toggle and semantic headings. A full audit is part of the WCAG work (Q-051).
  - hugo-book v0.15.0 built its assets but rendered no content pages from the
    mounted `docs/` with the same configuration. Hextra worked without changes,
    so hugo-book wasn't pursued further.
- 2026-09-25: Two Hextra hooks are overridden in `site/layouts/_markup/`:
  - `render-heading.html` drops level-1 headings, because every page carries
    its own `# Title` for raw readability and the theme already shows it. Each
    page now has exactly one H1.
  - `render-link.html` resolves repo-relative links against the page's source
    file. Links into `docs/` become site URLs, keeping their anchors. Links to
    files outside `docs/` (`CLAUDE.md`, the board, the excluded ADR template)
    go to the source on GitHub (`params.sourceURL`).

  Heading IDs use GitHub's scheme, so the same `#anchor` works on the site and
  on the forge.
- 2026-09-25: Negative check: a broken anchor and a missing page added to the
  built HTML made htmltest fail ("hash does not exist", "target does not
  exist").
- 2026-09-25: Hextra is MIT-licensed, and `hugo mod vendor` doesn't copy its
  license, so the notice is kept in `site/Hextra.LICENSE`.
  `make docs-theme-update` updates and re-vendors the theme.
