---
id: M05
title: Packaging
status: in-progress # planned | in-progress | done
started: 2026-10-07
closed:
---

# M05: Packaging

## Goal

A version tag publishes nbpdns as static linux amd64 and arm64 binaries,
with SHA-256 checksums, and as a multi-arch, non-root, distroless container
image with an SBOM, to GitLab. Every pipeline builds and tests the same
release without publishing it.

## Non-goals

- **No signing yet** (M17), and no SBOMs for the archives: only the image
  carries one.
- **No Helm chart, systemd unit, compose example or air-gap bundle:** those
  are M17 (Q-025's remainder).
- **No other platforms:** Linux only, amd64 and arm64.
- **No GitHub releases:** the mirror runs the checks on tags, but doesn't
  publish.
- **No image vulnerability scanning in CI** yet: that's M17's hardening.

## Phases

| Phase | Items |
|---|---|
| M5a Build | ITEM-0058 Pin GoReleaser, and build the archives and checksums |
| M5b Image | ITEM-0059 The container image with ko, the release tests, and the `release-check` CI jobs |
| M5c Publish | ITEM-0060 `make release` to GitLab on version tags, with `projctl`'s release-job rule and the GitLab release stage |
| M5d Docs and release | ITEM-0061 Release docs, and the CHANGELOG as 0.1.0 |

## Acceptance criteria

- [ ] ADR-0030, ADR-0031 and ADR-0032 are accepted. ADR-0015 and ADR-0016
  are superseded, Q-025 is answered, and REQ-045 exists.
- [ ] GoReleaser is pinned by SHA-256, and `make release-check` builds the
  archives for amd64 and arm64, with `checksums.txt`, and the image.
- [ ] The release tests pass. The binaries are static and of the right
  architecture, and report the build's version. The image runs as 65532,
  read-only, with no shell, and has its labels.
- [ ] The same commit builds byte-identical archives twice.
- [ ] `make release` refuses to run off a version tag, or without a CHANGELOG
  section, and publishes the image, with its SBOM, and a GitLab release with
  the archives.
- [ ] `projctl` allows only the tag-only `release` job, which tests show.
  Both forges' CI files pass `make project-lint`, and `release-check` runs
  on both.
- [ ] The docs pages above exist, and the CHANGELOG is 0.1.0.
- [ ] `/code-review high` has run. `/security-review` runs, since M05 adds
  publishing credentials to CI.
- [ ] The manual verification is recorded. The pipelines pass, and the user
  has merged through an MR with a merge commit, then tagged `v0.1.0`, whose
  pipeline published the release.

## Verification log

Append-only and dated. Record what was run and what was seen.

## Approved design

The plan approved on 2026-10-07, copied verbatim. Its headings are demoted two
levels to nest under this section; the text is unchanged. It's a snapshot.
Where it disagrees with an ADR or `CLAUDE.md`, they win.

### M05: Packaging

#### Context

M04 made nbpdns a service, but it's only built by `make build`, on the
developer's machine, for one platform. M05 makes releases: static linux
amd64 and arm64 binaries, with checksums, and a multi-arch container image,
built the same way every time. They're published to GitLab when you push a
version tag. The milestone ends with v0.1.0, the first release, which you
tag after the merge.

Decided with the user in the M05 design session, 2026-10-07:
- **Publishing:** GitLab, on version tags. The image goes to GitLab's
  container registry, and the archives and checksums go to a GitLab release,
  stored in the generic package registry. Every other pipeline builds and
  tests the release without publishing it. The Makefile takes the registry,
  the credentials and the GitLab URLs as variables, so only the CI file
  names GitLab.
- **Base image:** Debian's distroless static, non-root, pinned by digest.
- **Supply chain:** an SPDX SBOM is attached to the image now, and the
  archives get SHA-256 checksums. Signing waits for M17.
- **First release:** M05's last item turns the CHANGELOG's Unreleased
  section into 0.1.0. You tag `v0.1.0` on `main` after the merge, and its
  pipeline publishes it. Releases stay 0.x until v1.0 (M19).

#### Goal

A version tag publishes nbpdns as static linux amd64 and arm64 binaries,
with SHA-256 checksums, and as a multi-arch, non-root, distroless container
image with an SBOM, to GitLab. Every pipeline builds and tests the same
release without publishing it.

#### Non-goals

- **No signing yet** (M17), and no SBOMs for the archives: only the image
  carries one.
- **No Helm chart, systemd unit, compose example or air-gap bundle:** those
  are M17 (Q-025's remainder).
- **No other platforms:** Linux only, amd64 and arm64.
- **No GitHub releases:** the mirror runs the checks on tags, but doesn't
  publish.
- **No image vulnerability scanning in CI** yet: that's M17's hardening.

#### Decisions

##### ADR-0030: release with GoReleaser and ko, to GitLab on version tags

- **GoReleaser** (MIT), pinned as a release binary by SHA-256 in
  `tools/tools.mk` (ADR-0022), builds the binaries, archives and checksums.
  Its built-in ko integration builds the image: no Dockerfile, and no Docker
  CLI.
- **Binaries:**
  - `CGO_ENABLED=0`, `-trimpath`, `-ldflags=-s -w`, for linux/amd64 and
    linux/arm64.
  - Each file's time is the commit's, so that the same commit builds the
    same bytes.
  - The version comes from Go's build info, as it does now: a build at tag
    `v0.1.0` reports `v0.1.0`, so no `-X` flags are needed.
- **Archives:** `nbpdns_<version>_linux_<arch>.tar.gz`, holding the binary,
  `LICENSE`, `README.md` and `CHANGELOG.md`, with `checksums.txt`
  (SHA-256).
- **The image:**
  - `<registry image>:<version>`, `:<major>.<minor>` and `:latest`, as one
    multi-arch index for amd64 and arm64.
  - The OCI labels: source, version, revision, created, licenses, title and
    description.
  - ko's SPDX SBOM is attached to it in the registry.
  - The entrypoint is `nbpdns`, run as user 65532.
- **Publishing**, on a `vMAJOR.MINOR.PATCH` tag, through `make release`:
  - the image goes to the registry named by `RELEASE_IMAGE`, with
    `RELEASE_REGISTRY*` credentials;
  - the archives and checksums go to a GitLab release, through the generic
    package registry, using the CI job token, with release notes taken from
    the CHANGELOG's section for the version.
  - `make release` refuses to run unless HEAD is at such a tag and the
    CHANGELOG has a section for it.
- **Versioning:** semantic versions, 0.x until v1.0. The CHANGELOG's
  `[Unreleased]` section becomes the release's at tagging time.

##### ADR-0031: CI and runtime images, superseding ADR-0015

ADR-0031 restates ADR-0015. It keeps glibc Debian images for CI and for
container builds, and chooses the runtime image that ADR-0015 left open:
`gcr.io/distroless/static-debian13:nonroot`, pinned by digest, or the
debian12 variant if 13 isn't published. nbpdns is a static binary that
loads no libc, so a glibc runtime image would only add a library to patch;
static is the smallest Debian-based choice.

##### ADR-0032: staged CI with a release stage, superseding ADR-0016

ADR-0032 restates ADR-0016 and adds two things:
- a `build` job, `release-check`, which runs `make release-check` in every
  pipeline;
- a GitLab-only `release` stage, whose one job runs `make release`, only
  on version tags.

`projctl` allows exactly that exception. A job named `release` may run only
`make release`, and its only skip key may be one `rules:` entry, an `if`
that matches version tags. Every other job still runs only `make ci`'s
targets, with nothing that can skip it. GitLab's `workflow:` gains tag
pipelines.

##### Requirements

- REQ-045: "nbpdns is released, on version tags, as static linux amd64 and
  arm64 binaries with SHA-256 checksums, and as a multi-arch, non-root,
  distroless container image with an SBOM, built the same way every time."
- Q-025 moves to Answered, as Q-027 did. The binaries and the image are M05
  (ADR-0030, ADR-0031). The Helm chart, systemd unit, compose example and
  air-gap bundle are M17's.

#### Design

##### Tools and configuration

- `tools/tools.mk` pins GoReleaser v2, from its `checksums.txt`, as
  `goreleaser_Linux_x86_64.tar.gz` with the member `goreleaser`.
- `.goreleaser.yaml`, at the repository's root, covers:
  - `builds`, `archives`, `checksum` and `kos`;
  - `release`, with `gitlab_urls` from templates of environment variables
    (`api`, `download`, `use_job_token`, `use_package_registry`), disabled
    unless `GITLAB_TOKEN` is set;
  - `changelog: disable` (the notes come from the CHANGELOG);
  - `snapshot` names a pseudo-version.
- The base image is pinned by digest, as `kos.base_image`.

##### Make targets

| Target | Does |
|---|---|
| `release-check` | `goreleaser release --snapshot --clean`, which makes the archives and checksums in `dist/` and loads the image into the Docker daemon at `DOCKER_HOST` (`goreleaser.ko.local`). Then runs the release tests. Part of `make ci`. |
| `release` | Checks that HEAD is at a `v*.*.*` tag with a CHANGELOG section, writes a Docker config from `RELEASE_REGISTRY*` into a temporary `DOCKER_CONFIG`, extracts the release notes, and runs `goreleaser release --clean`. Not in `make ci`. |

`LAB_DOCKER_HOST` maps to `DOCKER_HOST` for `release-check` too, as it
does for the lab, so GitHub's container jobs keep working.

##### Release tests (`internal/release`, build tag `release`)

Plain Go, standard library only. They talk to the Docker Engine API over
`DOCKER_HOST`, a TCP address or a Unix socket.
- **Archives:**
  - Each one holds its files.
  - Each binary is a static ELF for its architecture: x86-64 or AArch64,
    with no interpreter.
  - `checksums.txt` matches every file.
  - The amd64 binary runs, and `nbpdns version -o json` reports the
    snapshot's version.
- **The image:**
  - Its config has user 65532, the `nbpdns` entrypoint, and the OCI labels,
    with the right version and revision.
  - A container from it, with a read-only root file system, runs
    `version -o json`, and runs `serve` with no server groups, which fails
    with the expected message.
  - It has no shell: running `sh` fails.

##### CI

- **GitLab:**
  - `workflow:` adds `if: $CI_COMMIT_TAG`.
  - The new `release-check` job, in stage `build`, has a Docker-in-Docker
    service, as `integration-test` does, but without the lab.
  - The new `release` stage and `release` job map `CI_REGISTRY_IMAGE`,
    `CI_REGISTRY`, `CI_REGISTRY_USER`, `CI_REGISTRY_PASSWORD`,
    `CI_JOB_TOKEN`, `CI_API_V4_URL` and `CI_SERVER_URL` to the `RELEASE_*`
    and `GITLAB_*` variables.
- **GitHub:** a `release-check` job with the same service and
  `LAB_DOCKER_HOST`, and no release job.
- **Memory:** the new job's Docker-in-Docker holds only the image build, a
  small fraction of the lab's 1.1 GiB.

##### Docs

| Section | Pages |
|---|---|
| How-to | "Install nbpdns": download an archive from a release, verify it against `checksums.txt`, install it; or pull the image. "Run nbpdns in a container": the config file and secrets mounted, `_FILE` secrets, the port, a read-only root file system, Kubernetes probes. |
| Reference | "Release artifacts": archive names and contents, the platforms, the image's tags, labels, user and entrypoint, and its SBOM. |
| Explanation | "How nbpdns is built and released": static binaries, reproducibility, the version from the build info, why distroless static, the SBOM, why signing comes later. |
| Contributing | "Make a release": move Unreleased to the version, tag, push, what the tag's pipeline does, and how to check it. |

The README gains an Install section linking to the how-to. The CHANGELOG
gets lines, then becomes 0.1.0 in the last item.

#### Items and phases

| Phase | Items |
|---|---|
| M5a Build | ITEM-0058 Pin GoReleaser, and build the archives and checksums |
| M5b Image | ITEM-0059 The container image with ko, the release tests, and the `release-check` CI jobs |
| M5c Publish | ITEM-0060 `make release` to GitLab on version tags, with `projctl`'s release-job rule and the GitLab release stage |
| M5d Docs and release | ITEM-0061 Release docs, and the CHANGELOG as 0.1.0 |

Once the plan is approved, one commit records the design:
- this plan, verbatim, under **Approved design** in M05's file;
- ADR-0030, ADR-0031 (superseding ADR-0015) and ADR-0032 (superseding
  ADR-0016);
- REQ-045, and Q-025 answered;
- the brief's dated answers;
- M05 marked in progress;
- the items.

Each item is then committed on `m05-packaging` as it's done.

#### Acceptance criteria

- [ ] ADR-0030, ADR-0031 and ADR-0032 are accepted. ADR-0015 and ADR-0016
  are superseded, Q-025 is answered, and REQ-045 exists.
- [ ] GoReleaser is pinned by SHA-256, and `make release-check` builds the
  archives for amd64 and arm64, with `checksums.txt`, and the image.
- [ ] The release tests pass. The binaries are static and of the right
  architecture, and report the build's version. The image runs as 65532,
  read-only, with no shell, and has its labels.
- [ ] The same commit builds byte-identical archives twice.
- [ ] `make release` refuses to run off a version tag, or without a CHANGELOG
  section, and publishes the image, with its SBOM, and a GitLab release with
  the archives.
- [ ] `projctl` allows only the tag-only `release` job, which tests show.
  Both forges' CI files pass `make project-lint`, and `release-check` runs
  on both.
- [ ] The docs pages above exist, and the CHANGELOG is 0.1.0.
- [ ] `/code-review high` has run. `/security-review` runs, since M05 adds
  publishing credentials to CI.
- [ ] The manual verification is recorded. The pipelines pass, and the user
  has merged through an MR with a merge commit, then tagged `v0.1.0`, whose
  pipeline published the release.

#### Verification

- `make check` on every commit, and `make release-check` and
  `make test-integration` locally.
- **Manual:**
  1. Run `make release-check` twice, and compare the two `checksums.txt`.
  2. A dry-run publish, before the merge:
     - tag a throwaway `v0.0.0-rc.1` locally, never pushed, and run
       `make release` against a temporary local registry container, with
       no `GITLAB_TOKEN`, so no GitLab release is made;
     - check that the manifest index has linux/amd64 and linux/arm64, that
       the SBOM is attached, and that the amd64 image runs and reports
       `v0.0.0-rc.1`.
  3. After your merge and `v0.1.0` tag:
     - the tag pipeline's `release` job passes;
     - the GitLab release has both archives and `checksums.txt`, and they
       verify;
     - the registry has the multi-arch image, and `docker run … version`
       reports `v0.1.0`.

#### Your side

- In GitLab, keep the container registry and the package registry on for
  the project, and protect `v*` tags, so that only maintainers can publish.
- After the merge, tag `v0.1.0` on `main` and push the tag. The tag's
  pipeline publishes it.
