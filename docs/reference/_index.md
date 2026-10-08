---
title: Reference
weight: 30
---

# Reference

Complete, accurate facts to look up rather than read through.

Most reference pages are **generated from the code** and must never be edited
by hand:

| Page | Generated from | Arrives in |
|---|---|---|
| [Configuration](configuration.md) | the config registry | M01 |
| [Command line](command-line.md) | the command-line definitions | M01 |
| [Supported versions](supported-versions.md) | the supported NetBox and PowerDNS releases | M01, M02 |
| [Metrics](metrics.md) | the metric declarations | M04 |
| [API](api.md) | `api/openapi.yaml` | M06 |
| Audit events | the audit event registry | M08 |
| Permissions | the permission registry | M11 |

The others are written by hand:

- [Service endpoints](service-endpoints.md): what `nbpdns serve` answers, and
  the fields of its status page. A test checks the page against the code.
- [Release artifacts](release-artifacts.md): what each release publishes:
  the archives, their checksums, and the container image, with its tags,
  settings, labels, and SBOMs.
