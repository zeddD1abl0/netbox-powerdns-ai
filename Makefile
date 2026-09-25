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
