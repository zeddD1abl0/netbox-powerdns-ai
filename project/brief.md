# Project brief

This file is append-only. The original brief stays exactly as written. Later
answers and clarifications are added below it, dated, and never edit earlier
text. Numbered requirements derived from it live in
[`requirements.md`](requirements.md).

## Original brief (2026-09-25)

> I'm starting a project that will be fully developed by Claude AI. I want to do
> this in the best possible way and I want to include the best setup to make
> this a repeatable and expandable project.
>
> What guidelines would you set up?
> What is the best way to track the work in the project?
> What information do you require to get started?
>
> The project will be written in Goland and should be able to run as a single
> binary or a container/pod, depending on deployment. Configuration should be
> web-based, as well as environmental and possibly config file. There should be
> user management and single sign-on.
>
> Because this will be used to manage multiple DNS servers in various complex
> setups, it needs to support logging to a SIEM and/or other external solution.
> It must support auditing and traceability. There should be an API with clearly
> defined metrics, and the capability to change settings and to possibly be
> extended in the future. Ideally, this would allow the config to be driven by
> IaC (Terraform/OpenTofu/Ansible).
>
> Documentation needs to be clear and it needs to cover all the API endpoints,
> etc. It's probably a good idea to follow some sort of documented standard for
> APIs. Documentation itself should be part of a standard process too, and
> should be readable without embellishment, though it should utilize the
> features in the chosen documentation platform.
>
> Before we implement, we should go over all the missing things in the above
> project brief.

## Answers, 2026-09-25

These were given during the M0 planning session.

| Question | Answer |
|---|---|
| Which DNS server software must the first release manage? | PowerDNS Authoritative. |
| What is NetBox's role relative to this application? | NetBox is the source of truth. This app renders NetBox's DNS data, pushes it to PowerDNS, detects drift and audits every step. |
| Where should the project's work be tracked? | In-repo Markdown only. |
| Which documentation platform? | Not chosen yet. Constraint given instead: "Don't consider that this project is GitLab only. The repo is currently published to a GitLab repo, but in the future, a branch of it could be in GitHub or similar. Ideally, whatever we do in this project will be as self-contained as possible." |

The planning session also produced these, recorded as ADRs:
- [ADR-0002](../docs/adr/0002-track-work-in-repo.md): in-repo tracking;
- [ADR-0003](../docs/adr/0003-self-contained-forge-neutral-toolchain.md): the self-contained rule;
- [ADR-0004](../docs/adr/0004-v1-scope-netbox-source-of-truth-powerdns-auth.md): v1 scope.

## Answers, 2026-09-25 (M0b discovery)

These were given in the M0b discovery session. Where the user typed their own
answer rather than choosing an option, it's quoted verbatim.

| Question | Answer |
|---|---|
| Q-006: Where does NetBox hold the DNS data? | The NetBox DNS plugin. |
| Q-008: Which PowerDNS Authoritative backends are in use? | "Ideally, we'll use the PowerDNS API rather than direct interaction with zone information." |
| Q-009: Which topologies must v1 handle? | Primary → secondaries, and independent sites/clusters. |
| Q-010: What happens when PowerDNS differs from NetBox? | A drift policy per zone. |
| Q-005a: Product and binary name? | `nbpdns` (the short form). |
| Q-005b: Go module path? | A GitHub path, then `github.com/<owner>/netbox-powerdns-ai`, keeping the current repo name. The owner wasn't given. |
| Q-005c: License? | Apache-2.0. |
| Q-019/Q-020: Persistence and high availability? | PostgreSQL + SQLite. |
| Q-047: Who commits? | Claude, per item, on a branch. |
| Q-048: What can Claude test against? | The container lab only. |
| Q-052: How do secondaries learn about new or deleted zones? | Catalog zones. |

The resulting decisions are recorded in ADR-0005 to ADR-0010.

## Answers, 2026-09-25 (M0b discovery, continued)

| Question | Answer |
|---|---|
| Q-055: Which GitHub owner goes in the module path? | "The GitHub repo has been added as a remote." The remote is `https://github.com/zeddD1abl0/netbox-powerdns-ai.git`. |
| Q-044: Which documentation platform? | Hugo. |
| API style guide (recorded as Q-056)? | The Zalando guidelines. |
| Zalando rule 115 forbids versions in URL paths. Follow it, or deviate with `/api/v1`? | Follow Zalando. |
| Accept the proposed defaults for Q-045 (audiences), Q-046 (hosting), Q-049 (CI runners)? | Accepted. |
| Q-018: Why build rather than extend an existing NetBox plugin? | Audit, SIEM and traceability; topologies and change safety; independent of NetBox upgrades. |

### Why build rather than extend (Q-018)

Prior art exists. ArnesSI/netbox-powerdns-sync is a NetBox plugin, last
supported on NetBox 3.6. nbpdns is built as a separate service because:

- **It's independent of NetBox upgrades.** It runs as its own service and talks
  to NetBox through its API, so a NetBox upgrade can't break DNS sync. The
  existing plugin stalled at NetBox 3.6.
- **Audit, SIEM and traceability.** There's an end-to-end trail from a change
  in NetBox to every PowerDNS write, and it's exported to a SIEM.
- **Topologies and change safety.** Server groups, catalog zones, a drift
  policy per zone, change limits, and verification after every apply.
