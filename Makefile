# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

.PHONY: all build lore writ clean test docs

all: build

build: lore writ

lore:
	go build $(LDFLAGS) -o bin/lore ./cmd/lore

writ:
	go build $(LDFLAGS) -o bin/writ ./cmd/writ

clean:
	rm -rf bin/
	rm -rf docs/

test:
	go test ./...

# Generate markdown documentation from Cobra commands
docs: build
	mkdir -p docs/lore docs/writ
	./bin/lore man --install --path docs/lore
	./bin/writ man --install --path docs/writ

# Install binaries to GOBIN or ~/.local/bin
install: build
	@mkdir -p $(or $(GOBIN),$(HOME)/.local/bin)
	cp bin/lore $(or $(GOBIN),$(HOME)/.local/bin)/
	cp bin/writ $(or $(GOBIN),$(HOME)/.local/bin)/
	@echo "Installed to $(or $(GOBIN),$(HOME)/.local/bin)"

# Install completions to XDG directories
install-completions: build
	./bin/lore completion bash --install
	./bin/lore completion zsh --install
	./bin/writ completion bash --install
	./bin/writ completion zsh --install

# Install man pages to XDG directory
install-man: build
	./bin/lore man --install
	./bin/writ man --install

# Full install: binaries, completions, man pages
install-all: install install-completions install-man
