---
title: Set a zone's drift policy
weight: 35
---

# Set a zone's drift policy

A zone's **drift policy** says what nbpdns does when PowerDNS doesn't serve
what NetBox says. This guide sets a default policy for a server group,
overrides it for single zones, and checks that `nbpdns drift` uses them.

## Before you start

You need nbpdns with at least one server group declared in its config file:
see [Connect nbpdns to PowerDNS](connect-nbpdns-to-powerdns.md).

## Choose a policy

| Policy | `nbpdns drift` | From M13 |
|---|---|---|
| `report` | Compares the zone and reports its drift. | The same. |
| `enforce` | Compares the zone and reports its drift, marked `enforce (from M13)`. | nbpdns also corrects the drift on the group's primary. |
| `ignore` | Doesn't compare the zone, and lists it as ignored. | nbpdns leaves the zone alone. |

A zone with no policy set is `report`.

> [!IMPORTANT]
> nbpdns writes nothing to PowerDNS until M13, so `enforce` reports drift
> exactly as `report` does. Set it now to mark the zones nbpdns should
> correct once it can.

## Set a group's default policy

Set `drift_policy` on the group in the config file. It applies to every zone
the group serves that `zone_policies` doesn't name:

```yaml
powerdns:
  groups:
    - name: site-a
      views: [_default_]
      drift_policy: enforce
      primary:
        url: https://pdns-a.example.com:8443
        api_key_file: /etc/nbpdns/pdns-site-a.key
```

## Set a policy for single zones

Add `zone_policies` to the group, a mapping from zone name to policy:

```yaml
powerdns:
  groups:
    - name: site-a
      views: [_default_]
      drift_policy: enforce
      zone_policies:
        legacy.example.com: ignore
        test.example.com: report
      primary:
        url: https://pdns-a.example.com:8443
        api_key_file: /etc/nbpdns/pdns-site-a.key
```

- Write each name as NetBox has it. The final dot is optional, and case
  doesn't matter.
- Give an internationalized name in its ASCII form, such as
  `xn--bcher-kva.example` for `bücher.example`.
- Policies are per group. To ignore a zone in every group that serves it, set
  its policy in each of them.

## Check the policies

1. Check that nbpdns reads the policies:

   ```shell
   nbpdns config show --config /etc/nbpdns/nbpdns.yaml | grep -E 'KEY|polic'
   ```

   ```text
   KEY                                          VALUE                                              SOURCE
   powerdns.groups.site-a.drift_policy          enforce                                            file /etc/nbpdns/nbpdns.yaml
   powerdns.groups.site-a.zone_policies         legacy.example.com=ignore,test.example.com=report  file /etc/nbpdns/nbpdns.yaml
   ```

   An unknown policy, a name that isn't in ASCII form, or a zone listed twice
   is an error that names the group and the field, such as:

   ```text
   nbpdns: powerdns.groups[0] (site-a) (from file /etc/nbpdns/nbpdns.yaml): drift_policy: "strict" isn't a drift policy; use enforce, report, ignore
   ```

2. Run the drift report:

   ```shell
   nbpdns drift --config /etc/nbpdns/nbpdns.yaml --group site-a
   ```

   - Zones with the policy `ignore` are listed under **Ignored zones, not
     compared**, and counted in the `IGNORED` column. They never count as
     drift.
   - The drift section gives each drifted zone's policy in its `POLICY`
     column.

3. Look for warnings at the end of the report, such as:

   ```text
   Warnings about the configuration or NetBox's zones:
   GROUP   WARNING
   site-a  zone_policies names test.example.com., which isn't in any of the group's NetBox views
   ```

   nbpdns also logs each warning, on standard error, and the JSON report
   lists them under `warnings`. This one means `zone_policies` names a zone that NetBox doesn't assign to the
   group. NetBox can change without nbpdns's config, so this is a warning,
   not an error. Check the name for a typo, or remove the entry if the zone
   is gone.

## See also

- [How nbpdns finds drift](../explanation/how-nbpdns-finds-drift.md)
  explains what each policy changes in the report.
- The [configuration reference](../reference/configuration.md#powerdnsgroups)
  lists `drift_policy` and `zone_policies` with every other field of a
  server group.
