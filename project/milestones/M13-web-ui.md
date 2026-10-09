---
id: M13
title: Web UI
status: planned # planned | in-progress | done
started:
closed:
---

# M13: Web UI

A roadmap entry, written ahead on `plan-m08-m12` (ADR-0037), and added by
ADR-0043. Its design, items and ADRs come in a plan-mode session before it
starts.

## Goal

Operators run nbpdns from a browser: they see drift and health, change
settings and groups, read the audit trail, and manage users, roles and
tokens, each as their roles allow.

## Scope

- **Q-016's pages:**
  - an ops dashboard: each group's drift and health, the refreshes, and
    NetBox's webhooks;
  - a zone's drift and its history;
  - settings and groups, with `managed_by` respected, so that a
    file-managed group is read-only;
  - the audit viewer, with the chain's state;
  - users, roles and tokens.
- **Q-050:** server-rendered templ templates, generated and type-checked,
  with htmx; assets vendored and embedded; no Node build; a strict
  Content-Security-Policy, as `/api/docs` has.
- **Q-051:** WCAG 2.2 AA, checked in CI; English only, with the text kept in
  one place for later translation.
- Each page shows only what the signed-in user may do (M11).

## Non-goals

- **No record editor:** NetBox is the editor.
- **No approvals:** they come with change safety, M15.
- **No translations.**

## Dependencies

- M12: sign-in and sessions.
- M11: permissions, for what each page shows.
- M08 to M10: the data the pages show and change.

## Decided, 2026-10-09

- Q-016's scope, templ and htmx (Q-050), and WCAG 2.2 AA in English
  (Q-051).

## Open for its design

- How CI checks WCAG 2.2 AA without a Node toolchain.
- htmx under a strict Content-Security-Policy, with no inline script.
- How the dashboard stays current: polling, or server-sent events.
