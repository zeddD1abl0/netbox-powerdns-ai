---
title: "0026: Read PowerDNS through its API, with PowerDNS 5.1 only"
status: accepted
date: 2026-10-06
decision-makers: [jordan]
requirements: [REQ-028, REQ-029, REQ-030, REQ-041, REQ-042]
questions: [Q-021, Q-022, Q-039, Q-043, Q-053]
supersedes: ADR-0024
---

# 0026: Read PowerDNS through its API, with PowerDNS 5.1 only

## Context and problem statement

M02 reads each PowerDNS server group's zones into the model that M01 reads
NetBox into (ADR-0023), so that M03 can compare them. ADR-0006 settled that
nbpdns uses only the PowerDNS HTTP API, and ADR-0007 that a server group is
one primary, which nbpdns talks to, and its secondaries. That leaves:
- how nbpdns reaches the primaries (Q-021), and how the API is secured in
  transit (Q-022);
- which PowerDNS releases are supported (Q-053);
- where server groups are declared (Q-043), and how NetBox's zones are
  assigned to them, which ADR-0007 left open;
- what extension points other backends get (Q-039).

The PowerDNS API raises the stakes. It authenticates with one static key,
sent in the `X-API-Key` header, with no scopes and no read-only form: any key
can create, change or delete every zone, record, TSIG key and DNSSEC key on
its server, and through zone transfers on the group's secondaries. The
web server that serves the API speaks plain HTTP only; its own protections
are the address it binds to and `webserver-allow-from`. M02 to M11 only
read, but the key nbpdns holds can write.

### Why this restates ADR-0024

ADR-0024 supported PowerDNS 5.1 and 5.0, each the primary of a server group
in the lab. The first pipeline with that lab failed: the integration job
needed more memory than the runner nodes had to spare, as it had when the
lab ran two NetBoxes (ADR-0023). The user decided on 2026-10-06: "Can we
concentrate on just PowerDNS 5.1 for the moment".

Accepted ADRs aren't edited, so this ADR restates ADR-0024 with the supported
PowerDNS releases narrowed to 5.1. **The rest of the decision is unchanged.**

## Decision drivers

- Read everything M03 needs from each group, through the API only (REQ-028).
- Support the topologies of REQ-029 and REQ-030: a primary per group, and
  independent sites that may serve the same zones.
- Supported releases are explicit, checked and tested, as for NetBox
  (ADR-0023).
- Keep the API key, which can rewrite DNS, as safe as nbpdns can make it,
  and give operators a setup that keeps it off the network in clear.
- Don't build abstractions before a second implementation needs them.
- The integration job fits the CI runners, which have little memory to
  spare.

## Considered options

1. **Reaching PowerDNS:** the API directly; or an agent on each DNS host.
2. **Securing the API:** require HTTPS; allow `http://` only on the same
   host; or allow `http://` with a warning, with a TLS proxy as the
   reference setup. For the proxy: pass all methods, or only GET until M12.
3. **Releases:** 5.1 only; 5.1 and 5.0; 5.1, 5.0 and 4.9. ADR-0024 chose 5.1
   and 5.0; the runners couldn't fit the lab that needed.
4. **Assigning zones to groups:** by NetBox view; by the zone's name servers;
   by a NetBox tag or custom field; or listed in the config file.
5. **Where groups are declared:** the config file; environment variables and
   flags as well.
6. **Extension points:** a backend interface now; or none until a second
   backend or M03 needs one.

## Decision outcome

**Q-021: the API, directly.** nbpdns calls each group's primary over HTTP,
HTTPS wherever the API is behind TLS. No agents run on the DNS hosts; an
agent mode stays a possible later extension.

**Q-022: `http://` is allowed with a warning, and the reference setup is a
TLS proxy that passes all methods.** The user chose both on 2026-10-06.
- nbpdns accepts an `http://` URL for a primary, logs a warning every time it
  connects that way, and its configuration reference warns that the key, which
  can change every zone, then crosses the network in clear.
- The reference setup, documented as a how-to:
  - the PowerDNS web server binds to `127.0.0.1`, or a Unix socket from 5.0,
    with `webserver-allow-from` set;
  - the API key is stored hashed (`pdnsutil hash-password`);
  - a reverse proxy on the same host terminates TLS, and can require a client
    certificate from nbpdns (mTLS);
  - all methods pass, so the same setup serves M12's writes unchanged.
- nbpdns supports what that setup needs: TLS 1.2 or later, verified against
  the system roots plus a CA file, and an optional client certificate and
  key. There's no option to skip verification, and redirects aren't followed,
  so the key goes only to the configured URL.
- The key is a `config.Secret`: redacted wherever it could be printed or
  logged, and readable from a file.

**Q-053: PowerDNS Authoritative 5.1.** 5.1 is the current release train
(released 2026-06-03). 5.0, which gets critical fixes until about March 2027,
is no longer supported: the user chose 5.1 only on 2026-10-06, to fit the CI
runners. 4.9 reached end of life around September 2026.
- nbpdns reads `/api/v1/servers/{server_id}` for the daemon type and version.
  A server that isn't authoritative, such as a Recursor, is an error.
- An unsupported release is a warning, and the command carries on;
  `nbpdns powerdns check` reports it as a failure, as for NetBox.
- Each supported release runs in the lab, and the integration tests read
  from it. A release is added to `Supported` only with its lab instance, and
  only when the runners have room for one more PowerDNS server and its
  image.

**Assigning zones to groups: by NetBox view.** Each group lists the NetBox
views it serves, and a view may be served by more than one group, as
independent sites serving the same zones are (REQ-030). A NetBox zone is
served by every group that lists its view. One PowerDNS server holds one zone
of each name, so a zone name in two views of the same group is a problem that
nbpdns reports. The user chose this on 2026-10-06.

**Q-043: server groups are declared in the config file.** Each group is an
entry under `powerdns.groups`: a name, its views, and its primary's URL, API
key or key file, server ID, and optional CA file and client certificate.
Groups are structured resources, not settings, so they have no environment
variables or flags. When M07 stores resources in the database, those
declared in the file become `managed_by=file`.

**Q-039: no backend interface yet.** The PowerDNS client returns the shared
model (`internal/dns`). M03, the first code that uses two sources, defines
the interface it needs, in the Go way. A plugin runtime needs an ADR of its
own.

**Reading:**
- The zones list, `GET …/zones?dnssec=false`, gives each zone's name, kind,
  serial and catalog. The zone itself, `GET …/zones/{id}`, gives its RRsets,
  disabled records included. Neither pages.
- Zones are read with at most `powerdns.concurrency` requests in flight,
  each within `powerdns.timeout`. GET requests are retried as the NetBox client
  retries them; the two clients share one HTTP client.
- Errors are typed: unreachable; key rejected; not an authoritative server,
  or no such `server_id`; zone not found; unsupported release; and API
  errors with PowerDNS's own `error` text.
- Into the model: names are made lowercase; a record's status is `active` or
  `disabled`; nothing is managed; a zone's name servers come from its apex
  NS RRset; NetBox's view and default TTL are empty.

### Consequences

- Good: one write-capable key per group, held only by nbpdns, guarded like
  the NetBox token, with a documented setup that keeps it off the network in
  clear and can demand a client certificate as well.
- Good: assignment by view needs no NetBox changes, and fits both a zone
  served by several sites and a zone name that differs between views.
- Good: no abstraction is built before its second user.
- Bad: with `http://`, the key that can rewrite every zone crosses the
  network in clear. The warning makes it visible; it doesn't prevent it.
- Bad: PowerDNS can't give nbpdns a read-only key. Until M12, nothing but
  the operator's own proxy rules stops a leaked key from writing.
- Bad: groups can't come from the environment, so a container deployment
  needs a config file, even if its secrets come from files.
- Good: the lab runs one PowerDNS server, so the integration job needs less
  memory and disk than with two.
- Bad: deployments on PowerDNS 5.0 lose support. nbpdns still reads from
  them, with a warning, but nothing tests it, and `nbpdns powerdns check`
  fails.
- Bad: the lab has one server group, so behavior across several groups is
  tested against local test servers only, not the lab.
- Bad: the supported release has to be updated, with the lab, as new trains
  appear.

### Confirmation

- `TestSupportedMatchesLab` fails if the supported PowerDNS releases and the
  lab's primaries differ.
- Integration tests read fixtures from the lab's 5.1 primary, including a
  wrong key and an unknown `server_id`. The version check is also
  unit-tested against a list of two releases.
- Local test servers cover TLS, a CA file, a client certificate, redirects
  and retries.
- The TLS proxy how-to is checked by hand against the lab, with a client
  certificate.

## Pros and cons of the options

### Agents on the DNS hosts

- Good: the API could stay on `127.0.0.1`.
- Bad: one more program to deploy, update and secure on every primary, for
  what a TLS proxy gives.

### Requiring HTTPS, or allowing `http://` only on the same host

- Good: the key never crosses a network in clear.
- Bad: every deployment, and the lab, would need a proxy before nbpdns could
  even read; CI reaches the lab by name, so it would need an exception. The
  user chose consistency with NetBox's rule.

### A proxy that passes only GET until M12

- Good: a leaked key couldn't write through the proxy while nbpdns only
  reads.
- Bad: the setup changes when M12 writes. The user chose one setup that
  serves both.

### Assigning by name servers, by tag or custom field, or by a list of zones

- Name servers: no configuration per zone, but hidden primaries aren't in
  the NS set, and groups that share NS names (anycast) can't be told apart.
- A tag or custom field: the most flexible, but every zone must carry it,
  and nbpdns's NetBox token would need to read it.
- A list of zones: fully explicit, but each new zone in NetBox needs a
  config change.

### Groups from environment variables and flags too

- Good: a container could run without a config file.
- Bad: a list of groups, each with its own key, doesn't map onto variables
  and flags without an invented naming scheme.

### A backend interface now

- Good: other DNS vendors would have a place to plug in.
- Bad: designed with one implementation and no consumer, it would likely be
  wrong for both.

## More information

- ADR-0024, which this supersedes; ADR-0006 (PowerDNS through its API only),
  ADR-0007 (server groups and catalog zones), ADR-0023 (the model).
- The PowerDNS HTTP API: <https://doc.powerdns.com/authoritative/http-api/>.
- The PowerDNS release lifecycle: <https://doc.powerdns.com/authoritative/appendices/EOL.html>.
- ADR-0024 was recorded in M02's design, 2026-10-06, and implemented by
  ITEM-0031 and ITEM-0033 to ITEM-0036. This restatement is recorded by
  ITEM-0037.
