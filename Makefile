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

.PHONY: all build clean test vet lint shell-lint check dev docs dist dist-all

all: build

build:
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

check: vet lint shell-lint test

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
