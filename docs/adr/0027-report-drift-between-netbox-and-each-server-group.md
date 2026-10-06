---
title: "0027: Report drift between NetBox and each server group's primary"
status: accepted
date: 2026-10-06
decision-makers: [jordan]
requirements: [REQ-024, REQ-031, REQ-043]
questions: [Q-017, Q-027]
supersedes:
---

# 0027: Report drift between NetBox and each server group's primary

## Context and problem statement

M01 reads NetBox's DNS data, and M02 reads each server group's primary, both
into one normalized RRset model (ADR-0023, ADR-0025). NetBox's zones are
assigned to groups by view (ADR-0026). M03 compares the two and reports the
differences, without writing anything. That needs answers to:
- what counts as drift, and what doesn't: records that aren't served, the
  SOA serial that PowerDNS rewrites itself, zones that only PowerDNS has;
- where each zone's drift policy is set, which ADR-0008 left to this
  milestone;
- what happens when NetBox or a primary can't be read (Q-027);
- what scale the comparison must handle (Q-017);
- how scripts can tell drift from failure.

## Decision drivers

- No false drift: the same data, written differently or not served, isn't
  drift (REQ-024, ADR-0025).
- NetBox is the source of truth, but zones that only PowerDNS has must never
  look like something to delete before brownfield import (M14) decides.
- A report that couldn't read everything mustn't look complete.
- Scripts and CI can act on the result through exit codes alone.
- Keep the comparison testable without I/O, so every case is a table test.

## Considered options

1. **Where the policy is set:** a NetBox custom field on the zone; nbpdns's
   config file; a NetBox tag.
2. **Zones only on a primary:** drift; listed but not drift; left out.
3. **The SOA:** compared on every field; on every field but the serial; not
   compared.
4. **Records not served:** compared like the rest; compared only where one
   side serves them.
5. **A primary that can't be read:** the run fails at once; that group is
   marked failed and the others are compared.

## Decision outcome

The user chose the policy's place, the treatment of zones only on a primary,
the SOA rule and the scale target on 2026-10-06. The rest follows from the
drivers.

**What's compared: what each side serves.** NetBox's active records are
compared with the primary's records that aren't disabled. A record that
neither side serves, inactive in NetBox and disabled in PowerDNS, isn't
compared.

**Zones**, for each group:
- An active NetBox zone in one of the group's views is expected on the
  primary. If it's missing there, the zone's state is `missing`, which is
  drift.
- A NetBox zone that isn't active, such as one that's parked, is expected
  absent. If the primary serves it, its state is `inactive_in_netbox`, which
  is drift.
- A zone on the primary that NetBox doesn't assign to the group is
  `unmanaged`. It's listed, and isn't drift. Brownfield import (M14) adopts
  such zones.
- A zone on both sides is `in_sync`, or `drift` with its RRset changes.

**RRsets** are keyed by owner name and type:
- only in NetBox: `missing`;
- only on the primary: `extra`;
- in both: `changed` if the values or the TTL differ. Each side's TTL and
  values are given.

**The SOA** is compared on every field but the serial, which PowerDNS
rewrites itself when its API changes a zone (SOA-EDIT-API). Each side's
serial is shown.

**Problems** that normalization found, on either side, are listed with the
report, and aren't drift.

**Policies are set in nbpdns's config file** (ADR-0008's open point):
- each server group has a `drift_policy`, `report` if unset, and
  `zone_policies`, a map from zone name to policy, which override it;
- `ignore` skips the zone, which is listed as ignored;
- `report` and `enforce` are compared and reported alike, since nothing is
  written until M12; the report marks `enforce` zones as acting from M12;
- a policy for a zone the group doesn't serve is a warning, not an error,
  since NetBox can change without the config.

**Failures (Q-027, for the command line):**
- If NetBox can't be read, nothing can be compared, and the command fails.
- If a group's primary can't be read, that group is marked failed, with the
  reason, and the other groups are still compared. The run fails, since its
  report is incomplete.
- Keeping the last-known state and alerting, Q-027's default for the
  service, arrives with the service (M04) and the database (M07).

**Exit codes:**
- 0: everything was compared, with no drift;
- 3: everything was compared, and there's drift;
- 1: something couldn't be read, which wins over drift;
- 2: a usage error, as for every command.

**Q-039:** the comparison is a pure function over the shared model. Reading
goes through two small interfaces that `internal/drift` defines: one for
NetBox, one for a group's primary. They're the first backend interfaces,
shaped by their first consumer.

**Q-017:** nbpdns is designed and tested for 1,000 zones and 100,000 records
per run (REQ-043), with time and memory measured. Larger targets wait for a
deployment that needs them.

### Consequences

- Good: a report says plainly whether PowerDNS serves what NetBox says, with
  no drift from the SOA serial, records nobody serves, or writing styles.
- Good: existing zones on a primary are visible without being counted as
  drift or implied for deletion.
- Good: a failed group is reported without hiding the others, and the exit
  code still says the run is incomplete.
- Bad: a zone's policy lives in nbpdns's config, not beside the zone in
  NetBox, so NetBox users can't see or change it there, and each exception
  is a config change.
- Bad: a record that NetBox has inactive and PowerDNS has disabled, with
  different values, isn't reported, since neither serves it.
- Bad: the scale target is modest; larger deployments need it raised and
  measured again.

### Confirmation

- Table tests cover every case above, with no I/O.
- A benchmark compares 1,000 zones and 100,000 records in under a second.
- Integration tests read a drift fixture with one of each case from the lab's
  NetBox and lab-a's primary, and check the exit codes, including a group
  whose primary can't be read.

## Pros and cons of the options

### A NetBox custom field, or a tag

- Good: the policy sits beside the zone, in the source of truth, and the
  read-only token already receives custom fields and tags.
- Bad: a NetBox admin must create the field, and a tag is free text that can
  contradict itself. The user chose the config file.

### Counting zones only on a primary as drift, or leaving them out

- Counting them: strict, but every existing zone would fail every report
  until M14 adopts it.
- Leaving them out: quiet, but a zone created on a primary by hand would go
  unseen.

### Comparing the SOA serial, or not comparing the SOA

- The serial: every zone would always show drift.
- No SOA: a wrong contact or timer would go unseen.

### Failing the whole run when one primary can't be read

- Good: simplest.
- Bad: one unreachable site hides the drift of every other.

## More information

- ADR-0008 (the drift policies), ADR-0026 (server groups and assignment by
  view), ADR-0023 and ADR-0025 (the model).
- Recorded in M03's design, 2026-10-06. Implemented by ITEM-0042 to
  ITEM-0046.
