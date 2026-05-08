# SPDX-License-Identifier: MIT
# Copyright (c) 2025-2026 Noble Factor. All rights reserved.

SHELL := bash
.SHELLFLAGS := -o errexit -o nounset -o pipefail -c
.ONESHELL:
.SILENT:

## PARAMETERS

### VERSION

# Version for releases. Set to specific version for draft/pre-release testing.
# Examples:
#   make dist DEVLORE_VERSION=v0.1.0-draft   # Draft release for testing
#   make dist DEVLORE_VERSION=v0.1.0-alpha   # Pre-release
#   make dist                                 # Uses git describe
DEVLORE_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

VERSION ?= $(DEVLORE_VERSION)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -ldflags "-X github.com/NobleFactor/devlore-cli/internal/cli.Version=$(VERSION) -X github.com/NobleFactor/devlore-cli/internal/cli.Commit=$(COMMIT) -X github.com/NobleFactor/devlore-cli/internal/cli.BuildDate=$(BUILD_DATE)"

### PLATFORMS

# Platforms for cross-compilation.
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

### BUILD_TAGS

# Build tags for opt-in test suites.
# Available tags:
#   e2e — LLM-dependent end-to-end tests (requires configured AI provider)
# Usage: make test BUILD_TAGS=e2e
BUILD_TAGS ?=

### STAR

# Code generator (star binary). Resolves to the Last-Known-Good snapshot at
# build/star.lkg when present, falling back to the in-tree build/star.
#
# The LKG binary is the escape hatch when in-tree changes (templates,
# generate.star, runtime types, provider signatures) break star compilation
# or cause it to panic on startup. Without an LKG, codegen rules cannot run
# and gen files cannot be regenerated, forcing hand-patching. Promote a new
# LKG via `make star-lkg` ONLY after a known-green build.
STAR_LKG ?= build/star.lkg
STAR ?= $(if $(wildcard $(STAR_LKG)),$(STAR_LKG),build/star)

## VARIABLES (static)

# Provider source roots.
P := pkg/op/provider
SP := cmd/star/provider

## TARGETS

.PHONY: all build clean test test-race vet lint shell-lint complexity check dev docs dist dist-all star star-lkg generate inventory help

##@ Help

HELP_COLWIDTH ?= 24

help: ## Show available targets
	awk 'BEGIN {FS = ":.*##"; pad = $(HELP_COLWIDTH); print "Usage: make <target> [VAR=VALUE]"; print ""; print "Targets:"} /^[a-zA-Z0-9_.-]+:.*##/ {printf "  %-*s %s\n", pad, $$1, $$2} /^##@/ {printf "\n%s\n", substr($$0,5)}' $(MAKEFILE_LIST)

##@ Build

all: build

star: inventory ## Build the star code generator
	go build $(LDFLAGS) -o build/star ./cmd/star

star-lkg: star ## Snapshot build/star as last-known-good (run after a green build)
	cp build/star $(STAR_LKG)

build: generate ## Build all binaries (lore, star, writ, devlore-test)
	go build $(LDFLAGS) -o build/lore ./cmd/lore
	go build $(LDFLAGS) -o build/star ./cmd/star
	go build $(LDFLAGS) -o build/writ ./cmd/writ
	go build $(LDFLAGS) -o build/devlore-test ./cmd/devlore-test

clean: ## Remove build artifacts
	rm -rf build/

##@ Test

# TAGS controls which tests run. Default: all (untagged + integration + e2e).
# Examples:
#   make test              # all tests
#   make test TAGS=        # untagged only (unit tests)
#   make test TAGS=integration  # untagged + integration
#   make test TAGS=e2e     # untagged + e2e
#   make test-race TAGS=integration  # untagged + integration with race detector
TAGS ?= all
ifeq ($(TAGS),all)
  _TAGS := integration,e2e
else
  _TAGS := $(TAGS)
endif

test: generate ## Run tests (TAGS=all|integration|e2e|"", default: all)
	go test $(if $(_TAGS),-tags '$(_TAGS)') $$(go list ./... | grep -v '/pkg/op/provider$$') -timeout 60s

test-race: generate ## Run tests with race detector (TAGS=all|integration|e2e|"", default: all)
	go test $(if $(_TAGS),-tags '$(_TAGS)') $$(go list ./... | grep -v '/pkg/op/provider$$') -count=1 -race -timeout 120s

##@ Quality

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint
	golangci-lint run

shell-lint: ## Lint shell scripts
	.github/scripts/shell-lint.sh

complexity: ## Check cyclomatic complexity (max 20)
	echo "Checking cyclomatic complexity (max 20)..."
	go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
	if gocyclo -over 20 . | grep -v '_test.go' | head -1 | grep -q .; then
		echo "ERROR: Functions with complexity > 20:"
		gocyclo -over 20 . | grep -v '_test.go'
		exit 1
	fi
	echo "All functions below complexity threshold."

check: vet lint shell-lint complexity test ## Run all quality checks

##@ Development

dev: ## Activate git hooks
	git config core.hooksPath .githooks
	echo "Hooks activated: .githooks/pre-commit"

docs: generate ## Generate CLI documentation
	go run ./cmd/docgen --output-dir=docs/cli --version=$(VERSION)

##@ Distribution

dist: dist-all checksums ## Build distribution archives with checksums

dist-all: ## Build distribution archives for all platforms
	mkdir -p dist
	for platform in $(PLATFORMS); do
		os=$${platform%/*}
		arch=$${platform#*/}
		ext=""
		archive_ext="tar.gz"
		if [[ "$$os" == "windows" ]]; then ext=".exe"; archive_ext="zip"; fi
		echo "Building $$os/$$arch..."
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build $(LDFLAGS) -o dist/writ$$ext ./cmd/writ
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build $(LDFLAGS) -o dist/lore$$ext ./cmd/lore
		if [[ "$$archive_ext" == "tar.gz" ]]; then
			tar -czf dist/devlore-cli_$(VERSION)_$${os}_$${arch}.tar.gz -C dist writ$$ext lore$$ext
		else
			cd dist && zip -q devlore-cli_$(VERSION)_$${os}_$${arch}.zip writ$$ext lore$$ext && cd ..
		fi
		rm -f dist/writ$$ext dist/lore$$ext
	done

checksums: ## Generate SHA-256 checksums for distribution archives
	cd dist && shasum -a 256 devlore-cli_$(VERSION)_*.* > devlore-cli_$(VERSION)_checksums.txt
	echo "Checksums written to dist/devlore-cli_$(VERSION)_checksums.txt"

dist-clean: ## Remove distribution archives
	rm -rf dist/

##@ Code Generation

# Each grouped target (&:) fires one star invocation that produces all gen files.
# Generation runs only when provider.go is newer than the gen outputs.
#
# access=both     → receiver_type.gen_test + action.gen_test + node_builder.gen_test + module.gen_test + provider
# access=planned  → receiver_type.gen_test + action.gen_test + node_builder.gen_test + provider
# access=immediate → receiver_type.gen_test + module.gen_test + provider

# --- access=both providers ---

$(P)/json/gen/receiver_type.gen_test.go \
$(P)/json/gen/action.gen_test.go \
$(P)/json/gen/node_builder.gen_test.go \
$(P)/json/gen/module.gen_test.go \
$(P)/json/gen/provider.gen.go &: $(P)/json/provider.go
	$(STAR) devlore actions generate --source=$(P)/json --gen=true --write=true --output=$(P)/json

$(P)/platform/gen/receiver_type.gen_test.go \
$(P)/platform/gen/action.gen_test.go \
$(P)/platform/gen/node_builder.gen_test.go \
$(P)/platform/gen/module.gen_test.go \
$(P)/platform/gen/provider.gen.go &: $(P)/platform/provider.go
	$(STAR) devlore actions generate --source=$(P)/platform --gen=true --write=true --output=$(P)/platform

$(P)/regexp/gen/receiver_type.gen_test.go \
$(P)/regexp/gen/action.gen_test.go \
$(P)/regexp/gen/node_builder.gen_test.go \
$(P)/regexp/gen/module.gen_test.go \
$(P)/regexp/gen/provider.gen.go &: $(P)/regexp/provider.go
	$(STAR) devlore actions generate --source=$(P)/regexp --gen=true --write=true --output=$(P)/regexp

$(P)/template/gen/receiver_type.gen_test.go \
$(P)/template/gen/action.gen_test.go \
$(P)/template/gen/node_builder.gen_test.go \
$(P)/template/gen/module.gen_test.go \
$(P)/template/gen/provider.gen.go &: $(P)/template/provider.go
	$(STAR) devlore actions generate --source=$(P)/template --gen=true --write=true --output=$(P)/template

$(P)/yaml/gen/receiver_type.gen_test.go \
$(P)/yaml/gen/action.gen_test.go \
$(P)/yaml/gen/node_builder.gen_test.go \
$(P)/yaml/gen/module.gen_test.go \
$(P)/yaml/gen/provider.gen.go &: $(P)/yaml/provider.go $(P)/yaml/resource.go
	$(STAR) devlore actions generate --source=$(P)/yaml --gen=true --write=true --output=$(P)/yaml

# --- access=planned providers ---

$(P)/appnet/gen/receiver_type.gen_test.go \
$(P)/appnet/gen/action.gen_test.go \
$(P)/appnet/gen/node_builder.gen_test.go \
$(P)/appnet/gen/provider.gen.go \
$(P)/appnet/gen/resource.gen.go &: $(P)/appnet/provider.go $(P)/appnet/resource.go
	$(STAR) devlore actions generate --source=$(P)/appnet --gen=true --write=true --output=$(P)/appnet

$(P)/archive/gen/receiver_type.gen_test.go \
$(P)/archive/gen/action.gen_test.go \
$(P)/archive/gen/node_builder.gen_test.go \
$(P)/archive/gen/provider.gen.go &: $(P)/archive/provider.go
	$(STAR) devlore actions generate --source=$(P)/archive --gen=true --write=true --output=$(P)/archive

$(P)/encryption/gen/receiver_type.gen_test.go \
$(P)/encryption/gen/action.gen_test.go \
$(P)/encryption/gen/node_builder.gen_test.go \
$(P)/encryption/gen/provider.gen.go &: $(P)/encryption/provider.go
	$(STAR) devlore actions generate --source=$(P)/encryption --gen=true --write=true --output=$(P)/encryption

$(P)/file/gen/receiver_type.gen_test.go \
$(P)/file/gen/action.gen_test.go \
$(P)/file/gen/node_builder.gen_test.go \
$(P)/file/gen/module.gen_test.go \
$(P)/file/gen/provider.gen.go &: $(P)/file/provider.go $(P)/file/resource.go
	$(STAR) devlore actions generate --source=$(P)/file --gen=true --write=true --output=$(P)/file

$(P)/git/gen/receiver_type.gen_test.go \
$(P)/git/gen/action.gen_test.go \
$(P)/git/gen/node_builder.gen_test.go \
$(P)/git/gen/provider.gen.go \
$(P)/git/gen/resource.gen.go &: $(P)/git/provider.go $(P)/git/resource.go
	$(STAR) devlore actions generate --source=$(P)/git --gen=true --write=true --output=$(P)/git

$(P)/pkg/gen/receiver_type.gen_test.go \
$(P)/pkg/gen/action.gen_test.go \
$(P)/pkg/gen/node_builder.gen_test.go \
$(P)/pkg/gen/provider.gen.go \
$(P)/pkg/gen/resource.gen.go &: $(P)/pkg/provider.go $(P)/pkg/resource.go
	$(STAR) devlore actions generate --source=$(P)/pkg --gen=true --write=true --output=$(P)/pkg

$(P)/service/gen/receiver_type.gen_test.go \
$(P)/service/gen/action.gen_test.go \
$(P)/service/gen/node_builder.gen_test.go \
$(P)/service/gen/provider.gen.go \
$(P)/service/gen/resource.gen.go &: $(P)/service/provider.go $(P)/service/resource.go
	$(STAR) devlore actions generate --source=$(P)/service --gen=true --write=true --output=$(P)/service

$(P)/shell/gen/receiver_type.gen_test.go \
$(P)/shell/gen/action.gen_test.go \
$(P)/shell/gen/node_builder.gen_test.go \
$(P)/shell/gen/provider.gen.go &: $(P)/shell/provider.go
	$(STAR) devlore actions generate --source=$(P)/shell --gen=true --write=true --output=$(P)/shell

$(P)/powershell/gen/receiver_type.gen_test.go \
$(P)/powershell/gen/action.gen_test.go \
$(P)/powershell/gen/node_builder.gen_test.go \
$(P)/powershell/gen/provider.gen.go &: $(P)/powershell/provider.go
	$(STAR) devlore actions generate --source=$(P)/powershell --gen=true --write=true --output=$(P)/powershell

$(P)/flow/gen/receiver_type.gen_test.go \
$(P)/flow/gen/action.gen_test.go \
$(P)/flow/gen/node_builder.gen_test.go \
$(P)/flow/gen/provider.gen.go &: $(P)/flow/provider.go
	$(STAR) devlore actions generate --source=$(P)/flow --gen=true --write=true --output=$(P)/flow

# --- access=immediate providers ---

$(P)/plan/gen/receiver_type.gen_test.go \
$(P)/plan/gen/module.gen_test.go \
$(P)/plan/gen/provider.gen.go &: $(P)/plan/provider.go
	$(STAR) devlore actions generate --source=$(P)/plan --gen=true --write=true --output=$(P)/plan

# --- star-specific providers (cmd/star/provider, access=immediate) ---

$(SP)/staranalysis/gen/receiver_type.gen_test.go \
$(SP)/staranalysis/gen/module.gen_test.go \
$(SP)/staranalysis/gen/provider.gen.go &: $(SP)/staranalysis/provider.go
	$(STAR) devlore actions generate --source=$(SP)/staranalysis --gen=true --write=true --output=$(SP)/staranalysis

$(SP)/starcode/gen/receiver_type.gen_test.go \
$(SP)/starcode/gen/module.gen_test.go \
$(SP)/starcode/gen/provider.gen.go &: $(SP)/starcode/provider.go
	$(STAR) devlore actions generate --source=$(SP)/starcode --gen=true --write=true --output=$(SP)/starcode

$(SP)/starcomplexity/gen/receiver_type.gen_test.go \
$(SP)/starcomplexity/gen/module.gen_test.go \
$(SP)/starcomplexity/gen/provider.gen.go &: $(SP)/starcomplexity/provider.go
	$(STAR) devlore actions generate --source=$(SP)/starcomplexity --gen=true --write=true --output=$(SP)/starcomplexity

$(SP)/starindex/gen/receiver_type.gen_test.go \
$(SP)/starindex/gen/module.gen_test.go \
$(SP)/starindex/gen/provider.gen.go &: $(SP)/starindex/provider.go
	$(STAR) devlore actions generate --source=$(SP)/starindex --gen=true --write=true --output=$(SP)/starindex

$(SP)/starstats/gen/receiver_type.gen_test.go \
$(SP)/starstats/gen/module.gen_test.go \
$(SP)/starstats/gen/provider.gen.go &: $(SP)/starstats/provider.go
	$(STAR) devlore actions generate --source=$(SP)/starstats --gen=true --write=true --output=$(SP)/starstats

$(SP)/commands/gen/receiver_type.gen_test.go \
$(SP)/commands/gen/module.gen_test.go \
$(SP)/commands/gen/provider.gen.go &: $(SP)/commands/provider.go
	$(STAR) devlore actions generate --source=$(SP)/commands --gen=true --write=true --output=$(SP)/commands

$(SP)/config/gen/receiver_type.gen_test.go \
$(SP)/config/gen/module.gen_test.go \
$(SP)/config/gen/provider.gen.go &: $(SP)/config/provider.go
	$(STAR) devlore actions generate --source=$(SP)/config --gen=true --write=true --output=$(SP)/config

$(SP)/goast/gen/receiver_type.gen_test.go \
$(SP)/goast/gen/module.gen_test.go \
$(SP)/goast/gen/provider.gen.go &: $(SP)/goast/provider.go
	$(STAR) devlore actions generate --source=$(SP)/goast --gen=true --write=true --output=$(SP)/goast

$(SP)/lint/gen/receiver_type.gen_test.go \
$(SP)/lint/gen/module.gen_test.go \
$(SP)/lint/gen/provider.gen.go &: $(SP)/lint/provider.go
	$(STAR) devlore actions generate --source=$(SP)/lint --gen=true --write=true --output=$(SP)/lint

$(SP)/setup/gen/receiver_type.gen_test.go \
$(SP)/setup/gen/module.gen_test.go \
$(SP)/setup/gen/provider.gen.go &: $(SP)/setup/provider.go
	$(STAR) devlore actions generate --source=$(SP)/setup --gen=true --write=true --output=$(SP)/setup

$(SP)/shellcheck/gen/receiver_type.gen_test.go \
$(SP)/shellcheck/gen/module.gen_test.go \
$(SP)/shellcheck/gen/provider.gen.go &: $(SP)/shellcheck/provider.go
	$(STAR) devlore actions generate --source=$(SP)/shellcheck --gen=true --write=true --output=$(SP)/shellcheck

$(P)/ui/gen/receiver_type.gen_test.go \
$(P)/ui/gen/module.gen_test.go \
$(P)/ui/gen/provider.gen.go &: $(P)/ui/provider.go
	$(STAR) devlore actions generate --source=$(P)/ui --gen=true --write=true --output=$(P)/ui

# --- resource-only packages ---

$(P)/function/gen/resource.gen.go: $(P)/function/resource.go
	$(STAR) devlore actions generate --source=$(P)/function --gen=true --write=true --output=$(P)/function

$(P)/mem/gen/resource.gen.go: $(P)/mem/resource.go
	$(STAR) devlore actions generate --source=$(P)/mem --gen=true --write=true --output=$(P)/mem

NEW_OP_INVENTORY := \
	$(P)/appnet/gen/provider.gen.go \
	$(P)/archive/gen/provider.gen.go \
	$(P)/encryption/gen/provider.gen.go \
	$(P)/file/gen/provider.gen.go \
	$(P)/flow/gen/provider.gen.go \
	$(P)/function/gen/resource.gen.go \
	$(P)/git/gen/provider.gen.go \
	$(P)/json/gen/provider.gen.go \
	$(P)/mem/gen/resource.gen.go \
	$(P)/plan/gen/provider.gen.go \
	$(P)/platform/gen/provider.gen.go \
	$(P)/pkg/gen/provider.gen.go \
	$(P)/regexp/gen/provider.gen.go \
	$(P)/service/gen/provider.gen.go \
	$(P)/shell/gen/provider.gen.go \
	$(SP)/staranalysis/gen/provider.gen.go \
	$(SP)/starcode/gen/provider.gen.go \
	$(SP)/starcomplexity/gen/provider.gen.go \
	$(SP)/starindex/gen/provider.gen.go \
	$(SP)/starstats/gen/provider.gen.go \
	$(SP)/commands/gen/provider.gen.go \
	$(SP)/config/gen/provider.gen.go \
	$(SP)/goast/gen/provider.gen.go \
	$(SP)/lint/gen/provider.gen.go \
	$(SP)/setup/gen/provider.gen.go \
	$(SP)/shellcheck/gen/provider.gen.go \
	$(P)/template/gen/provider.gen.go \
	$(P)/ui/gen/provider.gen.go \
	$(P)/yaml/gen/provider.gen.go

inventory: ## Generate inventory files from op.Announce* call sites
	go run ./tools/New-OpInventory pkg/op/inventory/inventory.gen.go github.com/NobleFactor/devlore-cli pkg/op
	go run ./tools/New-OpInventory cmd/star/inventory/inventory.gen.go github.com/NobleFactor/devlore-cli cmd/star

generate: $(NEW_OP_INVENTORY) inventory ## Run all code generation
