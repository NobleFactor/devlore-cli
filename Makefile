# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.

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

# Platforms for cross-compilation
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

# Code generator (star binary from sibling repo)
STAR_REPO ?= ../noblefactor-ops.binding-unification
STAR ?= $(STAR_REPO)/bin/star

# Provider source root
P := pkg/op/provider

.PHONY: all build clean test vet lint shell-lint complexity check dev docs dist dist-all star generate

all: build

star:
	cd $(STAR_REPO) && go build -o bin/star ./cmd/star

# ── Provider code generation ────────────────────────────────────────────────────
# Each grouped target (&:) fires one star invocation that produces all gen files.
# Generation runs only when provider.go is newer than the gen outputs.

$(P)/file/gen/actions.gen.go \
$(P)/file/gen/immediate.gen.go \
$(P)/file/gen/planned.gen.go &: $(P)/file/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/file --gen=true --write=true --output=$(P)/file

$(P)/staranalysis/gen/convert.gen.go \
$(P)/staranalysis/gen/immediate.gen.go &: $(P)/staranalysis/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/staranalysis --gen=true --write=true --output=$(P)/staranalysis

$(P)/starcode/gen/immediate.gen.go \
$(P)/starcode/gen/sources.gen.go &: $(P)/starcode/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/starcode --gen=true --write=true --output=$(P)/starcode

$(P)/starcomplexity/gen/convert.gen.go \
$(P)/starcomplexity/gen/immediate.gen.go &: $(P)/starcomplexity/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/starcomplexity --gen=true --write=true --output=$(P)/starcomplexity

$(P)/starindex/gen/convert.gen.go \
$(P)/starindex/gen/immediate.gen.go &: $(P)/starindex/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/starindex --gen=true --write=true --output=$(P)/starindex

$(P)/starsources/gen/sources.gen.go &: $(P)/starsources/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/starsources --gen=true --write=true --output=$(P)/starsources

$(P)/starstats/gen/convert.gen.go \
$(P)/starstats/gen/immediate.gen.go &: $(P)/starstats/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/starstats --gen=true --write=true --output=$(P)/starstats

generate: \
	$(P)/file/gen/immediate.gen.go \
	$(P)/staranalysis/gen/immediate.gen.go \
	$(P)/starcode/gen/immediate.gen.go \
	$(P)/starcomplexity/gen/immediate.gen.go \
	$(P)/starindex/gen/immediate.gen.go \
	$(P)/starsources/gen/sources.gen.go \
	$(P)/starstats/gen/immediate.gen.go

build: generate
	go build $(LDFLAGS) -o build/lore ./cmd/lore
	go build $(LDFLAGS) -o build/writ ./cmd/writ

clean:
	rm -rf build/

test:
	go test ./... -timeout 60s

vet:
	go vet ./...

lint:
	golangci-lint run

shell-lint:
	.github/scripts/shell-lint.sh

complexity:
	@echo "Checking cyclomatic complexity (max 20)..."
	@go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
	@if gocyclo -over 20 . | grep -v '_test.go' | head -1 | grep -q .; then \
		echo "ERROR: Functions with complexity > 20:"; \
		gocyclo -over 20 . | grep -v '_test.go'; \
		exit 1; \
	fi
	@echo "All functions below complexity threshold."

check: vet lint shell-lint complexity test

dev:
	git config core.hooksPath .githooks
	@echo "Hooks activated: .githooks/pre-commit"

docs:
	go run ./cmd/docgen --output-dir=docs/cli --version=$(VERSION)

# Build distribution archives for all platforms
dist: dist-all checksums

dist-all:
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		archive_ext="tar.gz"; \
		if [ "$$os" = "windows" ]; then ext=".exe"; archive_ext="zip"; fi; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build $(LDFLAGS) -o dist/writ$$ext ./cmd/writ; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build $(LDFLAGS) -o dist/lore$$ext ./cmd/lore; \
		if [ "$$archive_ext" = "tar.gz" ]; then \
			tar -czf dist/devlore-cli_$(VERSION)_$${os}_$${arch}.tar.gz -C dist writ$$ext lore$$ext; \
		else \
			cd dist && zip -q devlore-cli_$(VERSION)_$${os}_$${arch}.zip writ$$ext lore$$ext && cd ..; \
		fi; \
		rm -f dist/writ$$ext dist/lore$$ext; \
	done

checksums:
	@cd dist && shasum -a 256 devlore-cli_$(VERSION)_*.* > devlore-cli_$(VERSION)_checksums.txt
	@echo "Checksums written to dist/devlore-cli_$(VERSION)_checksums.txt"

dist-clean:
	rm -rf dist/
