---
id: ITEM-0039
title: Leave a secret's value out of its config error
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-005, REQ-006]
depends_on: [ITEM-0038]
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0039: Leave a secret's value out of its config error

## Goal

ITEM-0038 made a secret that YAML reads as a number an error, so that it
can't silently change. Its message quoted the value, such as "want a string,
not 8675309421", and `nbpdns` prints the error on standard error, which can
end up in job logs or the journal. CLAUDE.md says a secret is never logged.
Name only the kind of value instead.

## Acceptance criteria

- [x] A secret that YAML reads as a number, a boolean, a list, a mapping or nothing is refused with an error that names only the kind of value, for the NetBox token and for API keys.
- [x] A test checks the value isn't in the error, for decimal, hex, float and boolean forms.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Found by `/security-review` at M02's close. Its filter rated it
  3 of 10 as a vulnerability, below the report's bar: it needs an
  all-numeric key left unquoted, the value goes only to the stderr of the
  operator who wrote it, and loading fails at once. Fixed anyway, since it
  breaks the rule that a secret is never logged, and the leak was new in
  ITEM-0038.
- 2026-10-06: Done. `secretString` names the kind of value: "not a number",
  "not true or false", or what `describe` says of a list, a mapping or an
  empty value, which never includes a value. `TestSecretValueNotInErrors`
  covers `8675309421`, `0x7f3a9c11d2` (which YAML reads as 546444153298),
  `1e10`, `123.456` and `true`, for `netbox.token` and `primary.api_key`.
