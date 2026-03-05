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
STAR_REPO ?= ../noblefactor-ops
STAR ?= $(STAR_REPO)/bin/star

# Provider source root
P := pkg/op/provider

.PHONY: all build clean test test-race vet lint shell-lint complexity check dev docs dist dist-all star generate

all: build

star:
	cd $(STAR_REPO) && go build -o bin/star ./cmd/star

# ── Provider code generation ────────────────────────────────────────────────────
# Each grouped target (&:) fires one star invocation that produces all gen files.
# Generation runs only when provider.go is newer than the gen outputs.
#
# access=both    → actions + immediate + params + planned
# access=immediate → immediate + params
# Dependent types  → <type_snake>.gen.go (e.g., sources.gen.go for starcode.Sources)

# ── access=both providers ─────────────────────────────────────────────────────

$(P)/archive/gen/actions.gen.go \
$(P)/archive/gen/actions_gen_test.go \
$(P)/archive/gen/immediate.gen.go \
$(P)/archive/gen/immediate_gen_test.go \
$(P)/archive/gen/params.gen.go \
$(P)/archive/gen/planned.gen.go &: $(P)/archive/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/archive --gen=true --write=true --output=$(P)/archive

$(P)/encryption/gen/actions.gen.go \
$(P)/encryption/gen/actions_gen_test.go \
$(P)/encryption/gen/immediate.gen.go \
$(P)/encryption/gen/immediate_gen_test.go \
$(P)/encryption/gen/params.gen.go \
$(P)/encryption/gen/planned.gen.go &: $(P)/encryption/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/encryption --gen=true --write=true --output=$(P)/encryption

$(P)/file/gen/actions.gen.go \
$(P)/file/gen/actions_gen_test.go \
$(P)/file/gen/immediate.gen.go \
$(P)/file/gen/immediate_gen_test.go \
$(P)/file/gen/params.gen.go \
$(P)/file/gen/planned.gen.go &: $(P)/file/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/file --gen=true --write=true --output=$(P)/file

$(P)/git/gen/actions.gen.go \
$(P)/git/gen/actions_gen_test.go \
$(P)/git/gen/immediate.gen.go \
$(P)/git/gen/immediate_gen_test.go \
$(P)/git/gen/params.gen.go \
$(P)/git/gen/planned.gen.go &: $(P)/git/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/git --gen=true --write=true --output=$(P)/git

$(P)/net/gen/actions.gen.go \
$(P)/net/gen/actions_gen_test.go \
$(P)/net/gen/immediate.gen.go \
$(P)/net/gen/immediate_gen_test.go \
$(P)/net/gen/params.gen.go \
$(P)/net/gen/planned.gen.go &: $(P)/net/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/net --gen=true --write=true --output=$(P)/net

$(P)/pkg/gen/actions.gen.go \
$(P)/pkg/gen/actions_gen_test.go \
$(P)/pkg/gen/immediate.gen.go \
$(P)/pkg/gen/immediate_gen_test.go \
$(P)/pkg/gen/params.gen.go \
$(P)/pkg/gen/planned.gen.go &: $(P)/pkg/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/pkg --gen=true --write=true --output=$(P)/pkg

$(P)/service/gen/actions.gen.go \
$(P)/service/gen/actions_gen_test.go \
$(P)/service/gen/immediate.gen.go \
$(P)/service/gen/immediate_gen_test.go \
$(P)/service/gen/params.gen.go \
$(P)/service/gen/planned.gen.go &: $(P)/service/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/service --gen=true --write=true --output=$(P)/service

$(P)/shell/gen/actions.gen.go \
$(P)/shell/gen/actions_gen_test.go \
$(P)/shell/gen/immediate.gen.go \
$(P)/shell/gen/immediate_gen_test.go \
$(P)/shell/gen/params.gen.go \
$(P)/shell/gen/planned.gen.go &: $(P)/shell/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/shell --gen=true --write=true --output=$(P)/shell

$(P)/template/gen/actions.gen.go \
$(P)/template/gen/actions_gen_test.go \
$(P)/template/gen/immediate.gen.go \
$(P)/template/gen/immediate_gen_test.go \
$(P)/template/gen/params.gen.go \
$(P)/template/gen/planned.gen.go &: $(P)/template/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/template --gen=true --write=true --output=$(P)/template

# ── access=immediate providers ────────────────────────────────────────────────

$(P)/json/gen/immediate.gen.go \
$(P)/json/gen/immediate_gen_test.go \
$(P)/json/gen/params.gen.go &: $(P)/json/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/json --gen=true --write=true --output=$(P)/json

$(P)/regexp/gen/immediate.gen.go \
$(P)/regexp/gen/immediate_gen_test.go \
$(P)/regexp/gen/params.gen.go &: $(P)/regexp/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/regexp --gen=true --write=true --output=$(P)/regexp

$(P)/staranalysis/gen/immediate.gen.go \
$(P)/staranalysis/gen/immediate_gen_test.go \
$(P)/staranalysis/gen/params.gen.go &: $(P)/staranalysis/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/staranalysis --gen=true --write=true --output=$(P)/staranalysis

$(P)/starcode/gen/immediate.gen.go \
$(P)/starcode/gen/immediate_gen_test.go \
$(P)/starcode/gen/params.gen.go \
$(P)/starcode/gen/sources.gen.go &: $(P)/starcode/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/starcode --gen=true --write=true --output=$(P)/starcode

$(P)/starcomplexity/gen/immediate.gen.go \
$(P)/starcomplexity/gen/immediate_gen_test.go \
$(P)/starcomplexity/gen/params.gen.go &: $(P)/starcomplexity/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/starcomplexity --gen=true --write=true --output=$(P)/starcomplexity

$(P)/starindex/gen/immediate.gen.go \
$(P)/starindex/gen/immediate_gen_test.go \
$(P)/starindex/gen/params.gen.go &: $(P)/starindex/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/starindex --gen=true --write=true --output=$(P)/starindex

$(P)/starstats/gen/immediate.gen.go \
$(P)/starstats/gen/immediate_gen_test.go \
$(P)/starstats/gen/params.gen.go &: $(P)/starstats/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/starstats --gen=true --write=true --output=$(P)/starstats

$(P)/ui/gen/immediate.gen.go \
$(P)/ui/gen/immediate_gen_test.go \
$(P)/ui/gen/params.gen.go &: $(P)/ui/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/ui --gen=true --write=true --output=$(P)/ui

$(P)/yaml/gen/immediate.gen.go \
$(P)/yaml/gen/immediate_gen_test.go \
$(P)/yaml/gen/params.gen.go &: $(P)/yaml/provider.go | star
	$(STAR) devlore actions generate --source=$(P)/yaml --gen=true --write=true --output=$(P)/yaml

generate: \
	$(P)/archive/gen/immediate.gen.go \
	$(P)/encryption/gen/immediate.gen.go \
	$(P)/file/gen/immediate.gen.go \
	$(P)/git/gen/immediate.gen.go \
	$(P)/json/gen/immediate.gen.go \
	$(P)/net/gen/immediate.gen.go \
	$(P)/pkg/gen/immediate.gen.go \
	$(P)/regexp/gen/immediate.gen.go \
	$(P)/service/gen/immediate.gen.go \
	$(P)/shell/gen/immediate.gen.go \
	$(P)/staranalysis/gen/immediate.gen.go \
	$(P)/starcode/gen/immediate.gen.go \
	$(P)/starcomplexity/gen/immediate.gen.go \
	$(P)/starindex/gen/immediate.gen.go \
	$(P)/starstats/gen/immediate.gen.go \
	$(P)/template/gen/immediate.gen.go \
	$(P)/ui/gen/immediate.gen.go \
	$(P)/yaml/gen/immediate.gen.go

build: generate
	go build $(LDFLAGS) -o build/lore ./cmd/lore
	go build $(LDFLAGS) -o build/writ ./cmd/writ
	go build $(LDFLAGS) -o build/devlore-test ./cmd/devlore-test

clean:
	rm -rf build/

test: generate
	go test ./... -timeout 60s

test-race: generate
	go test ./... -count=1 -race -timeout 120s

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
