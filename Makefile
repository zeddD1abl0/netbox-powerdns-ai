# The single entry point for building, checking and tracking nbpdns.
# CI runs these targets and nothing else (ADR-0013). Run `make` for the list.

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help
MAKEFLAGS += --no-print-directory

ROOT := $(CURDIR)

# $(call tool,NAME) runs the pinned tool NAME from tools/NAME/go.mod.
tool = go tool -modfile=$(ROOT)/tools/$(1)/go.mod $(1)

PROJCTL := go -C tools/projctl run . -root $(ROOT)

# Go modules that fmt, vet, lint, test and vuln cover. A module with no
# packages yet (the root, until M1) is skipped.
GO_MODULES := . tools/projctl

# $(call each_module,COMMAND) runs COMMAND inside every Go module with packages.
define each_module
	@for m in $(GO_MODULES); do \
		if [ -z "$$(go -C $$m list ./... 2>/dev/null)" ]; then echo "$$m: no packages yet, skipped"; continue; fi; \
		echo "$$m: $(1)"; \
		( cd $$m && $(1) ); \
	done
endef

# Prose that Vale checks (docs/contributing/documentation-style.md).
VALE_FILES := docs README.md CHANGELOG.md

VACUUM = $(call tool,vacuum) lint --no-update-check -r api/ruleset.yaml -b -q -d --no-clip

##@ Pipeline

.PHONY: check
check: vet lint test vuln secrets docs-lint api-lint project-lint ## Everything CI checks (formatting is checked by lint)

.PHONY: ci
ci: check docs docs-links ## The full CI pipeline; CI runs exactly this

##@ Go

.PHONY: fmt
fmt: ## Format Go code with the configured formatters (gofmt, goimports)
	$(call each_module,$(call tool,golangci-lint) fmt --config $(ROOT)/.golangci.yml ./...)

.PHONY: vet
vet: ## Run go vet
	$(call each_module,go vet ./...)

.PHONY: test
test: ## Run unit tests with the race detector
	$(call each_module,go test -race ./...)

.PHONY: test-integration
test-integration: ## Run integration tests (build tag "integration") against the container lab
	$(call each_module,go test -race -tags integration ./...)

.PHONY: vuln
vuln: ## Check dependencies for known vulnerabilities (needs network for the vuln DB)
	$(call each_module,$(call tool,govulncheck) ./...)

.PHONY: secrets
secrets: ## Scan the git history for committed secrets
	$(call tool,gitleaks) git --no-banner --redact .

.PHONY: tools-update
tools-update: ## Update every pinned tool and projctl's dependencies to their latest versions
	@for d in tools/*/; do \
		t=$$(basename $$d); \
		if [ "$$t" = projctl ]; then go -C $$d get -u ./... && go -C $$d mod tidy; continue; fi; \
		pkg=$$(awk '/^tool /{print $$2}' $$d/go.mod); \
		echo "$$t: $$pkg@latest"; \
		go -C $$d get -tool $$pkg@latest && go -C $$d mod tidy; \
	done

##@ Linters

.PHONY: lint
lint: ## Lint Go code with golangci-lint
	$(call each_module,$(call tool,golangci-lint) run --config $(ROOT)/.golangci.yml ./...)

.PHONY: docs-lint
docs-lint: ## Lint prose with Vale; any finding fails (Vale needs a C compiler, ADR-0013)
	@# Findings go to stdout; build output and errors go to stderr and stay visible.
	@rc=0; out=$$($(call tool,vale) --output=line --glob='!**/template.md' $(VALE_FILES)) || rc=$$?; \
	if [ -n "$$out" ]; then echo "$$out"; exit 1; fi; \
	if [ $$rc -ne 0 ]; then echo "vale failed (exit $$rc)"; exit $$rc; fi; \
	echo "vale: no findings in $(VALE_FILES)"

.PHONY: api-lint
api-lint: ## Lint api/openapi.yaml against the Zalando ruleset, and self-test the ruleset
	@$(VACUUM) api/testdata/good.yaml > /dev/null || { $(VACUUM) api/testdata/good.yaml; echo "api/testdata/good.yaml should pass"; exit 1; }
	@out=$$($(VACUUM) api/testdata/bad.yaml 2>&1) && { echo "api/testdata/bad.yaml passed; the ruleset has stopped catching problems"; exit 1; }; \
	for rule in $$(grep -oE '^  zalando-[a-z0-9-]+' api/ruleset.yaml); do \
		grep -q -- "$$rule" <<< "$$out" || { echo "api/testdata/bad.yaml doesn't trigger $$rule"; exit 1; }; \
	done; \
	echo "api ruleset: self-test passed"
	@if [ -f api/openapi.yaml ]; then $(VACUUM) api/openapi.yaml; else echo "api/openapi.yaml doesn't exist yet (M1)"; fi

.PHONY: vale-sync
vale-sync: ## Refresh the vendored Vale style packages (needs network)
	$(call tool,vale) sync

##@ Documentation site

HUGO = $(call tool,hugo) --source site

.PHONY: docs
docs: ## Build the docs site into site/public (offline: the theme is vendored)
	$(HUGO) --minify --cleanDestinationDir

.PHONY: docs-serve
docs-serve: ## Preview the docs site at http://localhost:1313
	$(HUGO) server

.PHONY: docs-links
docs-links: ## Check internal links and anchors in the built site (run make docs first)
	$(call tool,htmltest) -c site/htmltest.yml

.PHONY: docs-theme-update
docs-theme-update: ## Update the vendored Hextra theme to its latest release (needs network)
	$(HUGO) mod get -u github.com/imfing/hextra
	$(HUGO) mod vendor
	@echo "Update the version in site/Hextra.LICENSE if the license changed."

##@ Project tracking

.PHONY: project
project: ## Regenerate the project board and the ADR index
	$(PROJCTL) index

.PHONY: project-lint
project-lint: ## Check tracking files, Markdown links and CI files
	$(PROJCTL) lint

.PHONY: item
item: ## New work item: make item TITLE="…" [TYPE=task] [MILESTONE=Mnn]
	@test -n "$(TITLE)" || { echo 'usage: make item TITLE="…" [TYPE=feature|bug|debt|task] [MILESTONE=Mnn]'; exit 2; }
	$(PROJCTL) new item -type $(or $(TYPE),task) $(if $(MILESTONE),-milestone $(MILESTONE)) "$(TITLE)"

.PHONY: adr
adr: ## New architecture decision record: make adr TITLE="…"
	@test -n "$(TITLE)" || { echo 'usage: make adr TITLE="…"'; exit 2; }
	$(PROJCTL) new adr "$(TITLE)"

##@ Help

.PHONY: help
help: ## Show this list
	@awk 'BEGIN { FS = ":.*## " } \
		/^##@/ { printf "\n%s\n", substr($$0, 5) } \
		/^[a-zA-Z0-9_-]+:.*## / { printf "  %-18s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
