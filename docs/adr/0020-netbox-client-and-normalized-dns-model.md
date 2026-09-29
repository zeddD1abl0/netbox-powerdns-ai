---
title: "0020: NetBox client and normalized DNS model"
status: accepted
date: 2026-09-29
decision-makers: [jordan]
requirements: [REQ-024, REQ-027, REQ-040]
questions: [Q-007]
supersedes:
---

# 0020: NetBox client and normalized DNS model

## Context and problem statement

nbpdns reads its DNS data from the NetBox DNS plugin (ADR-0006). M01 builds
the client that does it. The client must decide:
- which NetBox API to use: ADR-0006 chose REST, but left GraphQL to evaluate;
- which NetBox and plugin versions it supports, and how it authenticates
  (Q-007);
- how it behaves on a slow, failing or insecure connection.

The data it reads must also be put into a form that PowerDNS's data can be
put into too, since M02 reads PowerDNS into the same model and M03 compares
the two. PowerDNS stores RRsets: all the records with one owner name and
type, sharing one TTL. NetBox stores single records, each with its own
optional TTL.

## Decision drivers

- Read everything the drift report needs, at the scale targets, without
  loading NetBox heavily (REQ-024, REQ-027).
- Supported versions are explicit and checked, not assumed.
- Least privilege: a token that can only read DNS objects.
- Fail clearly: an operator can tell a network fault from a permission
  problem from an unsupported version.
- The model compares cleanly with PowerDNS, without false drift.
- Tests use the real NetBox for API behavior (CLAUDE.md), yet cover failures
  that a real NetBox can't be made to produce.

## Considered options

1. **API:** REST, or GraphQL.
2. **Token:** v2 tokens only, v1 tokens only, or v2 with v1 still accepted.
3. **Plain HTTP:** refuse `http://` URLs, allow them only for local hosts, or
   allow them with a warning.
4. **Model:** single records, as NetBox stores them, or RRsets, as PowerDNS
   stores them.
5. **Transport tests:** the lab only, or the lab plus a local test server for
   transport failures.

## Decision outcome

**API: REST only.** The plugin's REST endpoints are stable, documented and
filterable, and paging them scales to the default targets. GraphQL would add
a second query language and schema to track across NetBox versions, for no
data the REST API lacks.

**Supported versions (Q-007, REQ-040):** NetBox 4.7 and 4.6, with the DNS
plugin 1.7.x and 1.6.x. The user chose these on 2026-09-27; 4.7.1 was current.
- The client reads `/api/status/` for the NetBox version and the installed
  plugins.
- A missing plugin is an error.
- An unsupported NetBox or plugin version is a warning, and the command
  carries on. `nbpdns netbox check` reports it as a failure.

**Authentication:** a read-only **v2 token**, sent as
`Authorization: Bearer nbt_…`. A v1 token still works, sent as
`Authorization: Token …`, with a warning that recommends v2. The token is a
secret (`netbox.token`, with a `_FILE` variant), and is never logged.

**Transport:**
- TLS 1.2 or later, verified against the system roots plus an optional CA
  file (`netbox.ca_file`). There's no option to skip verification.
- **Plain `http://` is allowed**, because not every NetBox deployment has TLS
  (the user, 2026-09-29). The token then crosses the network unencrypted, so
  the `netbox.url` key's reference entry warns about it, and nbpdns logs a
  warning whenever the URL uses `http://`.
- Every request has a timeout (`netbox.timeout`).
- GET requests are retried on network errors and on 429, 502, 503 and 504,
  a few times, with capped exponential backoff and jitter. A `Retry-After`
  header sets the next delay.
- Lists follow NetBox's `next` links, with `limit` set to
  `netbox.page_size`. Records are read one zone at a time (`zone_id`), with
  at most `netbox.concurrency` requests in flight.
- **Errors are typed:** NetBox unreachable, authentication failed, missing
  permission (naming the object type), unsupported version, and API errors
  with NetBox's own detail.
- Outgoing requests carry the W3C `traceparent` header.

**Normalized model** (`internal/dns`):
- **Views** hold **zones**, and zones hold **RRsets**. An RRset is every
  record with one owner name, class and type. It has one TTL and one or more
  values.
- **Names** are lowercase, absolute, and end with a dot. The zone apex (`@`)
  becomes the zone's name.
- **Relative targets** in CNAME, MX, NS, SRV and PTR values are made absolute
  against the zone.
- **TXT values** are in canonical form: each string quoted, with `"` and `\`
  escaped, strings separated by one space, and none longer than 255 bytes.
- **TTLs.** A record's effective TTL is its own, or else the zone's default.
  An RRset's TTL is the **lowest** effective TTL of its active records (the
  user, 2026-09-29). Records that disagree are reported, so the data can be
  fixed in NetBox.
- **Each value keeps its record's status** (active or inactive) and the
  plugin's **managed** flag (records the plugin generates, such as SOA, NS
  and PTR). Inactive records don't count toward the RRset's TTL.

**Tests:**
- Integration tests run against real NetBox 4.7 and 4.6 in the lab, with a
  least-privilege v2 token.
- Unit tests map responses recorded from the lab (`testdata/`). There's no
  hand-written fake of NetBox's API.
- Retries, timeouts, `Retry-After` and TLS are tested against a local HTTP
  test server that returns only status codes, headers and delays, since a
  real NetBox can't be made to fail on demand (the user, 2026-09-29).

### Consequences

- Good: one API, paged and bounded, with versions and permissions checked up
  front.
- Good: RRsets match PowerDNS, so M03 compares like with like, and TTL
  disagreements inside NetBox are caught at the source.
- Good: failures that can't be forced on a real NetBox are still tested.
- Bad: with `http://`, the token is exposed to anyone on the path. The
  warning makes the risk visible, but doesn't prevent it.
- Bad: the supported versions have to be updated, with the lab, when NetBox
  releases a new minor version.
- Bad: when records in one RRset disagree on TTL, NetBox and the DNS servers
  won't show the same TTL for some of them, until the data is fixed.

### Confirmation

- Integration tests pass against both lab versions, including a token
  without permission and an unsupported version.
- Table-driven unit tests cover every normalization rule above.
- `make generate-check` keeps the configuration reference, with the
  `netbox.url` warning, in step with the code.

## Pros and cons of the options

### GraphQL

- Good: one query can fetch zones with their records.
- Bad: a second schema and query language to support across NetBox versions,
  and harder to page and bound.

### Refusing plain HTTP, or allowing it only for local hosts

- Good: the token never crosses a network unencrypted.
- Bad: rules out deployments whose NetBox has no TLS, and the CI lab, which
  runs NetBox over HTTP on a Docker-in-Docker host.

### Single records as the model

- Good: mirrors NetBox exactly.
- Bad: PowerDNS has one TTL per RRset, so every comparison would group
  records anyway, and disagreeing TTLs would show up as drift that PowerDNS
  can't fix.

### The lab only, for transport tests

- Good: no test double of any kind.
- Bad: retries, `Retry-After`, timeouts and TLS failures go untested, because
  a real NetBox can't be made to produce them.

## More information

- ADR-0006 chose the NetBox DNS plugin and left GraphQL to evaluate.
- The NetBox REST API: <https://github.com/netbox-community/netbox/blob/main/docs/integrations/rest-api.md>.
- The NetBox DNS plugin: <https://github.com/peteeckel/netbox-plugin-dns>.
- Recorded by ITEM-0023, before its implementation, so that its number
  matches M01's approved design.
