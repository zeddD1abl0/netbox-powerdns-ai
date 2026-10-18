---
id: ITEM-0111
title: 'database.url for PostgreSQL: TLS, the URL as a secret, and serve and the CLI on it'
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-032, REQ-054]
depends_on: [ITEM-0110]
created: 2026-10-18
closed:
---

# ITEM-0111: database.url for PostgreSQL: TLS, the URL as a secret, and serve and the CLI on it

## Goal

`serve` and the CLI on PostgreSQL (ADR-0046): `postgres://` and
`postgresql://` URLs in libpq's standard form, which pgx reads with
libpq's parameters, defaults (`sslmode=prefer`) and `PG*` environment;
`database.url` treated as a secret, with `NBPDNS_DATABASE_URL_FILE`, and
its password never logged or shown; `/status`'s database gains `backend`,
`url` without the password, and `tls`. Until ITEM-0112, `serve` on
PostgreSQL holds the advisory lock, and refuses to start while another
`serve` holds it, as the `flock` does. Docs: "Run nbpdns on PostgreSQL",
backing up with `pg_dump`, and supported versions.

## Acceptance criteria

- [ ] `nbpdns db status`, `db migrate`, `settings`, `groups`, `audit` and `serve` work on a `postgres://` URL.
- [ ] TLS and client certificates work through `sslmode`, `sslrootcert`, `sslcert` and `sslkey`; the password through a `passfile`, or the URL from `NBPDNS_DATABASE_URL_FILE`.
- [ ] The URL's password is in no log line, error, `/status` field or configuration fingerprint.
- [ ] `/status` reports the backend, the URL without its password, and whether the connection uses TLS; the spec and the contract tests cover them.
- [ ] A second `serve` on the same PostgreSQL database refuses to start.
- [ ] The how-to, the configuration reference, the backup how-to and the supported versions are updated, with a CHANGELOG line.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
