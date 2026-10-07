---
title: Install nbpdns
weight: 5
---

# Install nbpdns

Each nbpdns release is a static binary for Linux on amd64 and arm64, and a
container image for both. This guide downloads a release's binary, checks
it against the release's checksums, and installs it, or pulls the image.

[Release artifacts](../reference/release-artifacts.md) lists everything a
release holds.

## Before you start

For the binary, you need a Linux host on amd64 (`x86_64`) or arm64
(`aarch64`), as `uname -m` reports, with `sha256sum` and `tar`. The binary
is static: it needs no C library, so it runs on any Linux distribution.

For the image, you need Docker, Kubernetes, or another container runtime.

## Install the binary

1. Open the release on the project's GitLab, under **Deploy > Releases**.
   Its notes are the version's section of the
   [CHANGELOG](../../CHANGELOG.md).

2. Download the archive for your host's architecture, and
   `checksums.txt`, into an empty directory. For 0.1.0 on amd64, they're:

   ```text
   nbpdns_0.1.0_linux_amd64.tar.gz
   checksums.txt
   ```

   If the project isn't public, download them while signed in to GitLab.

3. Check the archive against `checksums.txt`:

   ```shell
   sha256sum --ignore-missing --check checksums.txt
   ```

   ```text
   nbpdns_0.1.0_linux_amd64.tar.gz: OK
   ```

   Anything but `OK` means the archive isn't the file the release
   published: don't install it.

   > [!NOTE]
   > Releases aren't signed until M17. A checksum shows that the archive is
   > the file `checksums.txt` lists, not who published it, so download both
   > from the project's GitLab, over HTTPS.

4. Unpack the binary, and install it:

   ```shell
   tar -xzf nbpdns_0.1.0_linux_amd64.tar.gz nbpdns
   sudo install -m 0755 nbpdns /usr/local/bin/nbpdns
   ```

   The archive also holds `LICENSE`, `README.md`, and `CHANGELOG.md`.

5. Check the version:

   ```shell
   nbpdns version
   ```

   ```text
   FIELD        VALUE
   version      v0.1.0
   commit       …
   commit_time  …
   modified     false
   go_version   go1.27.1
   platform     linux/amd64
   ```

## Pull the image

The image is in the project's container registry, under **Deploy >
Container registry** in GitLab, which shows its name, such as
`registry.example.com/group/netbox-powerdns-ai`.

1. If the registry isn't public, sign in to it with a GitLab token that can
   read it:

   ```shell
   docker login registry.example.com
   ```

2. Pull the release by its version:

   ```shell
   docker pull registry.example.com/group/netbox-powerdns-ai:0.1.0
   ```

   One tag covers amd64 and arm64: the runtime pulls the image for its
   host. The image is also tagged `0.1`, which moves to each 0.1 patch
   release, and `latest`, which moves to each release. Use the full
   version, or the image's digest, wherever you deploy it.

3. Check the version:

   ```shell
   docker run --rm registry.example.com/group/netbox-powerdns-ai:0.1.0 version
   ```

   It reports `v0.1.0`, as the binary does.

## Next steps

- [Configure nbpdns](configure-nbpdns.md).
- [Run nbpdns in a container](run-nbpdns-in-a-container.md).
