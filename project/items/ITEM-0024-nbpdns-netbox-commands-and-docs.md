---
id: ITEM-0024
title: nbpdns netbox commands and docs
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-018, REQ-020, REQ-021]
depends_on: [ITEM-0023]
created: 2026-09-27
closed: 2026-10-06
---

# ITEM-0024: nbpdns netbox commands and docs

## Goal

`nbpdns netbox check`, `zones` and `records`, with the M01 docs and
CHANGELOG, so the milestone is usable end to end.

## Acceptance criteria

- [x] The commands work as in M01's approved design, including `--output table|json`, with a non-zero exit and a clear message on failure.
- [x] The docs exist:
  - tutorial: "Read your NetBox DNS data with nbpdns";
  - how-to: "Give nbpdns read-only access to NetBox", "Configure nbpdns" and "Run the development lab";
  - reference: "Supported versions";
  - explanation: "How nbpdns reads NetBox" and "Configuration sources and precedence".
- [x] `CHANGELOG.md` has lines under Unreleased.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-27: Created from M01's approved design, before implementation
  started.
- 2026-10-06: Decided with the user (recorded in `project/brief.md`):
  - data problems that normalization works around are reported, as a
    warning on standard error and in the JSON output, and the command still
    succeeds;
  - `records --zone Z` without `--view` fails, listing the views, when the
    zone name exists in more than one view;
  - inactive records are shown, with their status.
- 2026-10-06: Done.
  - **`nbpdns netbox check`** prints one row per check (connection, netbox,
    plugin, token, and one per object type), each `ok`, `warning` or
    `failed`, as a table or JSON with an overall `ok`. Any failed row exits
    with status 1; warnings (`http://`, a v1 token) don't. It stops early
    when the rest can't pass: NetBox unreachable, the token rejected, or no
    plugin. An unsupported release is a failed row, as ADR-0020 says.
  - **`zones`** lists the zones in the normalized form, filtered by
    `--view` and `--status`, sorted by view and canonical name.
  - **`records --zone Z [--view V]`** finds the zone, then lists its
    RRsets, one row per value with the RRset's TTL, status and managed
    flag; inactive records are shown. Problems are logged as warnings and
    listed under `problems` (never null) in JSON, and the command still
    succeeds. A zone name in more than one view fails, naming the views.
    `--zone` missing or not ASCII is a usage error (status 2).
  - A missing `netbox.url` or `netbox.token` names every way to set it
    (`config.UnsetError`).
  - **Supported versions** is a generated reference page, from
    `netbox.Supported`, so the releases are declared once.
  - **Docs:** the tutorial, two how-to guides and two explanations, all
    written against the lab and checked by running each step: the
    tutorial's `curl` and `nbpdns` steps on both NetBoxes, and the
    read-only access guide's REST steps on 4.7. `run-the-development-lab.md`
    now says the tests create and remove their own data. NetBox's menu is
    labeled "Admin", which the Google style rejects, so the guide says
    "administration menu". Adding `REST` to the Vale vocabulary would also
    make Vale.Terms reject the word "rest", so headings avoid it instead.
  - **Tests:** unit tests for exit codes, usage errors, the unset-key
    message, the generated reference and both output forms of `records`;
    integration tests run every command against both NetBoxes with the
    fixture's least-privilege token, a token without permissions, a bad
    token, and a token file.
