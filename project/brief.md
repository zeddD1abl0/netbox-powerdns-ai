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

## Answers, 2026-10-06 (M03 design)

These were given in the M03 design session. Each was chosen from proposed
options; the label of the chosen option is quoted.

| Question | Answer |
|---|---|
| Where should each zone's drift policy (ADR-0008) be set? | "nbpdns's config file": a default per server group, with overrides per zone name. |
| How should the report treat zones on a primary that NetBox doesn't assign to its group? | "List them, not as drift". |
| How should SOA records be compared, given that PowerDNS rewrites the serial? | "All but the serial". |
| Q-017: what scale should M03 be designed and tested for? | "Smaller for now": 1,000 zones and 100,000 records. |

The user then approved M03's plan, which also answers Q-027 for the command
line. The decisions are recorded in ADR-0027, and as REQ-043.


## Answers, 2026-10-07 (M04 design)

These were given in the M04 design session. Each was chosen from proposed
options; the label of the chosen option is quoted.

| Question | Answer |
|---|---|
| M04's stub bundles the running service with packaging (GoReleaser, the container image). Should packaging stay in M04? | "Split it out": packaging becomes M05, and the stubs after it shift up one number. |
| When should `nbpdns serve` report ready on its readiness endpoint? | "After the first refresh", whatever its outcome. |
| How detailed should the drift metrics be? | "Per group, plus drifted zones". |
| How should traces be exported over OTLP? | "OTLP over HTTP only". |

Reviewing the plan, the user then asked for two changes:

> If we're already requiring GRPC, I'd like to add OTLP/GRPC to the
> supported list. Also, a /status page should probably be included in this
> list, returning useful states in a human/json layout (probably default to
> human and use ?json=1).

The user approved the plan with both. The decisions are recorded in
ADR-0028 and ADR-0029, and as REQ-044.

## Answers, 2026-10-07 (M05 design)

These were given in the M05 design session. Each was chosen from proposed
options; the label of the chosen option is quoted.

| Question | Answer |
|---|---|
| Where should releases be published? | "GitLab, on version tags": the image to GitLab's container registry, the archives and checksums to a GitLab release. |
| Which runtime base image should the container use? | "distroless static". |
| Should releases be signed, and carry SBOMs, in M05? | "Image SBOM now, signing in M17". |
| Should M05 end with a first release? | "Yes: you tag v0.1.0 after merging". |

The user then approved M05's plan. The decisions are recorded in ADR-0030,
ADR-0031 (superseding ADR-0015) and ADR-0032 (superseding ADR-0016), and as
REQ-045, answering Q-025 for the binaries and the image.

## Answers, 2026-10-08 (M06 design)

These were given in the M06 design session. Where an answer was one of the
proposed options, its label is quoted; the others are quoted in full.

| Question | Answer |
|---|---|
| Q-041: what will IaC manage through nbpdns's API? | "nbpdns config + the ability to read only DNS records. It may be useful to have a Terraform query nbpdns for records, and then push to other Terraform Providers." |
| Which viewer should the binary serve at `/api/docs`? | "Provide a pros and cons of Swagger UI vs Scalar. I would lean towards Scalar only because it appears to be better for AI, and we'll probably look at some sort of MCP at a MUCH later milestone." After the comparison: "Scalar, locked down". |
| Which read-only resources should M06's API have? | "Plus service status": server groups, zones and their changes, and the service's status. |
| Should M06 fix the slow lab start on GitLab's runner (ITEM-0065)? | "Unfortunately the issue is to do with the hardware restrictions on the runner. This is a known issue that if the node is busy with other actions, the memory paging kicks in, slowing down the NetBox run-up. Avoid trying to fix this, as retrying the pipeline continues to succeed, and I do not have the resources to expand the capacity of the GitLab Runner currently." |
| Should M06's API also serve DNS records? | "Yes, in M06": each zone's RRsets as NetBox defines them, in nbpdns's normalized form. |

The user then approved M06's plan. The decisions are recorded in ADR-0033
and ADR-0034, and as REQ-046 and REQ-047, answering Q-041. ITEM-0065 is
closed as won't-fix.

## Answers, 2026-10-08 (M07 design)

These were given in the M07 design session. Each was chosen from proposed
options; the label of the chosen option is quoted. Before the questions,
the user suggested a fix for the integration job that the runner kept
killing:

> I think a fix for the integration test failing might be to make it run
> separately from the unit tests

| Question | Answer |
|---|---|
| Q-054 (the trigger part): what should a NetBox webhook make nbpdns refresh? | "Only the affected zones": the zones a burst names, after a quiet spell; a view's change refreshes everything; the scheduled full refresh stays. |
| Q-037: how far should traceability go in M07? | "Request ID and user": in the zone refresh's trace, logs and status; the rest of the chain, and NetBox's change IDs, with M13. |
| How should webhooks be tested, given the lab has no NetBox worker? | "Replay in CI, worker on demand": signed, recorded NetBox 4.7 payloads in CI; an optional worker profile in the lab for real end-to-end runs. |
| Who sets up the webhook and event rule in NetBox? | "You, from nbpdns's docs": nbpdns keeps its read-only token. |

The user then approved M07's plan. The decisions are recorded in ADR-0035,
and as REQ-048, answering Q-037 and Q-054's trigger part; its import part
is Q-057, for M15.

## Answers, 2026-10-09 (planning ahead, and M08 design)

After M07 closed, the user asked to plan several milestones ahead, with
GitLab as their view, while keeping every push, tag and merge:

> I am a bit of a controlling person around code commits because of the
> runner configuration.

They gave Claude's token the Planner role, with the `api` scope, rather
than Developer, and clarified REQ-026's scope:

> As much as I've said not to rely on Forge Features, that was primarily for
> the code-base, etc. The pipeline itself may not always run on GitLab
> runners, and indeed, currently runs on GitLab and GitHub. The code may not
> always be on x86_64 hardware. Currently we use GitLab for code tracking and
> tracing though, so it makes sense to use those features where useful.

On versions, after `v1.0.0-mNN` was tried and its tag deleted:

> I'll clean up the v1.0.0-m7 stuff, and we'll go with v0.NN.0 until we
> think it's proper, and then v1.NN.0 from there

The user accepted ADR-0037, which records all of this, and its changes to
CLAUDE.md.

These were then given in the M08 design session, each chosen from proposed
options; the label of the chosen option is quoted.

| Question | Answer |
|---|---|
| How big should M08 be? | "As stubbed": the database, migrations and the lock; drift history; the audit core; runtime settings with `managed_by`; and secrets at rest. |
| Which query layer? | "sqlc", on goose's forward-only migrations and modernc.org/sqlite. |
| Q-035: is the proposed audit default right, from M08? | "Yes, chained from the start": hash-chained from the first event, with a configurable retention. |
| Q-036: which compliance frameworks? | "Essential Eight / ISM", "ISO 27001", and "SOC 2 or PCI DSS". |
| What writes settings and groups before M10? | "A local CLI": `nbpdns settings` and `nbpdns groups`, each change audited with the OS user as its actor. |
| When does a running `serve` use a changed setting? | "Within seconds". |
| What should drift history keep? | "Also every refresh's changes": each zone's changes of state, and its RRset changes. |
| Where does the database live? | "Required, with a default path": `sqlite:///var/lib/nbpdns/nbpdns.db`. |

The user then approved M08's plan, recorded with its ADRs, ADR-0038 to
ADR-0042, proposed until M08 starts; REQ-049 to REQ-053; and Q-012's
settings part, Q-023, Q-024, Q-026's migrations part, Q-035 and Q-036
answered. Q-012's metadata part is Q-059, for M13, and Q-026's backup and
export part is Q-058, for M17.

### M09's roadmap, 2026-10-09

The user chose roadmap entries for M09 to M12 on this branch, full designs
waiting until each is next. For M09:

| Question | Answer |
|---|---|
| Q-060: which PostgreSQL releases? | "18, more as runners allow". |
| Q-061: can a SQLite install move to PostgreSQL? | "Not supported": a move starts with an empty database. |
| Q-062: how is the leader chosen? | "A PostgreSQL advisory lock". |

### M10's roadmap, 2026-10-09

| Question | Answer |
|---|---|
| When should local users, MFA and sessions arrive, given they need a sign-in page? | "With M12's UI": M10 is tokens, service accounts and the break-glass token. |
| Q-029, with the Essential Eight: which MFA methods? | "WebAuthn only". |
| Once the API needs a token, what stays open? | "Only /livez and /readyz". |
| Q-040: rate limits? | "Per token and per IP". |
