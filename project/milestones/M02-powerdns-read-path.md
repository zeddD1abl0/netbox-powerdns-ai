---
id: M02
title: PowerDNS read path
status: in-progress # planned | in-progress | done
started: 2026-10-06
closed:
---

# M02: PowerDNS read path

## Goal

`nbpdns powerdns` commands that read every zone and RRset from each server
group's primary, through the PowerDNS API, into the same normalized model as
NetBox's data; server groups declared in the config file; and the NetBox
zones each group serves, by view. Read-only.

## Non-goals

- No comparison or drift report (M03), and no writes (M12).
- No secondaries, catalog zones or DNS queries to servers (M12, M13).
- No PowerDNS views (5.0's split-horizon feature): one zone per name on each
  server. ADR-0007 keeps split-horizon within one server out of v1.
- No zone metadata, TSIG keys or DNSSEC keys (Q-011, M12).
- No database (M07). Server groups come from the config file only, not from
  environment variables, flags or an API.

## Phases

| Phase | Items |
|---|---|
| M2a Groundwork | ITEM-0031 Share one HTTP client between the NetBox and PowerDNS clients; ITEM-0032 Normalize record values with miekg/dns v2 (ADR-0025) |
| M2b Config | ITEM-0033 Server groups in the config file |
| M2c Lab | ITEM-0034 PowerDNS lab with 5.1 and 5.0 primaries |
| M2d PowerDNS | ITEM-0035 PowerDNS client (ADR-0024, REQ-041); ITEM-0036 `nbpdns powerdns` commands, `netbox zones --group`, docs and CHANGELOG (REQ-042) |
| Carried | ITEM-0030 Restore the GitHub push mirror (blocked on the user) |

## Acceptance criteria

- [ ] ADR-0024 and ADR-0025 are accepted. Q-021, Q-022, Q-039, Q-043 and Q-053
  are answered, and REQ-041 and REQ-042 exist.
- [ ] The NetBox client runs on `internal/httpclient`, with its tests
  unchanged and passing.
- [ ] Every record type normalizes through miekg/dns. M01's tests still pass,
  and NetBox's and PowerDNS's forms of the same data give one value.
- [ ] Server groups load from the config file, with strict fields, validation,
  key files, redaction and their sources in `config show`. Table-driven tests
  cover them, and the configuration reference documents them.
- [ ] `make lab-up` starts PowerDNS 5.1 and 5.0 primaries beside NetBox 4.7.
  The integration tests pass against both, and the job's memory is measured
  and recorded.
- [ ] `nbpdns powerdns check`, `zones` and `records`, and `nbpdns netbox zones
  --group`, work as designed, as tables and as JSON, with the exit codes.
- [ ] The API key is never logged or printed. Requests carry `traceparent`,
  `http://` warns, and redirects aren't followed. TLS, CA files and client
  certificates are tested.
- [ ] The docs pages above exist, the generated references are current, and
  the CHANGELOG is updated.
- [ ] `/code-review high` and `/security-review` have run, since M02 handles
  API keys.
- [ ] The manual verification is recorded. The GitLab pipeline passes, and
  so does GitHub's once ITEM-0030 restores the mirror. The user has merged
  through an MR with a merge commit.

## Decided after approval

> [!IMPORTANT]
> Changed during implementation, on 2026-10-06, with the reasons recorded in
> the items named. These override the approved design below.
>
> - **Hex is uppercase in normalized values**, not lowercase: miekg/dns
>   prints SSHFP's and DS's hex in uppercase whatever case it's given, so
>   uppercase is the one form every type can share (ITEM-0032).
> - **A number too big for its field is a problem.** miekg/dns keeps it
>   modulo the field's size (an SRV port of 70000 becomes 4464), so nbpdns
>   compares the numbers it was given with the ones printed, and keeps a
>   mismatched value as given (ITEM-0032).
> - **Request log lines changed** to `http request`, with a `service`
>   attribute, since sloglint requires constant messages (ITEM-0031).
> - **`netbox zones` takes `--view` or `--group`, not both,** since a group
>   chooses its views (ITEM-0036).
> - **An HTML error page gives its title** in both clients' errors, so a
>   proxy's refusal reads on one line (ITEM-0036).

## Verification log

Append-only and dated. Record what was run and what was seen.

## Approved design

The plan approved on 2026-10-06, copied verbatim. Its headings are demoted two
levels to nest under this section; the text is unchanged. It's a snapshot.
Where it disagrees with an ADR or `CLAUDE.md`, they win.

### M02: PowerDNS read path

#### Context

M01 is merged (`7934f8b`): nbpdns reads NetBox 4.7's DNS plugin into a
normalized RRset model. M02 does the same for PowerDNS: it reads each server
group's primary through the PowerDNS API into that model, so that M03 can
compare the two. It's read-only, like M01, and it's the first milestone that
holds a PowerDNS API key, which can change every zone on its server.

Decided with the user in the M02 design session, 2026-10-06:
- **PowerDNS releases (Q-053):** 5.1 and 5.0. 5.1 is the current train
  (released 2026-06-03); 5.0 has critical fixes only, until about March 2027;
  4.9 reached end of life around September 2026.
- **Zones to groups (ADR-0007's open point):** by NetBox view. Each group
  lists the views it serves, and a view may be served by several groups.
- **Record data:** normalized with `codeberg.org/miekg/dns` (v2), for every
  record type.
- **Plain HTTP:** allowed for a PowerDNS API, with a warning, as for NetBox.
- **Reference setup (Q-022):** a TLS reverse proxy in front of the API, which
  passes all methods, with an optional client certificate (mTLS).
- **The lab** stays plain HTTP. TLS, CA files and client certificates are
  tested against local test servers, as in M01.
- **The lab's groups have primaries only.** Secondaries arrive with the catalog
  zones that feed them, in M12.

Why the API needs care, as discussed: a PowerDNS API key has no scopes and
can't be read-only; the API has no TLS of its own; so a leaked key, or one
read off the network, can rewrite DNS for every zone in the group. nbpdns
guards the key as it guards the NetBox token, and the docs give the
reference setup that keeps it off the network in clear.

#### Goal

`nbpdns powerdns` commands that read every zone and RRset from each server
group's primary, through the PowerDNS API, into the same normalized model as
NetBox's data; server groups declared in the config file; and the NetBox
zones each group serves, by view. Read-only.

#### Non-goals

- No comparison or drift report (M03), and no writes (M12).
- No secondaries, catalog zones or DNS queries to servers (M12, M13).
- No PowerDNS views (5.0's split-horizon feature): one zone per name on each
  server. ADR-0007 keeps split-horizon within one server out of v1.
- No zone metadata, TSIG keys or DNSSEC keys (Q-011, M12).
- No database (M07). Server groups come from the config file only, not from
  environment variables, flags or an API.

#### Decisions

- **ADR-0024, reading PowerDNS from server groups:**
  - **Q-021:** nbpdns calls each primary's API directly, over HTTPS where
    available. No agents on the DNS hosts.
  - **Q-022:** the reference setup is a TLS reverse proxy on the PowerDNS
    host, with the webserver bound to `127.0.0.1` (or a Unix socket, from
    5.0), `webserver-allow-from` set, the key stored hashed
    (`pdnsutil hash-password`), and an optional client certificate for
    nbpdns. All methods pass, so it serves M12 unchanged. `http://` URLs work
    with a warning, as for NetBox.
  - **Q-053:** PowerDNS Authoritative 5.1 and 5.0, checked like NetBox's
    releases. An unsupported release warns, and `powerdns check` fails it.
    A server that isn't authoritative, such as a Recursor, is an error.
  - **Zones to groups:** by NetBox view. A group lists its views; a view may
    be served by several groups (independent sites, REQ-030). A zone name in
    two views of one group is a problem nbpdns reports, since one PowerDNS
    server holds one zone per name.
  - **Q-043:** server groups are declared in the config file. When M07
    stores resources in the database, those from the file become
    `managed_by=file`.
  - **Q-039:** no backend interface or plugin runtime yet. The PowerDNS
    client returns the shared model; M03, the first consumer of two sources,
    defines the interface it needs. A plugin runtime needs its own ADR.
- **ADR-0025, record data normalized with miekg/dns v2:**
  `codeberg.org/miekg/dns`, BSD-3, parses and prints every record type.
  Pinned, since it's before 1.0 and its API may change between releases. v1
  (`github.com/miekg/dns`) is in maintenance and due to be archived; our own
  parsers would leave every unlisted type to false drift. It also serves the
  DNS queries M13's verification will need. ADR-0023's rules all still hold;
  this adds how each value is parsed.
- **Requirements:** REQ-041 "Reads PowerDNS Authoritative 5.1 and 5.0 through
  its HTTP API"; REQ-042 "Zones are assigned to server groups by NetBox view,
  and a view may be served by several groups". Q-021, Q-022, Q-039, Q-043
  and Q-053 move to Answered.
- **New dependency:** `codeberg.org/miekg/dns`, with its modules, each listed
  with its license in ITEM-0032.

#### Design

##### A shared HTTP client (`internal/httpclient`)

The PowerDNS client needs what the NetBox client already does, so that moves
out of `internal/netbox/client.go` into a shared package, unchanged in
behavior:
- the `http.Client`: TLS 1.2 or later, system roots plus a CA file, now
  also an optional client certificate (`cert_file`, `key_file`); redirects
  never followed; `tracing.Transport` for `traceparent`;
- GET with retries: `retryPolicy`, `delay` (jitter, `Retry-After`),
  `retryable` and `transient`, the 64 MiB body cap, and reading the body
  before decoding it;
- the `http://` warning, and debug logging without credentials.

Each API client keeps its own headers, error mapping and endpoints. The
NetBox client's tests pass unchanged; the transport tests move with the code.

##### Configuration

Scalar keys, in the registry (`internal/config/keys.go`), with env and flags:
`powerdns.timeout` (30s) and `powerdns.concurrency` (4).

Server groups are a list, in the config file only:

```yaml
powerdns:
  groups:
    - name: site-a            # [a-z0-9-], unique; used by --group
      views: [_default_]      # NetBox views it serves; at least one
      primary:
        url: https://pdns-a.example.com:8443
        api_key_file: /run/secrets/pdns-site-a   # or api_key; one of them
        server_id: localhost  # the default
        ca_file: /etc/nbpdns/pdns-ca.pem         # optional
        cert_file: /etc/nbpdns/client.pem        # optional, with key_file
        key_file: /etc/nbpdns/client-key.pem
```

- The registry gains a list key with a field schema. It drives strict
  decoding (an unknown field is an error, through mapstructure's
  `ErrorUnused`), validation, `config show` and the generated reference.
- **Validation**, all reported together: name and URL as `checkURL` checks
  them; exactly one of `api_key` and `api_key_file`; `cert_file` and
  `key_file` together; unique names.
- **`config show`** lists each group field as a row, such as
  `powerdns.groups.site-a.primary.url`, with the file as its source. API keys
  show as `[redacted]`.
- The reference says groups can only come from the file, and warns about
  `http://` and the key's power.

##### PowerDNS client (`internal/powerdns`)

Built on `internal/httpclient`, with the key sent as `X-API-Key` (a
`config.Secret`):
- `Server`: `GET /api/v1/servers/{server_id}`, giving the daemon type and
  version, which are checked against `Supported`.
- `Zones`: `GET …/zones?dnssec=false`, giving each zone's name, kind, serial
  and catalog.
- `Zone`: `GET …/zones/{id}`, giving the zone's RRsets, disabled records
  included.
- `ReadZones`: every zone, with at most `powerdns.concurrency` requests in
  flight.
- **Typed errors:** unreachable; key rejected (401); not authoritative or
  unknown `server_id`; zone not found; unsupported version; and API errors
  with PowerDNS's own `error` text.
- **Into the model:** names are already absolute and are lowercased. A
  record's status is `active` or `disabled`, with `Active` set accordingly,
  and nothing is `managed`. A zone's name servers come from its apex NS
  RRset. The NetBox-only zone fields, `view` and `default_ttl`, are empty.

##### Normalized model (`internal/dns`)

`Value` parses every type with miekg/dns (`NewData(type, value, origin)`) and
prints its canonical text, with domain names lowercased. NetBox's unquoted
TXT values keep their M01 rule. A value that doesn't parse is kept as given,
and reported as a problem, as now. A table test feeds NetBox's form and
PowerDNS's form of the same data for CAA, TLSA, SSHFP, DS, HTTPS, SVCB,
NAPTR, LOC, TXT, MX, SRV and SOA, and expects one value for each.

##### Commands

They follow M01's style: logs on stderr, output on stdout, `--output
table|json`, and exit status 0, 1 or 2.

| Command | Does |
|---|---|
| `nbpdns powerdns check [--group G]` | For each group: the connection (with the `http://` warning), the server (authoritative, version supported), the key accepted, and its zones readable. Any failure exits 1. |
| `nbpdns powerdns zones [--group G]` | Group, zone, kind, serial and catalog, sorted by group and canonical name |
| `nbpdns powerdns records --zone Z [--group G]` | One zone's RRsets, with disabled records shown and problems reported as in M01. `--group` is needed when there's more than one group. |
| `nbpdns netbox zones --group G` | Only the NetBox zones that group G serves, through its views. A zone name in two of its views is a problem. |

##### Lab (`deploy/dev/compose.yaml`)

- **Group `lab-a`:** a primary on PowerDNS 5.1, on port 8151.
- **Group `lab-b`:** a primary on PowerDNS 5.0, on port 8150.
- **Images:** the official `powerdns/pdns-auth-51` and `-50` images, pinned by
  digest, with the SQLite backend.
- **API access:** a lab-only API key, published, through `PDNS_AUTH_API_KEY`.
  The webserver listens on every interface, and its settings are passed on
  the command line, so there are no bind mounts.
- **Ports:** they bind as the NetBox lab's do (ITEM-0029). DNS ports aren't
  published.
- **`internal/lab`** lists the PowerDNS instances. `TestSupportedMatchesLab`
  covers PowerDNS too.
- **Fixtures** are created per test through the API: a zone with a random
  name, holding varied types (CAA, TLSA, HTTPS, a long TXT, multi-value
  RRsets, and a disabled record). The responses are recorded into
  `testdata/` with `-record`, for the unit tests.
- **CI memory** is measured again in an emulated job and recorded, with the
  GitLab job's comment updated. A PowerDNS server needs tens of MB.

##### Docs

| Section | Pages |
|---|---|
| Tutorial | "Read your PowerDNS zones with nbpdns", using the lab |
| How-to | "Connect nbpdns to PowerDNS" (groups, views, key files); "Put the PowerDNS API behind a TLS proxy" (the reference setup, checked by hand against the lab with a temporary proxy and a client certificate); "Run the development lab", updated |
| Reference | Configuration, command line and supported versions, all generated; the last now covers PowerDNS |
| Explanation | "How nbpdns reads PowerDNS": groups, assignment by view, primaries only, normalization, why the key matters. "How nbpdns reads NetBox" gains the miekg/dns note. |

`CHANGELOG.md` lines go under Unreleased.

#### Items and phases

| Phase | Items |
|---|---|
| M2a Groundwork | ITEM-0031 Share one HTTP client between the NetBox and PowerDNS clients; ITEM-0032 Normalize record values with miekg/dns v2 (ADR-0025) |
| M2b Config | ITEM-0033 Server groups in the config file |
| M2c Lab | ITEM-0034 PowerDNS lab with 5.1 and 5.0 primaries |
| M2d PowerDNS | ITEM-0035 PowerDNS client (ADR-0024, REQ-041); ITEM-0036 `nbpdns powerdns` commands, `netbox zones --group`, docs and CHANGELOG (REQ-042) |
| Carried | ITEM-0030 Restore the GitHub push mirror (blocked on the user) |

Once the plan is approved, one commit records the design: this plan, verbatim,
under **Approved design** in M02's file; ADR-0024 and ADR-0025; the answered
questions and the new REQs; the brief's dated answers; and the items. Each
item is then committed as it's done, on `m02-powerdns-read-path`.

#### Acceptance criteria

- [ ] ADR-0024 and ADR-0025 are accepted. Q-021, Q-022, Q-039, Q-043 and Q-053
  are answered, and REQ-041 and REQ-042 exist.
- [ ] The NetBox client runs on `internal/httpclient`, with its tests
  unchanged and passing.
- [ ] Every record type normalizes through miekg/dns. M01's tests still pass,
  and NetBox's and PowerDNS's forms of the same data give one value.
- [ ] Server groups load from the config file, with strict fields, validation,
  key files, redaction and their sources in `config show`. Table-driven tests
  cover them, and the configuration reference documents them.
- [ ] `make lab-up` starts PowerDNS 5.1 and 5.0 primaries beside NetBox 4.7.
  The integration tests pass against both, and the job's memory is measured
  and recorded.
- [ ] `nbpdns powerdns check`, `zones` and `records`, and `nbpdns netbox zones
  --group`, work as designed, as tables and as JSON, with the exit codes.
- [ ] The API key is never logged or printed. Requests carry `traceparent`,
  `http://` warns, and redirects aren't followed. TLS, CA files and client
  certificates are tested.
- [ ] The docs pages above exist, the generated references are current, and
  the CHANGELOG is updated.
- [ ] `/code-review high` and `/security-review` have run, since M02 handles
  API keys.
- [ ] The manual verification is recorded. The GitLab pipeline passes, and
  so does GitHub's once ITEM-0030 restores the mirror. The user has merged
  through an MR with a merge commit.

#### Verification

- `make check` on every commit, and `make test-integration` against both
  PowerDNS primaries and NetBox 4.7.
- **Manual, on a fresh lab:**
  1. Follow the tutorial: config with both groups, `config show` redacting
     the keys, then `powerdns check`, `zones` and `records`.
  2. A wrong key; a `server_id` that doesn't exist; a URL with nothing
     listening. Each gives a clear error and exit 1.
  3. A disabled record is shown as `disabled`.
  4. Put the same zone, with CAA, TLSA and HTTPS records, in NetBox and in
     PowerDNS, and see the same values from `netbox records` and
     `powerdns records`: a preview of M03.
  5. `netbox zones --group lab-a` lists only the zones in lab-a's views.
  6. The TLS proxy how-to, step by step, with a temporary proxy in front of
     lab-a's primary and a client certificate, using `ca_file`, `cert_file`
     and `key_file`.
  7. At `--log-level debug`, no line holds the API key.
- The emulated CI job's memory, before and after, is recorded in ITEM-0034.

#### Your side

- Restore the GitLab push mirror to GitHub (ITEM-0030). Its error is under
  Settings, then Repository, then Mirroring repositories.
