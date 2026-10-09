---
title: Make a release
weight: 20
---

# Make a release

A release is a version tag on `main`, and the tag's GitLab pipeline
publishes it
([ADR-0030](../adr/0030-release-with-goreleaser-and-ko-to-gitlab-on-versio.md)).
This page prepares the CHANGELOG, tags the release, and checks what the
pipeline published. [How nbpdns is built and
released](../explanation/how-nbpdns-is-built-and-released.md) explains the
build.

## Before you start

You need:

- to be a maintainer of the GitLab project;
- the project's container registry and package registry turned on, under
  **Settings > General > Visibility, project features, permissions**;
- `v*` tags protected, under **Settings > Repository > Protected tags**,
  so that only maintainers can make a tag that publishes;
- in **Settings > Packages and registries**, the image's tags `latest` and
  versions, such as `0.1` and `0.1.0`, protected, so that only maintainers
  can push them, and duplicate generic packages refused. Every pipeline's
  jobs have the registry's credentials and a job token, so without these
  any developer's branch could push over a release's image or files;
- the changes to release merged into `main`, and `main`'s pipeline passing.

## Choose the version

Versions are [semantic](https://semver.org/), and each milestone is a
release
([ADR-0037](../adr/0037-plan-milestones-ahead-with-gitlab-milestones-and-i.md)):

- a milestone's merge is the minor version of its number, without the
  leading zero: `0.8.0` for M08, `0.10.0` for M10;
- a release with fixes only, between milestones, is a patch version, such as
  `0.8.1`;
- the versions stay 0.x until the project is judged stable and secure, and
  are `1.NN.0` from the milestone after that.

The releases are internal: nothing is meant for use before 1.x.

The tag is the version with a `v`: `v0.8.0`. A tag with anything more, such
as `v0.8.0-rc.1`, publishes nothing.

Release from `main`, in version order. Each release moves the image's
`latest` tag to itself, so a release of an older version, such as 0.7.1
after 0.8.0, would move `latest` back. Releases of older versions aren't
supported yet.

## Prepare the changelog

Do this on the branch whose merge request makes the release, so that
it's reviewed with the rest: for a milestone, when it closes.

1. In `CHANGELOG.md`, rename `## [Unreleased]` to the version and today's
   date, and add an empty `## [Unreleased]` before it:

   ```markdown
   ## [Unreleased]

   ## [0.8.0] - 2026-11-02

   ### Added
   ```

2. Check the release notes that `make release` publishes:

   ```shell
   go run ./internal/cmd/releasenotes -tag v0.8.0 CHANGELOG.md
   ```

   It prints the version's section, without its heading. It fails if the
   section is missing or empty.

3. Commit, and merge the branch as usual.

## Tag the release

1. Tag `main`'s merge commit, and push the tag to GitLab:

   ```shell
   git switch main
   git pull
   git tag -a v0.8.0 -m "nbpdns 0.8.0"
   git push origin v0.8.0
   ```

2. Watch the tag's pipeline. It runs every check, `release-check`
   included, then the `release` stage, whose one job runs `make release`:

   - it refuses to run unless the commit has exactly one `v` tag, the
     version's, and the CHANGELOG has its section;
   - it pushes the image, tagged `0.8.0`, `0.8`, and `latest`, with its
     SBOMs, to the project's container registry;
   - it makes the GitLab release, named after the tag, with the
     version's section of the CHANGELOG as its notes, and the archives
     and `checksums.txt` in the package registry.

If the project is mirrored to GitHub, the mirror's workflow runs the same
checks on the tag, and publishes nothing.

## Check the release

1. Open the release, under **Deploy > Releases**. Its notes are the
   version's section of the CHANGELOG, and it has both archives and
   `checksums.txt`. Download them, and check them:

   ```shell
   sha256sum --check checksums.txt
   ```

   ```text
   nbpdns_0.8.0_linux_amd64.tar.gz: OK
   nbpdns_0.8.0_linux_arm64.tar.gz: OK
   ```

2. Check that the image covers both platforms:

   ```shell
   docker buildx imagetools inspect registry.example.com/group/netbox-powerdns-ai:0.8.0
   ```

   It lists `linux/amd64` and `linux/arm64/v8` under **Manifests**.

3. Check the version it reports:

   ```shell
   docker run --rm registry.example.com/group/netbox-powerdns-ai:0.8.0 version
   ```

   It reports `v0.8.0`, and `modified false`.

## If the release job fails

- **It refused to run:** nothing was published. Fix the cause on a branch,
  and merge it. Then delete the tag, from GitLab and from your clone, and
  tag the new merge commit.
- **It failed while publishing:** read the job's log to see what it
  published, then retry the job. The build is reproducible, so a retry
  builds the same image and archives, and pushing the same image again
  changes nothing.
- **The release's notes are wrong or empty:** edit them in GitLab, under
  **Deploy > Releases**, and paste the output of `releasenotes`, as in
  [Prepare the changelog](#prepare-the-changelog). The notes aren't part
  of what was built, so changing them changes nothing else.
- **Something else was published and is wrong:** never move or delete a
  published tag. Fix it on a branch, and release the next patch version.
