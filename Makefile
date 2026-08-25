# SPDX-License-Identifier: MIT
# Copyright (c) 2025-2026 Noble Factor. All rights reserved.

SHELL := bash
.SHELLFLAGS := -o errexit -o nounset -o pipefail -c
.ONESHELL:

# GNU make 3.82+ is a prerequisite, declared here so a wrong version says so.
#
# `.ONESHELL:` arrived in 3.82. Older make ignores the directive SILENTLY and runs each recipe line
# in its own shell, which splits multi-line `if` and `for` blocks mid-statement — the failure reads
# `bash: -c: line 1: syntax error: unexpected end of file` and names nothing useful. macOS ships
# 3.81 and always will (it is the last GPLv2 release), so `brew install make` is required there;
# `$(brew --prefix)/opt/make/libexec/gnubin` on PATH makes plain `make` the right one.
#
# The comparison is lexical, which is exact for 3.x/4.x and would misjudge a hypothetical make 10.
ifneq ($(firstword $(sort 3.82 $(MAKE_VERSION))),3.82)
$(error GNU make 3.82+ required, found $(MAKE_VERSION). On macOS: brew install make, then put \
$$(brew --prefix)/opt/make/libexec/gnubin on PATH — or invoke gmake)
endif
.SILENT:

## PARAMETERS

### VERSION

# Version for releases. Set to specific version for draft/pre-release testing.
# Examples:
#   make dist DEVLORE_VERSION=v0.1.0-draft   # Draft release for testing
#   make dist DEVLORE_VERSION=v0.1.0-alpha   # Pre-release
#   make dist                                 # Uses git describe
# --match "v*" keeps non-release tags (e.g. develop/lkg-N) out of the version:
# a slash-bearing describe result breaks the dist archive paths.
DEVLORE_VERSION ?= $(shell git describe --tags --match "v*" --always --dirty 2>/dev/null || echo "dev")

VERSION ?= $(DEVLORE_VERSION)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Freeze the three, and keep them frozen. `?=` defines RECURSIVELY expanded variables, so every
# $(shell …) above re-runs at every single reference — a different `git describe`, a different
# `date`, each time the variable is named. LDFLAGS is `:=` and captures one set; a recipe naming
# $(VERSION) again gets another.
#
# That is not hypothetical: on the windows runner the stamp check compared `8a62b17-dirty` against a
# binary reporting `8a62b17`, because codegen rewrites tracked *.gen.go files mid-build (line
# endings) and `--dirty` changed its answer between the link and the check. Simply-expanded means
# one evaluation per make run, so the stamp and everything compared against it cannot disagree.
#
# `?=` above still honors an override from the environment or the command line; those win over these
# assignments and carry no $(shell) to re-run.
VERSION := $(VERSION)
COMMIT := $(COMMIT)
BUILD_DATE := $(BUILD_DATE)

# Carry the frozen triple across the sub-make boundary, so one `make` invocation means one stamp for
# every binary it produces.
#
# `build/star: FORCE` shells out to `$(MAKE) star`, and a sub-make is a NEW run: it re-parses this
# file and the `?=` above would run a second `date` and a second `git describe`. Exporting puts the
# frozen values in the sub-make's environment, where those same `?=` guards find them already defined
# and skip the $(shell) entirely — the override path the comment above already describes.
#
# This extends the simply-expanded fix rather than reversing it. That fix stopped re-evaluation
# WITHIN a run; the sub-make is the one boundary it did not reach, which is why build/star and
# build/<host>/star could be stamped seconds apart and agree only by landing in the same one-second
# tick of `date`.
export VERSION COMMIT BUILD_DATE

# The package holding the stamped variables. Named once: `verify-ldflags` asserts that every other
# build definition in the repository names the same one.
VERSION_PACKAGE := github.com/NobleFactor/devlore-cli/pkg/application

LDFLAGS := -ldflags "-X $(VERSION_PACKAGE).Version=$(VERSION) -X $(VERSION_PACKAGE).Commit=$(COMMIT) -X $(VERSION_PACKAGE).BuildDate=$(BUILD_DATE)"

### PREFIX

# Installation prefix for `make install`. Each tool's self install places
# binaries, man pages, completions, and configs under this root.
PREFIX ?= ~/.local

### PLATFORM SELECTION

# Every platform the project supports. `dist` ships all of them, always — a release is not a
# selection.
ALL_PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

# PLATFORM is the caller's selection: `all`, or one or more <goos>/<goarch> names. It stays empty
# unless named, because build and dist want opposite defaults — you build what you are about to run,
# and you ship everything.
#
#   make build                                  # this machine
#   make build PLATFORM=linux/amd64
#   make build PLATFORM=all                     # every supported platform
#   make dist                                   # every supported platform
#   make dist  PLATFORM="linux/amd64 darwin/arm64"
#
# The inner loop should not pay to link six platforms it is not about to run: warm, the full matrix
# takes ~41s against ~13s for the host alone. Cross-compilation stays one word away rather than a tax
# on every build.
PLATFORM ?=

# Resolve the selection against a target-supplied default: $(call select,<default list>). `all`
# expands to every supported platform; empty means the caller did not choose, so the default stands.
#
# A function rather than a variable because the default differs per target, and because a command-line
# PLATFORM cannot be reassigned by a plain assignment — only `override` does that, and a bare
# `PLATFORM := $(ALL_PLATFORMS)` inside an
# ifeq is silently ignored and the loop builds a platform literally named "all". Verified by writing
# it that way first.
select = $(if $(PLATFORM),$(if $(filter all,$(PLATFORM)),$(ALL_PLATFORMS),$(PLATFORM)),$(1))

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

# Bootstrap and staleness: codegen recipes invoke $(STAR); a fresh checkout has
# no star binary, and an existing binary may predate the current source (a stale
# star silently rejects or mis-generates current inputs). FORCE delegates to the
# star target on every run — Go's build cache makes an up-to-date rebuild cost
# seconds. The LKG escape hatch is unaffected: when build/star.lkg exists,
# $(STAR) resolves to it and this rule is never consulted.
#
# This rule is also what keeps a cross-compile honest. `build` writes the PRODUCT star to
# build/star$(GOEXE), which on a non-Windows target is the same path as the host tool, so a
# cross-build leaves a target-architecture binary sitting at build/star. Because FORCE delegates
# here on every run, the next codegen rebuilds it host-side before invoking it — verified 2026-08-24
# by cross-building for linux, forcing codegen, and watching 25 star invocations succeed. #598's
# build/<goos>-<goarch>/ layout removes the path collision outright.
build/star: FORCE
	$(MAKE) star

FORCE:

## VARIABLES (static)

# This Makefile's own directory, with a trailing slash. `make -C` and recursive invocations change the
# working directory, so anything addressing a file in the repository resolves through this rather than
# through a relative path.
PROJECT_ROOT := $(dir $(realpath $(firstword $(MAKEFILE_LIST))))

# Provider source roots.
P := pkg/op/provider
SP := cmd/star/provider

# Host toolchain, for build tools only.
#
# GOOS/GOARCH are ambient, so they reach every `go` invocation — including the ones that build and
# run the generators. Cross-compiling the product would otherwise cross-compile the tools too, and
# the host would try to execute a target-architecture binary (`exec format error`). A tool is built
# and run for the HOST; only the product follows the ambient target.
#
# Pinned explicitly rather than by clearing GOOS/GOARCH, so a recipe using $(HOST_GO) says why it
# differs from a plain `go`. When the target IS the host these expand to the ambient values, so a
# native build is byte-for-byte what it was.
#
# CGO_ENABLED=0 matches what products are built with, so `star` is one binary built one way rather
# than two that differ in whether net and os/user are the cgo or the pure-Go implementations. The
# codebase is pure Go — zero `import "C"` — so nothing on the host needs cgo, and every tool comes
# out static.
HOST_GOOS := $(shell go env GOHOSTOS)
HOST_GOARCH := $(shell go env GOHOSTARCH)
HOST_GO := GOOS=$(HOST_GOOS) GOARCH=$(HOST_GOARCH) CGO_ENABLED=0 go

# The target the product is being built for, and whether that differs from the host. `go env` reports
# the ambient GOOS/GOARCH when set and the host's otherwise, so this is the effective target either
# way. CROSS is non-empty only when the produced binaries cannot run here — the one thing a recipe
# needs to know before executing something it just built.
TARGET_GOOS := $(shell go env GOOS)
TARGET_GOARCH := $(shell go env GOARCH)
CROSS := $(if $(filter $(TARGET_GOOS)/$(TARGET_GOARCH),$(HOST_GOOS)/$(HOST_GOARCH)),,cross)

# Executable suffix for the HOST, for the tools that are only ever built here. $(GOEXE) below is the
# TARGET's suffix and belongs to products.
HOST_GOEXE := $(if $(filter windows,$(HOST_GOOS)),.exe,)

# Where the host's own products land. `install` and the scenario tests want runnable binaries, which
# on this machine means this directory and no other.
HOST_DIR := build/$(HOST_GOOS)-$(HOST_GOARCH)

# The split that decides what gets cross-compiled.
#
# PRODUCTS ship, so they are built once per selected platform, into build/<goos>-<goarch>/. TOOLS
# are instruments of this checkout — they run here, on this machine, against this tree, and nowhere
# else — so they are built once for the host and stay flat at build/<name>.
#
# devlore-test is a TOOL: it is the graph test harness, and it does not ship today. Cross-compiling a
# harness produces binaries no one can run and nothing distributes.
PRODUCTS := lore star writ
TOOLS := devlore-docs devlore-index devlore-inventory devlore-test

# Products that are also run from THIS checkout. They ship, and they are used here, so each keeps a
# flat copy at build/<name> beside its per-platform product — "the binary that runs on this machine"
# gets one path that does not shift with the host triple, the guarantee $(STAR) has always relied on
# for build/star.
#
# `star` is absent deliberately: build/star is produced by the `star:` target through the
# `build/star: FORCE` bootstrap, which must run before codegen and therefore before this list is
# reached. Adding it here would build the same bytes a second time and put two recipes in charge of
# one path.
HOST_COPIES := lore writ

## TARGETS

.PHONY: all help help-short help-full build install clean test test-race cover vet vet-all lint lint-all build-all shell-lint complexity verify-ldflags check dev docs dist dist-all star star-lkg generate inventory help

##@ Help

HELP_COLWIDTH ?= 24

help: help-short ## Show brief help (alias: help-short)

help-short: ## Show brief help for annotated targets
	awk 'BEGIN {FS = ":.*##"; pad = $(HELP_COLWIDTH); print "Usage: make <target> [VAR=VALUE]"; print ""; print "Targets:"} /^[a-zA-Z0-9_.-]+:.*##/ {printf "  %-*s %s\n", pad, $$1, $$2} /^##@/ {printf "\n%s\n", substr($$0,5)}' $(MAKEFILE_LIST)
	printf '\nRun `make help-full` for the manual, including every VAR=VALUE.\n'

help-full: ## Show the manual: every target and every variable (man page)
	# The path form rather than `man -l`: BSD man (macOS) has no -l, while both BSD and GNU man treat
	# an argument containing a slash as a file to render. PROJECT_ROOT supplies that slash even when
	# make was invoked from elsewhere.
	man -P 'less -R' "$(PROJECT_ROOT)docs/devlore-cli.1"

##@ Build

all: build

star: inventory ## Build the star code generator
	$(HOST_GO) build $(LDFLAGS) -o build/star ./cmd/star

star-lkg: star ## Snapshot build/star as last-known-good (run after a green build)
	cp build/star $(STAR_LKG)

# GOEXE is ".exe" on Windows and empty elsewhere — binaries must carry it to be executable there.
GOEXE := $(shell go env GOEXE)

build: generate ## Build every product for PLATFORM (default: this machine; `all` for every platform)
	# The build comes to the machine: one host produces every platform's binaries, and installing
	# anywhere is a copy. The codebase is pure Go — zero `import "C"` — so the whole matrix
	# cross-compiles from here with GOOS/GOARCH alone, no per-platform toolchains and no build VMs.
	#
	# Widen with `make build PLATFORM=all`, or name a subset. Generators already ran once, host-side,
	# in `generate`; this loop only links.
	for platform in $(call select,$(HOST_GOOS)/$(HOST_GOARCH)); do
		os=$${platform%/*}
		arch=$${platform#*/}
		ext=""
		if [ "$$os" = "windows" ]; then ext=".exe"; fi
		mkdir -p build/$$os-$$arch
		for product in $(PRODUCTS); do
			GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build $(LDFLAGS) -o build/$$os-$$arch/$$product$$ext ./cmd/$$product
		done
	done
	# Development tools are host-only by construction — they act on this checkout, so a
	# target-architecture copy would be unrunnable and meaningless.
	for tool in $(TOOLS) $(HOST_COPIES); do
		$(HOST_GO) build $(LDFLAGS) -o build/$$tool$(HOST_GOEXE) ./cmd/$$tool
	done
	# The stamp must reach the binary. `-X` against a symbol that does not exist is NOT an error —
	# the linker ignores it and the binary reports its compiled-in default. Every release before
	# 2026-08-16 shipped that way, unnoticed, because nothing compared the two. See
	# docs/plans/version-stamping.md.
	#
	# Only the host's own copy can be executed, and it exists only when the host is in the selection —
	# which it is by default, but not under a restricting PLATFORM. A skip is announced rather than
	# silently taken: a guard that says nothing reads exactly like a passing one.
	if [ ! -x "$(HOST_DIR)/writ$(HOST_GOEXE)" ]; then
		echo "$(HOST_GOOS)/$(HOST_GOARCH) is not in PLATFORM: skipping the version-stamp check (nothing built here can run here)"
	else
		stamped="$$($(HOST_DIR)/writ$(HOST_GOEXE) version --short)"
		if [ "$$stamped" != "$(VERSION)" ]; then
			echo "ERROR: version stamp did not bind."
			echo "  build computed: $(VERSION)"
			echo "  binary reports: $$stamped"
			echo "  The -X paths in LDFLAGS name symbols that do not exist — check $(VERSION_PACKAGE)."
			exit 1
		fi
	fi

install: build ## Install lore, star, and writ via self install (PREFIX=~/.local)
	$(HOST_DIR)/lore$(HOST_GOEXE) self install $(PREFIX)
	$(HOST_DIR)/star$(HOST_GOEXE) self install $(PREFIX)
	$(HOST_DIR)/writ$(HOST_GOEXE) self install $(PREFIX)

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

test-scenario: build ## Run every scenario: the real binaries driven end to end in a sandbox
	# The writ-deploy scenario (docs/plans/writ-deploy-scenario.md) — writ's alone.
	WRIT_SCENARIO_RUN=1 go test -run TestWritDeployScenario -v -count=1 -timeout 600s ./cmd/writ
	# Self install / uninstall, once per tool. Belongs to no single command, so it lives in cmd/scenario.
	DEVLORE_SCENARIO_RUN=1 go test -run TestSelfInstallScenario -v -count=1 -timeout 600s ./cmd/scenario

cover: generate ## Report coverage (per-package inline + total); writes coverage.out. Not a gate — use test/check for that.
	go test $(if $(_TAGS),-tags '$(_TAGS)') $$(go list ./... | grep -v '/pkg/op/provider$$') -coverprofile=coverage.out -timeout 120s || true
	go tool cover -func=coverage.out | tail -1

##@ Quality

vet: ## Run go vet
	go vet ./...

# The per-GOOS sweep (docs/plans/platform-test-matrix.md, #373 phase 1b). go vet, golangci-lint,
# and the compiler all honor build constraints, so a single-GOOS run never sees the other
# platforms' _darwin.go/_windows.go/_linux.go files — the real Darwin and Windows package
# managers went unanalyzed while their stubs were checked. Same runner, one invocation per GOOS.
vet-all: ## Run go vet under every supported GOOS
	for os in linux darwin windows; do \
		echo "== go vet GOOS=$$os =="; \
		GOOS=$$os go vet ./... || exit 1; \
	done

lint-all: ## Run golangci-lint under every supported GOOS
	for os in linux darwin windows; do \
		echo "== golangci-lint GOOS=$$os =="; \
		GOOS=$$os golangci-lint run || exit 1; \
	done

build-all: generate ## Compile every package under every supported GOOS (no binaries emitted)
	for os in linux darwin windows; do \
		echo "== go build GOOS=$$os =="; \
		GOOS=$$os go build ./... || exit 1; \
	done

lint: ## Run golangci-lint
	golangci-lint run

shell-lint: ## Lint shell scripts
	.github/scripts/shell-lint.sh

complexity: ## Check cyclomatic complexity (max 20)
	echo "Checking cyclomatic complexity (max 20)..."
	$(HOST_GO) install github.com/fzipp/gocyclo/cmd/gocyclo@latest
	gocyclo_bin="$$(go env GOPATH)/bin/gocyclo"
	[ -x "$$gocyclo_bin" ] || { echo "ERROR: gocyclo not found at $$gocyclo_bin after 'go install'"; exit 1; }
	violations="$$("$$gocyclo_bin" -over 20 . | grep -v '_test.go' || true)"
	if [ -n "$$violations" ]; then
		echo "ERROR: Functions with complexity > 20:"
		echo "$$violations"
		exit 1
	fi
	echo "All functions below complexity threshold."

verify-ldflags: ## Assert every build definition stamps the same package
	# .goreleaser.yaml carries its own -X flags and is NOT the release path — nothing runs it today
	# (release.yaml runs `make dist`). It is kept correct anyway: whoever adopts goreleaser inherits
	# whatever is in that file, and a wrong symbol path there fails the way this whole defect failed,
	# silently. Agreement is the check, because the Makefile's own paths are proved to bind by `build`.
	for symbol in Version Commit BuildDate; do
		if ! grep -q -- "-X $(VERSION_PACKAGE).$$symbol=" .goreleaser.yaml; then
			echo "ERROR: .goreleaser.yaml does not stamp $(VERSION_PACKAGE).$$symbol"
			echo "  Both build definitions must name the same package, or one of them stamps nothing."
			exit 1
		fi
	done
	echo "Build definitions agree on $(VERSION_PACKAGE)."

check: vet lint shell-lint complexity verify-ldflags test ## Run all quality checks

##@ Development

dev: ## Activate git hooks
	git config core.hooksPath .githooks
	echo "Hooks activated: .githooks/pre-commit"

docs: build ## Generate CLI documentation
	build/devlore-docs$(HOST_GOEXE) --output-dir=docs/cli --version=$(VERSION)

##@ Distribution

dist: dist-all checksums ## Build distribution archives with checksums

dist-all: build ## Build distribution archives for PLATFORM (default: every supported platform)
	# Depends on `build` for its stamp check, not for its binaries. This is the path that ships
	# (.github/workflows/release.yaml runs `make dist`), it cross-compiles with the same LDFLAGS and
	# the same VERSION, and it cannot run what it produces for other platforms. `build` proves on the
	# host that those flags bind to real symbols — which is the failure that shipped unnoticed until
	# 2026-08-16. See docs/plans/version-stamping.md.
	mkdir -p dist
	for platform in $(call select,$(ALL_PLATFORMS)); do
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

# Each grouped target (&:) fires one star invocation that produces all gen files. | $(STAR)
# Generation runs only when provider.go is newer than the gen outputs.
#
# access=both     → receiver_type.gen_test + action.gen_test + module.gen_test + provider
# access=planned  → receiver_type.gen_test + action.gen_test + provider
# access=immediate → receiver_type.gen_test + module.gen_test + provider
#
# Every member listed must be one the generator actually emits: under GNU Make 4.4.1's grouped-target
# semantics a member that can never appear leaves the whole group permanently stale, so the recipe reruns
# on every build and cross-compiles die on the exec-format mismatch. node_builder.gen_test was retired
# with NodeBuilder (phase 5) and removed from these groups on 2026-08-24 (#572).

# --- access=both providers ---

$(P)/json/action_names.gen.go \
$(P)/json/gen/receiver_type.gen_test.go \
$(P)/json/gen/action.gen_test.go \
$(P)/json/gen/module.gen_test.go \
$(P)/json/gen/provider.gen.go &: $(P)/json/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/json --gen=true --write=true --output=$(P)/json

$(P)/platform/action_names.gen.go \
$(P)/platform/gen/receiver_type.gen_test.go \
$(P)/platform/gen/action.gen_test.go \
$(P)/platform/gen/module.gen_test.go \
$(P)/platform/gen/provider.gen.go &: $(P)/platform/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/platform --gen=true --write=true --output=$(P)/platform

$(P)/regex/action_names.gen.go \
$(P)/regex/gen/receiver_type.gen_test.go \
$(P)/regex/gen/action.gen_test.go \
$(P)/regex/gen/module.gen_test.go \
$(P)/regex/gen/provider.gen.go &: $(P)/regex/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/regex --gen=true --write=true --output=$(P)/regex

$(P)/template/action_names.gen.go \
$(P)/template/gen/receiver_type.gen_test.go \
$(P)/template/gen/action.gen_test.go \
$(P)/template/gen/module.gen_test.go \
$(P)/template/gen/provider.gen.go &: $(P)/template/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/template --gen=true --write=true --output=$(P)/template

$(P)/yaml/action_names.gen.go \
$(P)/yaml/gen/receiver_type.gen_test.go \
$(P)/yaml/gen/action.gen_test.go \
$(P)/yaml/gen/module.gen_test.go \
$(P)/yaml/gen/provider.gen.go &: $(P)/yaml/provider.go $(P)/yaml/resource.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/yaml --gen=true --write=true --output=$(P)/yaml

# --- access=planned providers ---

$(P)/appnet/action_names.gen.go \
$(P)/appnet/gen/receiver_type.gen_test.go \
$(P)/appnet/gen/action.gen_test.go \
$(P)/appnet/gen/provider.gen.go \
$(P)/appnet/gen/resource.gen.go &: $(P)/appnet/provider.go $(P)/appnet/resource.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/appnet --gen=true --write=true --output=$(P)/appnet

$(P)/archive/action_names.gen.go \
$(P)/archive/gen/receiver_type.gen_test.go \
$(P)/archive/gen/action.gen_test.go \
$(P)/archive/gen/provider.gen.go &: $(P)/archive/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/archive --gen=true --write=true --output=$(P)/archive

$(P)/encryption/action_names.gen.go \
$(P)/encryption/gen/receiver_type.gen_test.go \
$(P)/encryption/gen/action.gen_test.go \
$(P)/encryption/gen/provider.gen.go &: $(P)/encryption/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/encryption --gen=true --write=true --output=$(P)/encryption

$(P)/file/action_names.gen.go \
$(P)/file/gen/receiver_type.gen_test.go \
$(P)/file/gen/action.gen_test.go \
$(P)/file/gen/module.gen_test.go \
$(P)/file/gen/provider.gen.go &: $(P)/file/provider.go $(P)/file/resource.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/file --gen=true --write=true --output=$(P)/file

$(P)/git/action_names.gen.go \
$(P)/git/gen/receiver_type.gen_test.go \
$(P)/git/gen/action.gen_test.go \
$(P)/git/gen/provider.gen.go \
$(P)/git/gen/resource.gen.go &: $(P)/git/provider.go $(P)/git/resource.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/git --gen=true --write=true --output=$(P)/git

$(P)/pkg/action_names.gen.go \
$(P)/pkg/gen/receiver_type.gen_test.go \
$(P)/pkg/gen/action.gen_test.go \
$(P)/pkg/gen/provider.gen.go \
$(P)/pkg/gen/resource.gen.go &: $(P)/pkg/provider.go $(P)/pkg/resource.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/pkg --gen=true --write=true --output=$(P)/pkg

$(P)/service/action_names.gen.go \
$(P)/service/gen/receiver_type.gen_test.go \
$(P)/service/gen/action.gen_test.go \
$(P)/service/gen/provider.gen.go \
$(P)/service/gen/resource.gen.go &: $(P)/service/provider.go $(P)/service/resource.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/service --gen=true --write=true --output=$(P)/service

$(P)/shell/action_names.gen.go \
$(P)/shell/gen/receiver_type.gen_test.go \
$(P)/shell/gen/action.gen_test.go \
$(P)/shell/gen/provider.gen.go &: $(P)/shell/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/shell --gen=true --write=true --output=$(P)/shell

$(P)/powershell/action_names.gen.go \
$(P)/powershell/gen/receiver_type.gen_test.go \
$(P)/powershell/gen/action.gen_test.go \
$(P)/powershell/gen/provider.gen.go &: $(P)/powershell/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/powershell --gen=true --write=true --output=$(P)/powershell

$(P)/function/action_names.gen.go \
$(P)/function/gen/receiver_type.gen_test.go \
$(P)/function/gen/action.gen_test.go \
$(P)/function/gen/provider.gen.go \
$(P)/function/gen/resource.gen.go &: $(P)/function/provider.go $(P)/function/resource.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/function --gen=true --write=true --output=$(P)/function

$(P)/flow/action_names.gen.go \
$(P)/flow/gen/receiver_type.gen_test.go \
$(P)/flow/gen/action.gen_test.go \
$(P)/flow/gen/provider.gen.go &: $(P)/flow/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/flow --gen=true --write=true --output=$(P)/flow

# --- access=immediate providers ---

$(P)/plan/gen/receiver_type.gen_test.go \
$(P)/plan/gen/module.gen_test.go \
$(P)/plan/gen/provider.gen.go &: $(P)/plan/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/plan --gen=true --write=true --output=$(P)/plan

# --- star-specific providers (cmd/star/provider, access=immediate) ---

$(SP)/staranalysis/gen/receiver_type.gen_test.go \
$(SP)/staranalysis/gen/module.gen_test.go \
$(SP)/staranalysis/gen/provider.gen.go &: $(SP)/staranalysis/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(SP)/staranalysis --gen=true --write=true --output=$(SP)/staranalysis

$(SP)/starcode/gen/receiver_type.gen_test.go \
$(SP)/starcode/gen/module.gen_test.go \
$(SP)/starcode/gen/provider.gen.go &: $(SP)/starcode/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(SP)/starcode --gen=true --write=true --output=$(SP)/starcode

$(SP)/starcomplexity/gen/receiver_type.gen_test.go \
$(SP)/starcomplexity/gen/module.gen_test.go \
$(SP)/starcomplexity/gen/provider.gen.go &: $(SP)/starcomplexity/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(SP)/starcomplexity --gen=true --write=true --output=$(SP)/starcomplexity

$(SP)/starindex/gen/receiver_type.gen_test.go \
$(SP)/starindex/gen/module.gen_test.go \
$(SP)/starindex/gen/provider.gen.go &: $(SP)/starindex/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(SP)/starindex --gen=true --write=true --output=$(SP)/starindex

$(SP)/starstats/gen/receiver_type.gen_test.go \
$(SP)/starstats/gen/module.gen_test.go \
$(SP)/starstats/gen/provider.gen.go &: $(SP)/starstats/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(SP)/starstats --gen=true --write=true --output=$(SP)/starstats

$(SP)/commands/gen/receiver_type.gen_test.go \
$(SP)/commands/gen/module.gen_test.go \
$(SP)/commands/gen/provider.gen.go &: $(SP)/commands/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(SP)/commands --gen=true --write=true --output=$(SP)/commands

$(SP)/config/gen/receiver_type.gen_test.go \
$(SP)/config/gen/module.gen_test.go \
$(SP)/config/gen/provider.gen.go &: $(SP)/config/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(SP)/config --gen=true --write=true --output=$(SP)/config

$(SP)/goast/gen/receiver_type.gen_test.go \
$(SP)/goast/gen/module.gen_test.go \
$(SP)/goast/gen/provider.gen.go &: $(SP)/goast/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(SP)/goast --gen=true --write=true --output=$(SP)/goast

$(SP)/lint/gen/receiver_type.gen_test.go \
$(SP)/lint/gen/module.gen_test.go \
$(SP)/lint/gen/provider.gen.go &: $(SP)/lint/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(SP)/lint --gen=true --write=true --output=$(SP)/lint

$(SP)/setup/gen/receiver_type.gen_test.go \
$(SP)/setup/gen/module.gen_test.go \
$(SP)/setup/gen/provider.gen.go &: $(SP)/setup/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(SP)/setup --gen=true --write=true --output=$(SP)/setup

$(SP)/shellcheck/gen/receiver_type.gen_test.go \
$(SP)/shellcheck/gen/module.gen_test.go \
$(SP)/shellcheck/gen/provider.gen.go &: $(SP)/shellcheck/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(SP)/shellcheck --gen=true --write=true --output=$(SP)/shellcheck

$(P)/ui/gen/receiver_type.gen_test.go \
$(P)/ui/gen/module.gen_test.go \
$(P)/ui/gen/provider.gen.go &: $(P)/ui/provider.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/ui --gen=true --write=true --output=$(P)/ui

# --- resource-only packages ---

$(P)/mem/gen/resource.gen.go: $(P)/mem/resource.go | $(STAR)
	$(STAR) devlore actions generate --source=$(P)/mem --gen=true --write=true --output=$(P)/mem

NEW_OP_INVENTORY := \
	$(P)/appnet/gen/provider.gen.go \
	$(P)/archive/gen/provider.gen.go \
	$(P)/encryption/gen/provider.gen.go \
	$(P)/file/gen/provider.gen.go \
	$(P)/flow/gen/provider.gen.go \
	$(P)/function/gen/provider.gen.go \
	$(P)/git/gen/provider.gen.go \
	$(P)/json/gen/provider.gen.go \
	$(P)/mem/gen/resource.gen.go \
	$(P)/plan/gen/provider.gen.go \
	$(P)/platform/gen/provider.gen.go \
	$(P)/pkg/gen/provider.gen.go \
	$(P)/powershell/gen/provider.gen.go \
	$(P)/regex/gen/provider.gen.go \
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
	$(HOST_GO) run ./cmd/devlore-inventory pkg/op/inventory/inventory.gen.go github.com/NobleFactor/devlore-cli pkg/op
	$(HOST_GO) run ./cmd/devlore-inventory cmd/star/inventory/inventory.gen.go github.com/NobleFactor/devlore-cli cmd/star

generate: $(NEW_OP_INVENTORY) inventory ## Run all code generation
