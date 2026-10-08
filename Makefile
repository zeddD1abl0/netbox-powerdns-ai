# The single entry point for building, checking and tracking nbpdns.
# CI jobs run these targets and nothing else (ADR-0032). Run `make` for the list.

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help
MAKEFLAGS += --no-print-directory

ROOT := $(CURDIR)
# A comma, which $(call …) would otherwise take as an argument separator.
comma := ,

# The image CI runs in, pinned by digest. `make project-lint` checks that both
# forges' CI files use exactly this image (ADR-0032).
CI_IMAGE := golang:1.27.1@sha256:3680233e3204827fbdc66088528ae6d4b3d034f51d03a99d454f6de034888244

##@ Tools

# Release binaries are pinned per platform in tools/tools.mk and fetched once
# into .cache/tools/<platform>/ (ADR-0022). Each tool's variable, such as
# $(HUGO), is the path to its binary.
include tools/tools.mk
PLATFORM := $(shell go env GOOS)-$(shell go env GOARCH)
TOOLS_DIR := $(ROOT)/.cache/tools/$(PLATFORM)

# $(call binary_tool,PREFIX,NAME) defines $(PREFIX) and the rule that fetches it.
# A binary is fetched and verified again whenever tools.mk changes, so a new
# pin never leaves an old binary in use.
define binary_tool
$(1) := $$(TOOLS_DIR)/$(2)-$$($(1)_VERSION)
$$($(1)): tools/tools.mk
	@test -n "$$($(1)_SHA256_$$(PLATFORM))" || { echo "no pinned $(2) for $$(PLATFORM); run the tools in the CI image with 'make shell'"; exit 1; }
	@tools/fetch.sh "https://github.com/$$($(1)_REPO)/releases/download/$$(or $$($(1)_TAG),v$$($(1)_VERSION))/$$($(1)_ASSET_$$(PLATFORM))" \
		"$$($(1)_SHA256_$$(PLATFORM))" "$$($(1)_MEMBER_$$(PLATFORM))" "$$@"
endef
$(foreach t,$(BINARY_TOOLS),$(eval $(call binary_tool,$(word 1,$(subst :, ,$(t))),$(word 2,$(subst :, ,$(t))))))
BINARIES := $(foreach t,$(BINARY_TOOLS),$($(word 1,$(subst :, ,$(t)))))

# govulncheck and goimports publish no binaries, so they're built from source:
# $(call tool,NAME) runs the tool pinned in tools/NAME/go.mod.
tool = go tool -modfile=$(ROOT)/tools/$(1)/go.mod $(1)

.PHONY: tools
tools: $(BINARIES) ## Fetch every pinned tool binary into .cache/tools

.PHONY: tools-update
tools-update: ## Move every tool to its latest release (binaries, source-built tools, projctl's dependencies)
	tools/update.sh
	@for t in govulncheck goimports oapi-codegen; do \
		pkg=$$(awk '/^tool /{print $$2}' tools/$$t/go.mod); \
		echo "$$t: $$pkg@latest"; \
		go -C tools/$$t get -tool $$pkg@latest && go -C tools/$$t mod tidy; \
	done
	go -C tools/projctl get -u ./... && go -C tools/projctl mod tidy

.PHONY: tools-audit
tools-audit: ## Check every pinned hash against its release's published checksums file (changes nothing)
	tools/update.sh --check

.PHONY: shell
# The image's digest names a multi-arch index, and only linux-amd64 tools are
# pinned, so the shell always runs the amd64 variant (emulated on arm64 hosts).
shell: ## Open a shell in the CI image with the repository mounted (for macOS, Windows and arm64 hosts)
	docker run --rm -it --platform linux/amd64 --user "$$(id -u):$$(id -g)" -v "$(ROOT)":/src -w /src \
		-e HOME=/tmp -e GOTOOLCHAIN=local -e GOFLAGS=-modcacherw \
		-e GOMODCACHE=/src/.cache/gomod -e GOCACHE=/src/.cache/gobuild \
		$(CI_IMAGE) bash

##@ Pipeline

.PHONY: check
check: vet lint test vuln secrets docs-lint api-lint project-lint generate-check ## Everything CI checks (formatting is checked by lint)

.PHONY: ci
ci: check build docs-links test-integration release-check ## Every CI job's targets, run locally in one go

# Go modules that fmt, vet, lint, test and vuln cover. A module with no
# packages yet is skipped.
GO_MODULES := . tools/projctl tools/hooktest

# $(call each_module,COMMAND) runs COMMAND inside every Go module with packages.
define each_module
	@for m in $(GO_MODULES); do \
		pkgs=$$(go -C $$m list ./... 2>/dev/null) || { echo "$$m: go list failed:"; go -C $$m list ./...; exit 1; }; \
		if [ -z "$$pkgs" ]; then echo "$$m: no packages yet, skipped"; continue; fi; \
		echo "$$m: $(1)"; \
		( cd $$m && $(1) ); \
	done
endef

##@ Go

.PHONY: fmt
fmt: $(GOLANGCI_LINT) ## Format Go code with the configured formatters (gofmt, goimports)
	$(call each_module,$(GOLANGCI_LINT) fmt --config $(ROOT)/.golangci.yml ./...)

# Integration tests carry the "integration" build tag, and the end-to-end
# webhook test "webhooks" too. vet and lint check them as well; otherwise
# they'd skip those files.
.PHONY: vet
vet: ## Run go vet
	$(call each_module,go vet -tags integration$(comma)release$(comma)webhooks ./...)

# The hook tests in tools/hooktest run the pinned jq and golangci-lint.
TEST_TOOLS := $(JQ) $(GOLANGCI_LINT)
TEST_ENV := HOOKTEST_JQ=$(JQ) HOOKTEST_GOLANGCI_LINT=$(GOLANGCI_LINT)

# The race detector needs cgo, so a C compiler (ADR-0022). Without one, Go
# turns cgo off; say what's missing before Go does.
define need_cgo
	@test "$$(go env CGO_ENABLED)" = 1 || { \
		echo "The race detector needs cgo, and so a C compiler, which this host lacks."; \
		echo "Install one (on Debian or Ubuntu: apt install gcc libc6-dev), or run 'make shell'."; \
		exit 1; }
endef

.PHONY: test
test: $(TEST_TOOLS) ## Run unit tests with the race detector
	$(need_cgo)
	$(call each_module,$(TEST_ENV) go test -race ./...)

# The integration tests run only the packages that have them: in each module,
# those whose test files change when the "integration" tag is set. Every other
# package's tests run only in `make test`, so CI's integration job, which
# shares its runner with the lab, compiles and runs no more than it needs
# (ITEM-0075).
TEST_FILES := go list -f '{{.ImportPath}} {{.TestGoFiles}} {{.XTestGoFiles}}'

.PHONY: test-integration
test-integration: lab-up ## Start the lab, then run the integration tests (build tag "integration")
	$(need_cgo)
	@found=; for m in $(GO_MODULES); do \
		pkgs=$$(cd $$m && { $(TEST_FILES) ./...; $(TEST_FILES) -tags integration ./...; } | sort | uniq -u | cut -d' ' -f1 | sort -u); \
		if [ -z "$$pkgs" ]; then echo "$$m: no integration tests, skipped"; continue; fi; \
		found=1; \
		echo "$$m: go test -race -tags integration" $$pkgs; \
		( cd $$m && go test -race -tags integration $$pkgs ); \
	done; \
	[ -n "$$found" ] || { echo "No package has integration tests: is the build tag still \"integration\"?"; exit 1; }

# The end-to-end webhook test (ADR-0035) has the lab's NetBox send its own
# webhooks to nbpdns serve, through the worker of the profile webhooks. It
# needs a local Docker host, which the worker reaches nbpdns on, so it's not
# part of `make ci`; the integration tests replay NetBox's webhooks instead.
.PHONY: test-webhooks
test-webhooks: LAB_PROFILES = webhooks
test-webhooks: lab-up ## Start the lab with NetBox's worker, then have NetBox send webhooks to nbpdns serve (local only)
	$(need_cgo)
	go test -race -count=1 -tags integration,webhooks -run '^TestWebhooksFromNetBox$$' ./internal/cli/

##@ Build

.PHONY: build
build: ## Build nbpdns as a static binary, bin/nbpdns
	CGO_ENABLED=0 go build -trimpath -o bin/nbpdns ./cmd/nbpdns

##@ Release

# GoReleaser builds the release from .goreleaser.yaml (ADR-0030): archives
# for linux/amd64 and linux/arm64, and checksums.txt, in dist/.
.PHONY: release-check
release-check: $(GORELEASER) ## Build the release as a snapshot, into dist/, publishing nothing, and test it
	$(need_cgo)
	$(GORELEASER) release --snapshot --clean
	go test -race -count=1 -tags release ./internal/release/

# make release publishes the release of the version tag at HEAD (ADR-0030):
# the image to RELEASE_IMAGE, logging in to RELEASE_REGISTRY as
# RELEASE_REGISTRY_USER with RELEASE_REGISTRY_PASSWORD, and, if GITLAB_TOKEN
# is set, the archives to a GitLab release at GITLAB_URL (API GITLAB_API_URL),
# with the CHANGELOG's section as its notes. GitLab's tag-only release job
# runs it (ADR-0032). HEAD must have exactly one v tag, vMAJOR.MINOR.PATCH,
# so that Go, which stamps the version, the notes, and GoReleaser, which
# names the release, all use it. The credentials stay in shell variables,
# never make's, so no recipe line can print them.
.PHONY: release
release: $(GORELEASER) ## Publish the version tag at HEAD: the image, and the GitLab release (CI runs it for tags)
	@tag=$$(git tag --points-at HEAD --list 'v*'); \
	[[ $$tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$$ ]] || { echo "release: HEAD needs exactly one v tag, of the form vMAJOR.MINOR.PATCH; it has: $$(echo $${tag:-none})" >&2; exit 1; }; \
	: "$${RELEASE_IMAGE:?release: set RELEASE_IMAGE, such as registry.example.com/group/nbpdns}"; \
	: "$${RELEASE_REGISTRY:?release: set RELEASE_REGISTRY, such as registry.example.com}"; \
	: "$${RELEASE_REGISTRY_USER:?release: set RELEASE_REGISTRY_USER}"; \
	: "$${RELEASE_REGISTRY_PASSWORD:?release: set RELEASE_REGISTRY_PASSWORD}"; \
	[[ $$RELEASE_REGISTRY =~ ^[A-Za-z0-9.-]+(:[0-9]+)?$$ ]] || { echo "release: RELEASE_REGISTRY must be a host, or host:port, such as registry.example.com" >&2; exit 1; }; \
	tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	go run ./internal/cmd/releasenotes -tag "$$tag" CHANGELOG.md > "$$tmp/notes.md"; \
	mkdir -m 700 "$$tmp/docker"; \
	auth=$$(printf '%s:%s' "$$RELEASE_REGISTRY_USER" "$$RELEASE_REGISTRY_PASSWORD" | base64 -w0); \
	printf '{"auths":{"%s":{"auth":"%s"}}}\n' "$$RELEASE_REGISTRY" "$$auth" > "$$tmp/docker/config.json"; \
	echo "release: publishing $$tag to $$RELEASE_IMAGE"; \
	DOCKER_CONFIG="$$tmp/docker" GORELEASER_CURRENT_TAG="$$tag" $(GORELEASER) release --clean --release-notes "$$tmp/notes.md"

# The reference pages generated from the code (internal/cmd/gendocs), and
# the API's server code and embedded spec, generated from api/openapi.yaml
# (ADR-0033): $(call api_code,FILE) writes the code through oapi-codegen,
# and $(call api_spec,FILE) the copy of the spec that the binary serves.
# Never edit them by hand.
api_code = $(call tool,oapi-codegen) -config api/oapi-codegen.yaml -o $(1) api/openapi.yaml
api_spec = { echo '\# Generated by `make generate` from api/openapi.yaml. Edit that file, not this one.'; cat api/openapi.yaml; } > $(1)

.PHONY: generate
generate: ## Regenerate the API's code and embedded spec, and the reference pages
	$(call api_code,internal/api/gen/api.gen.go)
	@$(call api_spec,internal/api/openapi.yaml)
	go run ./internal/cmd/gendocs -out docs/reference

.PHONY: generate-check
generate-check: ## Fail if a generated reference page, or the API's code or embedded spec, is out of date
	@tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	go run ./internal/cmd/gendocs -out "$$tmp"; \
	stale=0; for f in "$$tmp"/*; do \
		page=docs/reference/$$(basename "$$f"); \
		diff -u "$$page" "$$f" || { echo "$$page is out of date; run 'make generate'"; stale=1; }; \
	done; \
	$(call api_code,"$$tmp/api.gen.go"); \
	$(call api_spec,"$$tmp/openapi.yaml"); \
	for f in internal/api/gen/api.gen.go internal/api/openapi.yaml; do \
		diff -u "$$f" "$$tmp/$$(basename "$$f")" || { echo "$$f is out of date; run 'make generate'"; stale=1; }; \
	done; \
	if [ $$stale -eq 0 ]; then echo "generated references and API code: up to date"; fi; \
	exit $$stale

##@ Development lab

# The lab (deploy/dev/compose.yaml, REQ-036), and the release check's image,
# use the Docker host that DOCKER_HOST names, or the local one.
# LAB_DOCKER_HOST, if set, takes its place: a GitHub container job hands its environment to the runner's own
# docker commands too, so a DOCKER_HOST there would send them to the job's
# Docker-in-Docker service, which only the job can reach (ITEM-0041).
ifdef LAB_DOCKER_HOST
export DOCKER_HOST := $(LAB_DOCKER_HOST)
endif
LAB_DIR := deploy/dev
# The lab's credentials are public, so on a local Docker host, including one
# reached over TCP on a loopback address, its ports listen on 127.0.0.1 only.
# A remote one (DOCKER_HOST=tcp://..., as with Docker-in-Docker in CI) is
# reached by name, so they listen on every interface there.
LAB_LOOPBACK := tcp://localhost tcp://localhost:% tcp://127.% tcp://[::1] tcp://[::1]:%
LAB_BIND_ADDRESS := $(if $(filter-out $(LAB_LOOPBACK),$(filter tcp://%,$(DOCKER_HOST))),0.0.0.0,127.0.0.1)
# Compose profiles to start with the lab, such as webhooks, which adds
# NetBox's worker, which sends its webhooks (ADR-0035). CI starts none.
LAB_PROFILES ?=
LAB_COMPOSE = LAB_BIND_ADDRESS=$(LAB_BIND_ADDRESS) $(DOCKER_COMPOSE) --file $(LAB_DIR)/compose.yaml $(foreach p,$(LAB_PROFILES),--profile $(p))

.PHONY: lab-up
lab-up: $(DOCKER_COMPOSE) ## Start the NetBox lab, and wait until it's healthy
	@# A Docker-in-Docker service can still be starting when the job begins.
	@for i in $$(seq 60); do \
		$(LAB_COMPOSE) ls >/dev/null 2>&1 && break; \
		if [ $$i -eq 60 ]; then echo "The Docker daemon isn't answering$${DOCKER_HOST:+ at $$DOCKER_HOST}."; exit 1; fi; \
		sleep 1; \
	done
	$(LAB_COMPOSE) up --detach --wait --wait-timeout 1200

.PHONY: lab-down
lab-down: $(DOCKER_COMPOSE) ## Remove the NetBox lab and its data, in every profile
	$(LAB_COMPOSE) --profile '*' down --volumes --remove-orphans

##@ Security

.PHONY: vuln
vuln: ## Check dependencies for known vulnerabilities (needs network for the vuln DB)
	$(call each_module,$(call tool,govulncheck) ./...)

.PHONY: secrets
secrets: $(GITLEAKS) ## Scan the git history for committed secrets
	$(GITLEAKS) git --no-banner --redact .

##@ Linters

# Prose that Vale checks (docs/contributing/documentation-style.md).
VALE_FILES := docs README.md CHANGELOG.md

VACUUM_LINT = $(VACUUM) lint --no-update-check -r api/ruleset.yaml -b -q -d --no-clip

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Lint Go code with golangci-lint
	$(call each_module,$(GOLANGCI_LINT) run --config $(ROOT)/.golangci.yml ./...)

.PHONY: docs-lint
docs-lint: $(VALE) ## Lint prose with Vale; any finding fails
	@# Findings go to stdout; errors go to stderr and stay visible.
	@rc=0; out=$$($(VALE) --output=line --glob='!**/template.md' $(VALE_FILES)) || rc=$$?; \
	if [ -n "$$out" ]; then echo "$$out"; exit 1; fi; \
	if [ $$rc -ne 0 ]; then echo "vale failed (exit $$rc)"; exit $$rc; fi; \
	echo "vale: no findings in $(VALE_FILES)"

.PHONY: api-lint
api-lint: $(VACUUM) ## Lint api/openapi.yaml against the Zalando ruleset, and self-test the ruleset
	@$(VACUUM_LINT) api/testdata/good.yaml > /dev/null || { $(VACUUM_LINT) api/testdata/good.yaml; echo "api/testdata/good.yaml should pass"; exit 1; }
	@out=$$($(VACUUM_LINT) api/testdata/bad.yaml 2>&1) && { echo "api/testdata/bad.yaml passed; the ruleset has stopped catching problems"; exit 1; }; \
	for rule in $$(grep -oE '^  zalando-[a-z0-9-]+' api/ruleset.yaml); do \
		grep -q -- "$$rule" <<< "$$out" || { echo "api/testdata/bad.yaml doesn't trigger $$rule"; exit 1; }; \
	done; \
	echo "api ruleset: self-test passed"
	@if [ -f api/openapi.yaml ]; then $(VACUUM_LINT) api/openapi.yaml; else echo "api/openapi.yaml doesn't exist yet (M06)"; fi

# Scalar's API reference (ADR-0034), vendored into internal/api/docs: its
# standalone bundle, gzipped as the binary serves it, and its license, each
# checked by SHA-256. The npm package has no license file, so it comes from
# Scalar's repository, at a pinned commit: the repository tags no package's
# releases.
SCALAR_VERSION        := 1.73.1
SCALAR_SHA256         := 424d2f1e55df6c6a485a374bcebbab32c1945b7f2028219b425d79a07b3b15c6
SCALAR_LICENSE_COMMIT := 854b0f448b6f98884e0dc9964995deaf1559c94c
SCALAR_LICENSE_SHA256 := 380cd0a6ad700e1f821f2a509f0dd9ff835041cee2d43daf5dedc1adb2bcc620

.PHONY: vendor-scalar
vendor-scalar: ## Fetch the pinned Scalar API reference into internal/api/docs (needs network)
	@tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	curl -fsSL -o "$$tmp/scalar.tgz" https://registry.npmjs.org/@scalar/api-reference/-/api-reference-$(SCALAR_VERSION).tgz; \
	echo "$(SCALAR_SHA256)  $$tmp/scalar.tgz" | sha256sum --check --quiet; \
	tar -xzf "$$tmp/scalar.tgz" -C "$$tmp" package/dist/browser/standalone.js; \
	gzip -9 -n -c "$$tmp/package/dist/browser/standalone.js" > internal/api/docs/scalar.js.gz; \
	curl -fsSL -o "$$tmp/LICENSE" https://raw.githubusercontent.com/scalar/scalar/$(SCALAR_LICENSE_COMMIT)/LICENSE; \
	echo "$(SCALAR_LICENSE_SHA256)  $$tmp/LICENSE" | sha256sum --check --quiet; \
	cp "$$tmp/LICENSE" internal/api/docs/LICENSE.scalar; \
	echo "vendor-scalar: Scalar $(SCALAR_VERSION) in internal/api/docs"

.PHONY: vale-sync
vale-sync: $(VALE) ## Refresh the vendored Vale style packages (needs network)
	$(VALE) sync

##@ Documentation site

HUGO_CMD = $(HUGO) --source site

.PHONY: docs
docs: $(HUGO) ## Build the docs site into site/public (offline: the theme is vendored)
	$(HUGO_CMD) --minify --cleanDestinationDir

.PHONY: docs-serve
docs-serve: $(HUGO) ## Preview the docs site at http://localhost:1313
	$(HUGO_CMD) server

.PHONY: docs-links
docs-links: docs $(HTMLTEST) ## Build the site, then check its internal links and anchors
	$(HTMLTEST) -c site/htmltest.yml

.PHONY: docs-theme-update
docs-theme-update: $(HUGO) ## Update the vendored Hextra theme to its latest release (needs network)
	$(HUGO_CMD) mod get -u github.com/imfing/hextra
	$(HUGO_CMD) mod vendor
	@echo "Update the version in site/Hextra.LICENSE if the license changed."

##@ Project tracking

# Titles reach projctl through the environment, taken literally: make doesn't
# expand them and the shell doesn't run them, whatever quotes or $(…) they hold.
override TITLE := $(value TITLE)
override TYPE := $(value TYPE)
override MILESTONE := $(value MILESTONE)
export TITLE TYPE MILESTONE

PROJCTL := go -C tools/projctl run . -root $(ROOT)

.PHONY: project
project: ## Regenerate the project board and the ADR index
	$(PROJCTL) index

.PHONY: project-lint
project-lint: ## Check tracking files, Markdown links, and that CI matches `make ci`
	$(PROJCTL) lint

.PHONY: item
item: ## New work item: make item TITLE="…" [TYPE=task] [MILESTONE=Mnn]
	@test -n "$$TITLE" || { echo 'usage: make item TITLE="…" [TYPE=feature|bug|debt|task] [MILESTONE=Mnn]'; exit 2; }
	$(PROJCTL) new item -type "$${TYPE:-task}" $${MILESTONE:+-milestone "$$MILESTONE"} "$$TITLE"

.PHONY: adr
adr: ## New architecture decision record: make adr TITLE="…"
	@test -n "$$TITLE" || { echo 'usage: make adr TITLE="…"'; exit 2; }
	$(PROJCTL) new adr "$$TITLE"

##@ Help

.PHONY: help
help: ## Show this list
	@awk 'BEGIN { FS = ":.*## " } \
		/^##@/ { printf "\n%s\n", substr($$0, 5) } \
		/^[a-zA-Z0-9_-]+:.*## / { printf "  %-18s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
