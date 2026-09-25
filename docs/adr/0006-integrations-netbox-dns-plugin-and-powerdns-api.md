---
title: "0006: Integrations — read the NetBox DNS plugin, write through the PowerDNS API only"
status: accepted
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-024, REQ-027, REQ-028]
questions: [Q-006, Q-008]
---

# 0006: Integrations — read the NetBox DNS plugin, write through the PowerDNS API only

## Context and problem statement

ADR-0004 made NetBox the source of truth and PowerDNS Authoritative the target.
Two questions were left open:
- **Where in NetBox does the DNS data live?** Core IPAM has only a `dns_name`
  field on IP addresses. The NetBox DNS plugin (`netbox-plugin-dns`) models
  zones, records, views, nameservers and DNSSEC policies.
- **How does the app change PowerDNS?** Options are the HTTP API, the backend
  database, or zone files.

## Decision drivers

- The source of truth should be able to express full zones, not only
  address records.
- Don't duplicate logic that already exists and is maintained elsewhere.
- Work regardless of which backend each PowerDNS server uses.
- Keep a single, auditable write path.

## Considered options

For the NetBox side:
1. The NetBox DNS plugin.
2. Core IPAM `dns_name` only.
3. Both.

For the PowerDNS side:
1. The PowerDNS HTTP API only.
2. Direct backend access (SQL, LMDB, zone files).

## Decision outcome

**NetBox side (Q-006):** the **NetBox DNS plugin**.
- Zones, records, nameservers and SOA come from the plugin's models, read
  through the NetBox REST API. GraphQL is an option to evaluate in M3.
- Address records derived from IPAM come through the plugin's own IPAM DNSsync
  feature. This app doesn't derive records from IPAM itself.

**PowerDNS side (Q-008):** the **PowerDNS HTTP API only**.
- In the user's words: "Ideally, we'll use the PowerDNS API rather than direct
  interaction with zone information."
- The app never reads or writes PowerDNS backend databases, LMDB files or zone
  files.

### Consequences

- Good: one write path. Every change goes through an API call the app can log,
  audit and retry.
- Good: the app doesn't care which backend is in use, as long as the API can
  write to it.
- Constraint: **the server the app writes to in each group must use a backend
  the API can write to.** The generic SQL backends (gpgsql, gmysql) and LMDB
  qualify. The BIND backend doesn't: the API can't create or change its zones.
  Secondaries aren't affected, because they receive zones over AXFR
  (ADR-0007).
- Constraint: the app depends on the NetBox DNS plugin and its API. Supported
  plugin versions are tracked with NetBox versions (Q-007).
- Consequence: the PowerDNS API has no native TLS. How it's secured in transit
  is Q-022.

### Confirmation

- Design review in M3/M4: there is no database driver or file access for
  PowerDNS data anywhere in the code.
- Integration tests run against real PowerDNS through its API (REQ-036).

## More information

- The NetBox DNS plugin: <https://github.com/peteeckel/netbox-plugin-dns>.
- The PowerDNS Authoritative HTTP API: <https://doc.powerdns.com/authoritative/http-api/>.
