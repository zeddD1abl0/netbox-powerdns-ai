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

## Answers, 2026-09-25 (after the first GitLab pipeline)

The first GitLab job was evicted when the node ran out of ephemeral storage.
Measurement showed that building the eight tools from source needed about
6.5 GB. The user then said:

> Why are we building the tooling from scratch? Do these tools provide
> binaries that could save us a significant amount of bandwidth if we used
> them?

> I'd also like the CI process to be split sometime. The whole concept of the
> CI pipeline is that it should consist of multiple stages, through linting,
> building, testing, etc. Having a single command is a bit odd. I don't mind if
> the Makefile itself has a default "Just do everything". But the CI pipelines
> should definitely have stages, especially when we eventually build in
> security scanning, point testing, SBOM regression, etc.

> Before you change things, please note that I have no problem switching to a
> Ubuntu base image, or a Debian base image, or similar. There's no constraints
> that require an Alpine image at this point in time. The same is true of the
> base image that we use for Docker builds. While I would prefer to keep the
> attack surface small, I would also prefer to make things simpler for the
> future. Building 5 different tooling sets on 3 different OSes just to get
> around a security issue is not a good plan. Far better to default to a Ubuntu
> image which uses libc, and deal with the security vulnerability another way.

| Question | Answer |
|---|---|
| Which platforms should the pinned tool binaries cover? | "Linux amd64 only for the time being. As the project matures, we'll add arm64. We may even consider Mac and Windows builds, but Linux would be the expected deployment for the moment." |

These are recorded in ADR-0014 (tool binaries), ADR-0015 (glibc images) and
ADR-0016 (CI stages), and as REQ-038 and REQ-039.

## Answers, 2026-09-27 (M01 design)

These were given in the M01 design session. They weren't quoted at the time;
this is the summary recorded under "Approved design" in
[M01](milestones/M01-netbox-read-path.md).

| Topic | Answer |
|---|---|
| Milestone size | Smaller milestones. Each merge should be a useful, logical step. M01 reads from the NetBox API; later milestones add PowerDNS, then the database (SQLite, then PostgreSQL). |
| Authentication order | Read-only, unauthenticated features come first. Authentication arrives before anything can change state from outside. |
| Merging | Through a GitLab merge request with a merge commit. GitHub is a push mirror. |
| Libraries | Established libraries instead of custom modules: Cobra for the command line and Viper for configuration. For logging, the user compared slog with zap and chose `log/slog`. |
| NetBox webhooks | Needed. Where they go was left to Claude; they got their own milestone, after the REST API. |
| Q-007: supported versions | NetBox 4.7 and 4.6, with the DNS plugin 1.7.x and 1.6.x. |
| CI | Integration tests against a real NetBox run in every pipeline, using Docker-in-Docker. |

The resulting decisions are recorded in ADR-0018 (merging), ADR-0019 (the
milestones), ADR-0020 (the NetBox client) and ADR-0021 (Cobra and Viper).

## Answers, 2026-09-29 (review before M01 implementation)

A review of the repository before implementation raised these questions. The
user's answers are quoted.

| Question | Answer |
|---|---|
| Is GitLab set to merge with a merge commit, with squash disabled? | "Merge commit with Squash disabled is done" |
| May nbpdns reach NetBox over plain HTTP, which sends the token unencrypted? | "Allow over HTTP. Not all NetBox deployments will be secure. Add a warning to the configuration item" |
| Should the normalized DNS model group records into RRsets, with the lowest TTL when a set's records disagree? | "RRSET with the lowest TTL sounds good to me" |
| May retries, timeouts and TLS be tested against a local test server, since a real NetBox can't be made to fail on demand? | "Yes, test against a fake server" |
| Should the hook tests live in their own module, `tools/hooktest`, with `jq` pinned as a release binary? | "Both of these seem fine to me" |

The user didn't object to the other steps proposed in the same review:
- ITEM-0025 fixes the prerequisites, which omit the C compiler that `-race`
  needs;
- ADR-0020 is written before ADR-0021, to keep the numbers in M01's design;
- lint and vet also check files with the `integration` build tag.

Runner capacity for the Docker-in-Docker job wasn't confirmed yet.

## Answers, 2026-10-06 (finishing M01)

A review of the work in progress proposed defaults for three details of the
`nbpdns netbox` commands (ITEM-0024). The user's answers are quoted.

| Question | Answer |
|---|---|
| When NetBox's data has problems that normalization works around, such as an RRset whose records disagree on TTL, should `nbpdns netbox records` report them and still succeed? | "Yes, just report the error, don't fail." |
| When a zone name exists in more than one view and no `--view` is given, should the command fail and list the views? | "Yes, throw an error regarding multiple view names" |
| Should inactive records be shown by default? | "Yes, inactive records should just be shown" |

The user then asked for M01 to be completed.

## Answers, 2026-10-06 (the first M01 pipeline)

The first pipeline with the integration job failed. The user said:

> The pipeline died because it's quite significant in memory footprint and
> the nodes running the jobs don't have much spare capacity at the moment. It
> appears it runs up two separate NetBox instances and tests against both. If
> that is the case, it needs to narrow down to just testing against 4.7, and
> we'll add other versions and support from there.

It was the case: the lab ran NetBox 4.7 and 4.6, each with its own
PostgreSQL server. nbpdns now supports and tests NetBox 4.7 only, with the
DNS plugin 1.7.x. More releases are added as the runners have room. This is
recorded in ADR-0023, which supersedes ADR-0020, and REQ-040 is narrowed to
match.

## Answers, 2026-10-06 (M02 design)

These were given in the M02 design session. Each was chosen from proposed
options; the label of the chosen option is quoted.

| Question | Answer |
|---|---|
| Q-053: Which PowerDNS Authoritative releases should M02 support and test? | "5.1 and 5.0". 5.1 is the current train; 4.9 reached end of life around September 2026. |
| How should nbpdns decide which server group serves each NetBox zone? | "By NetBox view": each group lists the views it serves, and a view may go to several groups. |
| How should nbpdns normalize every record type? | "miekg/dns v2" (`codeberg.org/miekg/dns`). |
| What should M02 deliver, given that the PowerDNS API has no TLS and no read-only key? | "Please break this out into a larger discussion. Why is there concern about the PowerDNS API?" |

The discussion that followed: a PowerDNS API key has no scopes and can't be
read-only, so it can change every zone, record, TSIG key and DNSSEC key on
its server; the API's webserver has no TLS of its own; so a key that leaks,
or is read off the network, lets someone rewrite DNS for every zone in the
group. The usual layers are a TLS proxy on the PowerDNS host, a client
certificate at the proxy, allowing only GET until nbpdns writes, and network
limits. Then:

| Question | Answer |
|---|---|
| Should nbpdns accept an `http://` URL for a PowerDNS API? | "Allow, with a warning", as for NetBox. |
| Q-022: Which access setup should the docs present as the reference? | "TLS proxy, all methods": TLS and an optional client certificate, with writes passing too, so it serves M12 unchanged. |
| Should the lab put that proxy in front of one group's primary? | "No, local test servers": TLS is tested against Go test servers, and the how-to is checked by hand. |
| Should each lab group have its secondary in M02? | "Primaries only for now": secondaries arrive with catalog zones, in M12. |

The user then approved M02's plan, which also answers Q-021 (the API,
directly), Q-039 (no backend interface until M03 needs one) and Q-043
(server groups in the config file). They're recorded in ADR-0024 and
ADR-0025, and as REQ-041 and REQ-042.

## Answers, 2026-10-06 (the first M02 pipeline)

The first pipeline with M02's lab failed for lack of memory on the runners
again. The user said:

> The pipeline has failed because of the amount of memory in use again. Can
> we concentrate on just PowerDNS 5.1 for the moment

nbpdns now supports and tests PowerDNS Authoritative 5.1 only, and the lab
runs one PowerDNS server, the primary of the server group `lab-a`. This is
recorded in ADR-0026, which supersedes ADR-0024, and REQ-041 is narrowed to
match.

