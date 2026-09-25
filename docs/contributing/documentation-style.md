---
title: Documentation style
weight: 10
---

# Documentation style

These rules apply to every page under `docs/`, and to `README.md` and
`CHANGELOG.md`. Vale checks the mechanical parts: run `make docs-lint`.

## Standards

- **Structure:** [Diátaxis](https://diataxis.fr/). Each page is exactly one of
  tutorial, how-to guide, reference, or explanation. If a page is trying to be
  two, split it.
- **Style:** the [Google developer documentation style guide](https://developers.google.com/style).
  Where this page and the guide differ, this page wins. As in the guide,
  spelling is American English and lists use the serial (Oxford) comma.
- **ADRs:** accepted ADRs are never edited (ADR-0001), so they can't be changed
  to satisfy style rules. Vale checks them for spelling and banned words only.

## Writing

- Write for the reader's task. Put the answer or the steps first and background
  second.
- Use plain words and short sentences. Leave out marketing language and filler,
  such as `simply`, `just`, `easily`, `powerful`, and `seamless`.
- Use the second person ("you") and the active voice. Write instructions in the
  imperative: "Run `make check`."
- Use sentence case for headings.
- Define a term the first time you use it, or link to where it's defined.
- Use the same name for the same thing everywhere. The UI, API, config keys,
  and docs all use one vocabulary.

## Markdown

Every page must read cleanly as **raw Markdown on GitHub and GitLab**, as well
as on the built site (ADR-0003).

- **Callouts:** GitHub alert syntax only. Both forges and the site render it.

  ```markdown
  > [!NOTE]
  > Useful information.

  > [!WARNING]
  > Something that could cause data loss or an outage.
  ```

  The allowed types are `NOTE`, `TIP`, `IMPORTANT`, `WARNING` and `CAUTION`.
- **Diagrams:** Mermaid in a fenced `mermaid` code block. Don't use images of
  diagrams unless Mermaid can't express the diagram.
- **Code:** fenced blocks, always with a language tag (`go`, `yaml`, `shell`,
  `json`, `text`). Shell examples show the command without a `$` prompt.
- **Links:** relative links between pages, pointing at the `.md` file, so they
  work in the repo and on the site.
- **Front matter:** each page has `title` and `weight` (its order within the
  section). Nothing else unless the site needs it.
- **Section index pages** are named `_index.md`.
- **Shortcodes:** none are allowed yet. A shortcode is added only when plain
  Markdown has no equivalent, and it's listed here when added. It must also
  degrade to readable text in raw Markdown.

## Generated pages

Some reference pages are generated from code (see the [reference
section](../reference/)). They start with a comment naming their generator.
Change the source, then regenerate. Never edit a generated page by hand.

## Where a change goes

| You changed… | Update… |
|---|---|
| An API endpoint | `api/openapi.yaml` first; the reference regenerates |
| A config key, metric, audit event, or permission | its registry in code; the reference regenerates |
| Behavior a user would notice | the relevant how-to or explanation page, and `CHANGELOG.md` |
| A design decision | a new ADR |
