---
title: "0023: NetBox client and normalized DNS model, with NetBox 4.7 only"
status: accepted
date: 2026-10-06
decision-makers: [jordan]
requirements: [REQ-024, REQ-027, REQ-040]
questions: [Q-007]
supersedes: ADR-0020
---

# 0023: NetBox client and normalized DNS model, with NetBox 4.7 only

## Context and problem statement

ADR-0020 decided how nbpdns reads the NetBox DNS plugin, and supported two
releases: NetBox 4.7 and 4.6, with the plugin 1.7.x and 1.6.x. Every
supported release is tested, so the development lab ran one NetBox for each,
each with its own PostgreSQL server, and the CI integration job ran them all.

The first real pipeline with that job failed: the job needs more memory than
the runner nodes have to spare. Most of it is NetBox itself. Each NetBox uses
about 1 GB while it applies its first-start migrations and serves the tests,
so each supported release adds about 1 GB to the job.

The user decided on 2026-10-06: "it needs to narrow down to just testing
against 4.7, and we'll add other versions and support from there."

Accepted ADRs aren't edited, so this ADR restates ADR-0020 with the supported
releases narrowed to NetBox 4.7. **The rest of the decision is unchanged.**

## Decision drivers

As in ADR-0020:
- Read everything the drift report needs, at the scale targets, without
  loading NetBox heavily (REQ-024, REQ-027).
- Supported versions are explicit and checked, not assumed.
- Least privilege: a token that can only read DNS objects.
- Fail clearly: an operator can tell a network fault from a permission
  problem from an unsupported version.
- The model compares cleanly with PowerDNS, without false drift.
- Tests use the real NetBox for API behavior (CLAUDE.md), yet cover failures
  that a real NetBox can't be made to produce.

And now:
- The integration job fits the CI runners, which have little memory to
  spare.

## Considered options

For the supported releases, given the runners' memory:
1. **Keep 4.7 and 4.6**, and ask for runners with more memory.
2. **Test 4.7 only, but still support 4.6**, untested.
3. **Test each release in its own job**, one NetBox at a time.
4. **Support and test 4.7 only**, and add releases back as the runners
   allow.

The other choices are ADR-0020's, and are decided as it decided them: REST
rather than GraphQL; v2 tokens, with v1 still accepted; plain HTTP allowed
with a warning; RRsets as the model; the lab plus a local test server for
transport failures.

## Decision outcome

Chosen option: **support and test NetBox 4.7 only** (option 4), because it
fits the runners without giving up the rule that every supported release is
tested. The user chose it on 2026-10-06.

**API: REST only.** The plugin's REST endpoints are stable, documented and
filterable, and paging them scales to the default targets. GraphQL would add
a second query language and schema to track across NetBox versions, for no
data the REST API lacks.

**Supported versions (Q-007, REQ-040):** NetBox 4.7, with the DNS plugin
1.7.x. NetBox 4.7.1 was current when this was decided.
- The client reads `/api/status/` for the NetBox version and the installed
  plugins.
- A missing plugin is an error.
- An unsupported NetBox or plugin version, now including 4.6, is a warning,
  and the command carries on. `nbpdns netbox check` reports it as a failure.
- **Adding a release** adds it to `netbox.Supported`, a NetBox for it to the
  lab, and its responses to the unit tests' `testdata/`, together. A unit
  test fails if the supported list and the lab differ. Each release adds
  about 1 GB to the integration job, so one is added only when the runners
  have room for it.

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
- Integration tests run against real NetBox 4.7 in the lab, with a
  least-privilege v2 token.
- Unit tests map responses recorded from the lab (`testdata/`). There's no
  hand-written fake of NetBox's API.
- Retries, timeouts, `Retry-After` and TLS are tested against a local HTTP
  test server that returns only status codes, headers and delays, since a
  real NetBox can't be made to fail on demand (the user, 2026-09-29).
- The version check is also unit-tested against a list of two releases, so
  the code path for several releases stays covered while only one is
  supported.

### Consequences

- Good: the integration job needs one NetBox and one PostgreSQL server, about
  half the memory it needed, and starts faster.
- Good: every supported release is still tested; nothing is claimed that CI
  doesn't check.
- Good, as in ADR-0020: one API, paged and bounded, with versions and
  permissions checked up front; RRsets match PowerDNS, so M03 compares like
  with like; failures that can't be forced on a real NetBox are still tested.
- Bad: deployments on NetBox 4.6 lose support. nbpdns still reads from them,
  with a warning, but nothing tests that it reads correctly, and
  `nbpdns netbox check` fails.
- Bad: nothing tests that the client works across NetBox releases, so a
  difference between releases is only found when the next release is added.
- Bad, as in ADR-0020: with `http://`, the token is exposed to anyone on the
  path; the supported version has to be updated, with the lab, when NetBox
  releases a new minor version; when records in one RRset disagree on TTL,
  NetBox and the DNS servers won't show the same TTL for some of them, until
  the data is fixed.

### Confirmation

- `TestSupportedMatchesLab` fails if `netbox.Supported` and the lab's NetBox
  instances differ.
- Integration tests pass against the lab's NetBox 4.7, including a token
  without permission and an unsupported version.
- Table-driven unit tests cover every normalization rule above.
- `make generate-check` keeps the supported versions reference and the
  configuration reference, with the `netbox.url` warning, in step with the
  code.

## Pros and cons of the options

### Keep 4.7 and 4.6, with bigger runners

- Good: the support ADR-0020 promised stays.
- Bad: the runners don't have the memory, and getting more isn't in the
  project's hands.

### Test 4.7 only, but still support 4.6

- Good: 4.6 deployments keep their support, and CI fits.
- Bad: support would be claimed for a release nothing tests, against the
  driver that supported versions are checked, not assumed.

### A job per release

- Good: each job runs one NetBox, so the peak memory per job falls, and both
  releases stay supported.
- Bad: the jobs still need a runner each, at once or one after another, so
  the pipeline's total demand doesn't fall, and with two jobs it takes
  longer or needs two nodes at once. The lab, the Makefile and both forges'
  CI files would need a release matrix.

### The other choices

As argued in ADR-0020: GraphQL adds a second schema for no extra data;
single records would make disagreeing TTLs into drift that PowerDNS can't
fix; and the lab alone can't produce the transport failures the tests need.

## More information

- ADR-0020, which this supersedes, and ADR-0006, which chose the NetBox DNS
  plugin.
- The NetBox REST API: <https://github.com/netbox-community/netbox/blob/main/docs/integrations/rest-api.md>.
- The NetBox DNS plugin: <https://github.com/peteeckel/netbox-plugin-dns>.
- Recorded by ITEM-0028.
