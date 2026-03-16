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

# Code generator (star binary from sibling repo).
STAR_REPO ?= ../noblefactor-ops
STAR ?= $(STAR_REPO)/bin/star

## VARIABLES (static)

# Provider source root.
P := pkg/op/provider

## TARGETS

.PHONY: all build clean test test-race vet lint shell-lint complexity check dev docs dist dist-all star generate generate-register help

##@ Help

HELP_COLWIDTH ?= 24

help: ## Show available targets
	awk 'BEGIN {FS = ":.*##"; pad = $(HELP_COLWIDTH); print "Usage: make <target> [VAR=VALUE]"; print ""; print "Targets:"} /^[a-zA-Z0-9_.-]+:.*##/ {printf "  %-*s %s\n", pad, $$1, $$2} /^##@/ {printf "\n%s\n", substr($$0,5)}' $(MAKEFILE_LIST)

##@ Build

all: build

star: ## Build the star code generator from noblefactor-ops
	cd $(STAR_REPO) && go build -o bin/star ./cmd/star

build: generate ## Build all binaries (lore, writ, devlore-test)
	go build $(LDFLAGS) -o build/lore ./cmd/lore
	go build $(LDFLAGS) -o build/writ ./cmd/writ
	go build $(LDFLAGS) -o build/devlore-test ./cmd/devlore-test

clean: ## Remove build artifacts, generated files, and gen/ directories
	rm -rf build/
	rm -f $(P)/register.go
	find $(P) -type d -name gen -exec rm -rf {} +

##@ Test

test: generate ## Run tests (use BUILD_TAGS=e2e to include E2E tests)
	go test $(if $(BUILD_TAGS),-tags '$(BUILD_TAGS)') $$(go list ./... | grep -v '/pkg/op/provider$$') -timeout 60s

test-race: generate ## Run tests with race detector
	go test $(if $(BUILD_TAGS),-tags '$(BUILD_TAGS)') $$(go list ./... | grep -v '/pkg/op/provider$$') -count=1 -race -timeout 120s

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
# access=both     → actions_gen_test + receiver + receiver_gen_test + params
# access=planned  → actions_gen_test + receiver + params
# access=immediate → receiver + receiver_gen_test + params
# Dependent types  → <type_snake>.gen.go (e.g., sources.gen.go for starcode.Sources)

# --- access=both providers ---

$(P)/json/gen/actions_gen_test.go \
$(P)/json/gen/params.gen.go \
$(P)/json/gen/receiver.gen.go \
$(P)/json/gen/receiver_gen_test.go &: $(P)/json/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/json --gen=true --write=true --output=$(P)/json

$(P)/platform/gen/actions_gen_test.go \
$(P)/platform/gen/params.gen.go \
$(P)/platform/gen/receiver.gen.go \
$(P)/platform/gen/receiver_gen_test.go &: $(P)/platform/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/platform --gen=true --write=true --output=$(P)/platform

$(P)/regexp/gen/actions_gen_test.go \
$(P)/regexp/gen/params.gen.go \
$(P)/regexp/gen/receiver.gen.go \
$(P)/regexp/gen/receiver_gen_test.go &: $(P)/regexp/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/regexp --gen=true --write=true --output=$(P)/regexp

$(P)/template/gen/actions_gen_test.go \
$(P)/template/gen/params.gen.go \
$(P)/template/gen/receiver.gen.go \
$(P)/template/gen/receiver_gen_test.go &: $(P)/template/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/template --gen=true --write=true --output=$(P)/template

$(P)/yaml/gen/actions_gen_test.go \
$(P)/yaml/gen/params.gen.go \
$(P)/yaml/gen/receiver.gen.go \
$(P)/yaml/gen/receiver_gen_test.go &: $(P)/yaml/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/yaml --gen=true --write=true --output=$(P)/yaml

# --- access=planned providers ---

$(P)/appnet/gen/actions_gen_test.go \
$(P)/appnet/gen/params.gen.go \
$(P)/appnet/gen/receiver.gen.go \
$(P)/appnet/gen/resource.gen.go &: $(P)/appnet/provider.go $(P)/appnet/resource.go | star
	$(STAR) devlore actions generate --source=$(P)/appnet --gen=true --write=true --output=$(P)/appnet

$(P)/archive/gen/actions_gen_test.go \
$(P)/archive/gen/params.gen.go \
$(P)/archive/gen/receiver.gen.go &: $(P)/archive/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/archive --gen=true --write=true --output=$(P)/archive

$(P)/encryption/gen/actions_gen_test.go \
$(P)/encryption/gen/params.gen.go \
$(P)/encryption/gen/receiver.gen.go &: $(P)/encryption/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/encryption --gen=true --write=true --output=$(P)/encryption

$(P)/file/gen/actions_gen_test.go \
$(P)/file/gen/params.gen.go \
$(P)/file/gen/receiver.gen.go \
$(P)/file/gen/resource.gen.go &: $(P)/file/provider.go $(P)/file/resource.go | star
	$(STAR) devlore actions generate --source=$(P)/file --gen=true --write=true --output=$(P)/file

$(P)/git/gen/actions_gen_test.go \
$(P)/git/gen/params.gen.go \
$(P)/git/gen/receiver.gen.go \
$(P)/git/gen/resource.gen.go &: $(P)/git/provider.go $(P)/git/resource.go | star
	$(STAR) devlore actions generate --source=$(P)/git --gen=true --write=true --output=$(P)/git

$(P)/pkg/gen/actions_gen_test.go \
$(P)/pkg/gen/params.gen.go \
$(P)/pkg/gen/receiver.gen.go \
$(P)/pkg/gen/resource.gen.go &: $(P)/pkg/provider.go $(P)/pkg/resource.go | star
	$(STAR) devlore actions generate --source=$(P)/pkg --gen=true --write=true --output=$(P)/pkg

$(P)/service/gen/actions_gen_test.go \
$(P)/service/gen/params.gen.go \
$(P)/service/gen/receiver.gen.go \
$(P)/service/gen/resource.gen.go &: $(P)/service/provider.go $(P)/service/resource.go | star
	$(STAR) devlore actions generate --source=$(P)/service --gen=true --write=true --output=$(P)/service

$(P)/shell/gen/actions_gen_test.go \
$(P)/shell/gen/params.gen.go \
$(P)/shell/gen/receiver.gen.go &: $(P)/shell/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/shell --gen=true --write=true --output=$(P)/shell

# --- access=immediate providers ---

$(P)/staranalysis/gen/params.gen.go \
$(P)/staranalysis/gen/receiver.gen.go \
$(P)/staranalysis/gen/receiver_gen_test.go &: $(P)/staranalysis/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/staranalysis --gen=true --write=true --output=$(P)/staranalysis

$(P)/starcode/gen/params.gen.go \
$(P)/starcode/gen/receiver.gen.go \
$(P)/starcode/gen/receiver_gen_test.go &: $(P)/starcode/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/starcode --gen=true --write=true --output=$(P)/starcode

$(P)/starcomplexity/gen/params.gen.go \
$(P)/starcomplexity/gen/receiver.gen.go \
$(P)/starcomplexity/gen/receiver_gen_test.go &: $(P)/starcomplexity/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/starcomplexity --gen=true --write=true --output=$(P)/starcomplexity

$(P)/starindex/gen/params.gen.go \
$(P)/starindex/gen/receiver.gen.go \
$(P)/starindex/gen/receiver_gen_test.go &: $(P)/starindex/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/starindex --gen=true --write=true --output=$(P)/starindex

$(P)/starstats/gen/params.gen.go \
$(P)/starstats/gen/receiver.gen.go \
$(P)/starstats/gen/receiver_gen_test.go &: $(P)/starstats/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/starstats --gen=true --write=true --output=$(P)/starstats

$(P)/ui/gen/params.gen.go \
$(P)/ui/gen/receiver.gen.go \
$(P)/ui/gen/receiver_gen_test.go &: $(P)/ui/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/ui --gen=true --write=true --output=$(P)/ui

# --- resource-only packages ---

$(P)/mem/gen/resource.gen.go: $(P)/mem/resource.go | star
	$(STAR) devlore actions generate --source=$(P)/mem --gen=true --write=true --output=$(P)/mem

GEN_PROVIDERS := \
	$(P)/appnet/gen/receiver.gen.go \
	$(P)/archive/gen/receiver.gen.go \
	$(P)/encryption/gen/receiver.gen.go \
	$(P)/file/gen/receiver.gen.go \
	$(P)/git/gen/receiver.gen.go \
	$(P)/json/gen/receiver.gen.go \
	$(P)/mem/gen/resource.gen.go \
	$(P)/platform/gen/receiver.gen.go \
	$(P)/pkg/gen/receiver.gen.go \
	$(P)/regexp/gen/receiver.gen.go \
	$(P)/service/gen/receiver.gen.go \
	$(P)/shell/gen/receiver.gen.go \
	$(P)/staranalysis/gen/receiver.gen.go \
	$(P)/starcode/gen/receiver.gen.go \
	$(P)/starcomplexity/gen/receiver.gen.go \
	$(P)/starindex/gen/receiver.gen.go \
	$(P)/starstats/gen/receiver.gen.go \
	$(P)/template/gen/receiver.gen.go \
	$(P)/ui/gen/receiver.gen.go \
	$(P)/yaml/gen/receiver.gen.go

generate-register: $(GEN_PROVIDERS) ## Generate provider register.go with blank imports
	echo '// Code generated by make generate-register; DO NOT EDIT.' > $(P)/register.go
	echo '' >> $(P)/register.go
	echo '// Package provider triggers init() in all provider packages via blank imports.' >> $(P)/register.go
	echo '// Importing this package causes every provider to call op.Announce(), making' >> $(P)/register.go
	echo '// them available for op.InitAll().' >> $(P)/register.go
	echo 'package provider' >> $(P)/register.go
	echo '' >> $(P)/register.go
	echo 'import (' >> $(P)/register.go
	echo '	_ "github.com/NobleFactor/devlore-cli/pkg/op/flow"' >> $(P)/register.go
	for dir in $$(find $(P) -type d -name gen | sort); do
		pkg=$$(echo $$dir | sed 's|^|github.com/NobleFactor/devlore-cli/|')
		echo "	_ \"$$pkg\"" >> $(P)/register.go
	done
	echo ')' >> $(P)/register.go

generate: generate-register ## Run all code generation
