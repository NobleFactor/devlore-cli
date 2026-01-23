# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

.PHONY: all build clean test vet lint shellcheck shfmt check dev

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

shellcheck:
	find . -name '*.sh' -o -name 'pre-commit' | xargs shellcheck

shfmt:
	find . -name '*.sh' -o -name 'pre-commit' | xargs shfmt -d -i 4 -bn -ci

check: vet lint shellcheck shfmt test

dev:
	git config core.hooksPath .githooks
	@echo "Hooks activated: .githooks/pre-commit"
