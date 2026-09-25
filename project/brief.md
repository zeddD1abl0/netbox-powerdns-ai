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
