# Pinned tool release binaries (ADR-0014), included by the Makefile.
#
# Each tool has a repository, a version, and per platform an asset name and
# the SHA-256 that the download must match. The URL is
#   https://github.com/<REPO>/releases/download/v<VERSION>/<ASSET>
# Only linux-amd64 is pinned for now. To add a platform, add an _ASSET_ and a
# _SHA256_ line for it to every tool.
#
# `make tools-update` rewrites the _VERSION and _SHA256_ lines from each
# tool's latest release (tools/update.sh); _CHECKSUMS names that release's
# checksums file. MEMBER is the binary's path inside the archive.

GOLANGCI_LINT_REPO                  := golangci/golangci-lint
GOLANGCI_LINT_VERSION               := 2.14.0
GOLANGCI_LINT_CHECKSUMS             = golangci-lint-$(GOLANGCI_LINT_VERSION)-checksums.txt
GOLANGCI_LINT_ASSET_linux-amd64     = golangci-lint-$(GOLANGCI_LINT_VERSION)-linux-amd64.tar.gz
GOLANGCI_LINT_SHA256_linux-amd64    := ab90aeb7b066f92a33415b638a50fe5344bbb75a0d32ad30cc248d88f81032ab
GOLANGCI_LINT_MEMBER_linux-amd64    = golangci-lint-$(GOLANGCI_LINT_VERSION)-linux-amd64/golangci-lint

HUGO_REPO                           := gohugoio/hugo
HUGO_VERSION                        := 0.166.0
HUGO_CHECKSUMS                      = hugo_$(HUGO_VERSION)_checksums.txt
HUGO_ASSET_linux-amd64              = hugo_$(HUGO_VERSION)_linux-amd64.tar.gz
HUGO_SHA256_linux-amd64             := 45228f5a52eb118b0ca168068f01d7df0447314a24056f1d29667ed9fc368308
HUGO_MEMBER_linux-amd64             = hugo

VALE_REPO                           := vale-cli/vale
VALE_VERSION                        := 3.22.0
VALE_CHECKSUMS                      = vale_$(VALE_VERSION)_checksums.txt
VALE_ASSET_linux-amd64              = vale_$(VALE_VERSION)_Linux_64-bit.tar.gz
VALE_SHA256_linux-amd64             := 52f5cd0314a1b7384cac6aa102a68193977312f6ba9c9f3ae001b5deec8e3a10
VALE_MEMBER_linux-amd64             = vale

VACUUM_REPO                         := daveshanley/vacuum
VACUUM_VERSION                      := 0.30.6
VACUUM_CHECKSUMS                    = checksums.txt
VACUUM_ASSET_linux-amd64            = vacuum_$(VACUUM_VERSION)_linux_x86_64.tar.gz
VACUUM_SHA256_linux-amd64           := 179f038a71cd88721738d6c739dd07fbeebc905b48301935f2416e66aead95c6
VACUUM_MEMBER_linux-amd64           = vacuum

GITLEAKS_REPO                       := gitleaks/gitleaks
GITLEAKS_VERSION                    := 8.30.1
GITLEAKS_CHECKSUMS                  = gitleaks_$(GITLEAKS_VERSION)_checksums.txt
GITLEAKS_ASSET_linux-amd64          = gitleaks_$(GITLEAKS_VERSION)_linux_x64.tar.gz
GITLEAKS_SHA256_linux-amd64         := 551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb
GITLEAKS_MEMBER_linux-amd64         = gitleaks

HTMLTEST_REPO                       := wjdp/htmltest
HTMLTEST_VERSION                    := 0.17.0
HTMLTEST_CHECKSUMS                  = htmltest_$(HTMLTEST_VERSION)_checksums.txt
HTMLTEST_ASSET_linux-amd64          = htmltest_$(HTMLTEST_VERSION)_linux_amd64.tar.gz
HTMLTEST_SHA256_linux-amd64         := 775c597ee74899d6002cd2d93076f897f4ba68686bceabe2e5d72e84c57bc0fb
HTMLTEST_MEMBER_linux-amd64         = htmltest

# Tools pinned as release binaries, as <VARIABLE PREFIX>:<binary name>.
BINARY_TOOLS := GOLANGCI_LINT:golangci-lint HUGO:hugo VALE:vale VACUUM:vacuum GITLEAKS:gitleaks HTMLTEST:htmltest
