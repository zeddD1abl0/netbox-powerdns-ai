# The single entry point for building, checking and tracking nbpdns.
# CI jobs run these targets and nothing else (ADR-0016). Run `make` for the list.

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help
MAKEFLAGS += --no-print-directory

ROOT := $(CURDIR)

# The image CI runs in, pinned by digest. `make project-lint` checks that both
# forges' CI files use exactly this image (ADR-0016).
CI_IMAGE := golang:1.27.1@sha256:3680233e3204827fbdc66088528ae6d4b3d034f51d03a99d454f6de034888244

##@ Tools

# Release binaries are pinned per platform in tools/tools.mk and fetched once
# into .cache/tools/<platform>/ (ADR-0014). Each tool's variable, such as
# $(HUGO), is the path to its binary.
include tools/tools.mk
PLATFORM := $(shell go env GOOS)-$(shell go env GOARCH)
TOOLS_DIR := $(ROOT)/.cache/tools/$(PLATFORM)

# $(call binary_tool,PREFIX,NAME) defines $(PREFIX) and the rule that fetches it.
define binary_tool
$(1) := $$(TOOLS_DIR)/$(2)-$$($(1)_VERSION)
$$($(1)):
	@test -n "$$($(1)_SHA256_$$(PLATFORM))" || { echo "no pinned $(2) for $$(PLATFORM); run the tools in the CI image with 'make shell'"; exit 1; }
	@tools/fetch.sh "https://github.com/$$($(1)_REPO)/releases/download/v$$($(1)_VERSION)/$$($(1)_ASSET_$$(PLATFORM))" \
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
	@for t in govulncheck goimports; do \
		pkg=$$(awk '/^tool /{print $$2}' tools/$$t/go.mod); \
		echo "$$t: $$pkg@latest"; \
		go -C tools/$$t get -tool $$pkg@latest && go -C tools/$$t mod tidy; \
	done
	go -C tools/projctl get -u ./... && go -C tools/projctl mod tidy

.PHONY: tools-audit
tools-audit: ## Check every pinned hash against its release's published checksums file
	tools/update.sh --current
	@git diff --exit-code -- tools/tools.mk || { echo "a pinned hash differs from the published checksums"; exit 1; }

.PHONY: shell
shell: ## Open a shell in the CI image with the repository mounted (for macOS and Windows)
	docker run --rm -it --user "$$(id -u):$$(id -g)" -v "$(ROOT)":/src -w /src \
		-e HOME=/tmp -e GOTOOLCHAIN=local -e GOFLAGS=-modcacherw \
		-e GOMODCACHE=/src/.cache/gomod -e GOCACHE=/src/.cache/gobuild \
		$(CI_IMAGE) bash

##@ Pipeline

.PHONY: check
check: vet lint test vuln secrets docs-lint api-lint project-lint ## Everything CI checks (formatting is checked by lint)

.PHONY: ci
ci: check docs-links ## Every CI job's targets, run locally in one go

# Go modules that fmt, vet, lint, test and vuln cover. A module with no
# packages yet (the root, until M1) is skipped.
GO_MODULES := . tools/projctl

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

.PHONY: vet
vet: ## Run go vet
	$(call each_module,go vet ./...)

.PHONY: test
test: ## Run unit tests with the race detector
	$(call each_module,go test -race ./...)

.PHONY: test-integration
test-integration: ## Run integration tests (build tag "integration") against the container lab
	$(call each_module,go test -race -tags integration ./...)

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
	@if [ -f api/openapi.yaml ]; then $(VACUUM_LINT) api/openapi.yaml; else echo "api/openapi.yaml doesn't exist yet (M1)"; fi

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
