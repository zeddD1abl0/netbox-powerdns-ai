---
id: ITEM-0111
title: 'database.url for PostgreSQL: TLS, the URL as a secret, and serve and the CLI on it'
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-032, REQ-054]
depends_on: [ITEM-0110]
created: 2026-10-18
closed: 2026-10-22
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

- [x] `nbpdns db status`, `db migrate`, `settings`, `groups`, `audit` and `serve` work on a `postgres://` URL.
- [x] TLS and client certificates work through `sslmode`, `sslrootcert`, `sslcert` and `sslkey`; the password through a `passfile`, or the URL from `NBPDNS_DATABASE_URL_FILE`.
- [x] The URL's password is in no log line, error, `/status` field or configuration fingerprint.
- [x] `/status` reports the backend, the URL without its password, and whether the connection uses TLS; the spec and the contract tests cover them.
- [x] A second `serve` on the same PostgreSQL database refuses to start.
- [x] The how-to, the configuration reference, the backup how-to and the supported versions are updated, with a CHANGELOG line.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
- 2026-10-18: Started. Plan:
  - `internal/config`: `database.url` becomes a `Secret`, with its `_FILE`
    variants, and takes `sqlite:`, `postgres://` and `postgresql://` URLs.
    `config show`, the fingerprint and its errors show it with any password,
    in the userinfo or in `password` and `sslpassword`, masked as `xxxxx`.
    The config only checks the form; pgx reads the rest when the store opens,
    so a missing certificate doesn't stop `nbpdns drift`.
  - `internal/store`: the store knows its backend, whether its connection
    uses TLS (pgconn's `net.Conn` is a `*tls.Conn`), and the server's release,
    which is checked against `store.Supported` (18), with a warning, as for
    NetBox and PowerDNS. `(*Store).Lock` takes a session advisory lock on a
    connection of its own, names its holder through `application_name`, and
    `Close` discards the session. ITEM-0112 replaces it with the leader's.
  - `internal/cli`: `openStore` picks the backend from the URL. serve,
    `db migrate` and `secrets rotate-master-key` take the lock: the flock
    first on SQLite, as in M08, and the advisory lock after opening on
    PostgreSQL. `db status` and serve's log show the backend, the masked URL
    and TLS.
  - `/status`'s `database` gains `backend`, `url` and `tls` (null on SQLite),
    spec first. `path` stays a string, empty on PostgreSQL, so the change is
    additive.
  - Tests: config forms and masking; a TLS test through a TLS-terminating
    proxy in the test, in front of the lab's PostgreSQL, with a client
    certificate and `verify-full`; `passfile`; `sslmode=require` refused;
    the CLI and serve on the lab, with the password checked absent from
    every output.
- 2026-10-22: Done, as planned on 2026-10-18; the work was left
  uncommitted then, and verified and finished now.
  - `internal/config`: `database.url` is a `Secret`, with its `_FILE`
    variants, that takes `sqlite:`, `postgres://` and `postgresql://`
    URLs. `maskURL` masks the userinfo's password and the `password` and
    `sslpassword` parameters as `xxxxx`, in `config show`, the setting
    that the fingerprint hashes, `ShownURL` and every error; a URL that
    doesn't parse is shown as `[redacted]`.
  - `internal/store`: `Backend`, `TLS`, `Cleartext` and `ServerRelease`,
    from the connection that `OpenPostgres` makes at once; `Supported`
    (18), which generates the supported versions' PostgreSQL section.
    `(*Store).Lock` takes `pg_try_advisory_lock(serveLockKey)` on a
    connection of its own, names its holder through `application_name`,
    and `Close` ends that session. ITEM-0112 replaces it with the leader's.
  - `internal/cli`: `openStore` picks the database from the URL, and warns
    of an unsupported release or a connection that crosses the network
    unencrypted. `openLocked` takes the flock before opening SQLite, and
    the advisory lock after opening PostgreSQL, for `serve`, `db migrate`
    and `secrets rotate-master-key`. `config show` waits 10 s at most for
    PostgreSQL.
  - `/status`, `/api/status` and `db status` gain `backend`, `url` and
    `tls`, spec first; `TestStatus`, the API's contract test, has a case
    on each database.
  - Tests: the config's forms and masking; `TestPostgresTLS`, through a
    TLS server in the test in front of the lab's PostgreSQL, with a client
    certificate, `verify-full`, `verify-ca`, `require`, a passfile,
    `PGPASSWORD` and `PGSSLMODE`; `TestCLIOnPostgres` and
    `TestServeOnPostgres`, with the password checked absent from every
    output, and a second `serve` refused.
  - Verified: `make check`; `make test-integration`, in 1 min 53 s, its
    largest process at 1.38 GiB. In the lab, the backup how-to's
    PostgreSQL steps, with PostgreSQL 18.6's `pg_dump` and `pg_restore`
    as nbpdns's role, through a passfile: the restored copy verifies and
    keeps its settings. `sslmode=require` is refused with "server refused
    TLS connection"; the default, `prefer`, connects unencrypted, with
    the warning.
  - Found on the way: `make vuln` failed on GO-2026-6809, published on
    2026-10-21, so ITEM-0118 updated Go to 1.27.3, committed first, as
    7147165. `nbpdns audit verify` says "1 events": ITEM-0119.
