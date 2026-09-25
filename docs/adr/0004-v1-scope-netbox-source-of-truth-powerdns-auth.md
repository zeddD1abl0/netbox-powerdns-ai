---
title: "0004: v1 scope — NetBox is the source of truth, PowerDNS Authoritative is the target"
status: accepted
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-009, REQ-023, REQ-024]
questions: [Q-001, Q-002]
---

# 0004: v1 scope — NetBox is the source of truth, PowerDNS Authoritative is the target

## Context and problem statement

The brief says the application will "manage multiple DNS servers in various
complex setups", but doesn't say which DNS software, or where DNS data is
authored. Both answers shape the data model, the sync engine, the UI and the
IaC story.

## Decision drivers

- The repository name (`netbox-powerdns-ai`) and the user's existing tooling
  (NetBox, PowerDNS, LightningStream).
- Keep v1 scope achievable, and leave a clear extension path (REQ-016).

## Considered options

For the DNS software:
1. PowerDNS Authoritative only.
2. PowerDNS Authoritative, Recursor and dnsdist.
3. Multiple vendors behind a pluggable backend.

For NetBox's role:
1. NetBox is the source of truth.
2. This app is the source of truth, and NetBox supplies IPAM data.
3. The owner is chosen per zone.
4. No NetBox integration in v1.

## Decision outcome

Chosen: **PowerDNS Authoritative only** (Q-001), with **NetBox as the source of
truth** (Q-002). The user chose both on 2026-09-25.

The application is a control plane between the two. It:
- reads DNS data from NetBox;
- renders it for PowerDNS;
- pushes it to the right PowerDNS servers;
- verifies the result;
- detects and reports drift;
- audits every step.

### Consequences

- DNS records are **authored in NetBox**, not in this app. Anything that edits
  records (people, IaC, automation) goes through NetBox. Whether the app has
  any record editor at all is Q-016. The proposed default is none.
- IaC for this app manages the app's own configuration, not DNS records (Q-041).
- The app is never in the DNS data path. If it's down, PowerDNS keeps serving
  (Q-027).
- The sync engine still goes behind an internal backend interface, so other DNS
  software can be added later without a rewrite (Q-039).
- Still open, and blocking M1:
  - how NetBox models DNS data (Q-006);
  - which PowerDNS versions and backends are in use (Q-008);
  - which topologies to support (Q-009);
  - how sync behaves (Q-010).

### Confirmation

Design reviews for M1–M4 check that every write path starts from NetBox data.

## More information

- Prior art: [ArnesSI/netbox-powerdns-sync](https://github.com/ArnesSI/netbox-powerdns-sync),
  a NetBox plugin last supported on NetBox 3.6 (Q-018).
