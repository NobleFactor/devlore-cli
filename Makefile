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

# PLATFORM is the caller's selection: the keyword `host`, the keyword `all`, or one or more
# <goos>/<goarch> names. It stays empty unless named, because build and dist want opposite defaults
# — you build what you are about to run, and you ship everything.
#
#   make build                                  # host — build's default
#   make build PLATFORM=host                    # the same, said out loud
#   make build PLATFORM=linux/amd64
#   make build PLATFORM=all                     # every supported platform
#   make dist                                   # every supported platform — dist's default
#   make dist  PLATFORM="linux/amd64 darwin/arm64"
#
# `host` rather than `current` or `local`: this file already names the machine it runs on six ways
# (HOST_GOOS, HOST_GOARCH, HOST_GO, HOST_GOEXE, HOST_DIR, HOST_COPIES), every one of them from Go's
# own GOHOSTOS/GOHOSTARCH, and CROSS is defined as target ≠ host. `PLATFORM=host` therefore reads as
# precisely what it does — do not cross-compile — and a third synonym would only blur that.
#
# PLATFORM must stay empty by default. A non-empty default is never overridden by a target, so
# dist's default would become unreachable and releases would go host-only without announcing it.
# The defaults live at the call sites, as keywords, where each target can state its own.
#
# The inner loop should not pay to link six platforms it is not about to run: warm, the full matrix
# takes ~41s against ~13s for the host alone. Cross-compilation stays one word away rather than a tax
# on every build.
PLATFORM ?=

# Expand the two keywords; anything else is already a list of <goos>/<goarch> names.
expand = $(if $(filter all,$(1)),$(ALL_PLATFORMS),$(if $(filter host,$(1)),$(HOST_GOOS)/$(HOST_GOARCH),$(1)))

# Resolve the selection against a target-supplied default: $(call select,host) or $(call select,all).
# The caller's PLATFORM wins when named; otherwise the target's own default stands. Both go through
# expand, so `host` and `all` mean the same thing whether typed or defaulted.
#
# A function rather than a variable because the default differs per target, and because a command-line
# PLATFORM cannot be reassigned by a plain assignment — only `override` does that, and a bare
# `PLATFORM := $(ALL_PLATFORMS)` inside an
# ifeq is silently ignored and the loop builds a platform literally named "all". Verified by writing
# it that way first.
select = $(call expand,$(if $(PLATFORM),$(PLATFORM),$(1)))

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

.PHONY: all help help-short help-full build install clean test test-race cover vet vet-all lint lint-all build-all shell-lint complexity verify-ldflags check dev docs dist dist-all star star-lkg generate regenerate inventory help print-golangci-lint-version

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
	for platform in $(call select,host); do
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

# Whether this toolchain can instrument for the race detector. Asked of the toolchain rather than
# transcribed from Go's own table (internal/platform.RaceDetectorSupported), which gains ports over
# time and would drift here. Two distinct failures hide behind a rejected -race, and only one is ours
# to fix:
#
#   "-race requires cgo"                      CGO_ENABLED=0, which this probe sets
#   "-race is not supported on <goos>/<arch>"  no ThreadSanitizer runtime exists for that target
#
# The probe therefore runs with cgo ON, so the second cannot masquerade as the first and silently
# cost race coverage on a platform that has it. `go list` compiles nothing -- the flag is rejected
# during validation -- so this costs ~15ms.
RACE_SUPPORTED := $(shell CGO_ENABLED=1 go list -race errors >/dev/null 2>&1 && echo 1)

test-race: generate ## Run tests with race detector (TAGS=all|integration|e2e|"", default: all)
	if [ -z "$(RACE_SUPPORTED)" ]; then
		message="race detector unavailable on $$(go env GOOS)/$$(go env GOARCH) -- tests run UNINSTRUMENTED"
		# GitHub renders ::warning:: as an annotation on the run summary, so an uninstrumented leg is
		# visible without opening the log. GITHUB_ACTIONS is unset locally and .SHELLFLAGS carries
		# nounset, hence the :- default.
		if [ -n "$${GITHUB_ACTIONS:-}" ]; then
			echo "::warning title=Race detector unavailable::$$message"
		fi
		echo
		echo "WARNING: $$message"
		echo
		race_flags=""
		export CGO_ENABLED=0
	else
		race_flags="-race"
		export CGO_ENABLED=1
	fi
	go test $(if $(_TAGS),-tags '$(_TAGS)') $$(go list ./... | grep -v '/pkg/op/provider$$') -count=1 $$race_flags -timeout 120s

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

lint-all: golangci-lint-pinned ## Run golangci-lint under every supported GOOS
	for os in linux darwin windows; do \
		echo "== golangci-lint GOOS=$$os =="; \
		GOOS=$$os golangci-lint run || exit 1; \
	done

build-all: generate ## Compile every package under every supported GOOS (no binaries emitted)
	for os in linux darwin windows; do \
		echo "== go build GOOS=$$os =="; \
		GOOS=$$os go build ./... || exit 1; \
	done

# golangci-lint is pinned because its embedded go/types decides what staticcheck can infer, so an
# unpinned local binary and a pinned CI binary disagree about the same source. #669 was exactly that:
# v2.13.1 found four real SA4023 defects that CI's v2.12.2 did not. This variable is the one place the
# version is written: CI reads it through `make print-golangci-lint-version`, so a bump is one line.
# v2.13.2 since 2026-09-04 -- the developer machine runs it and the whole tree is clean on it.
GOLANGCI_LINT_VERSION ?= v2.13.2

print-golangci-lint-version: ## Print the pinned golangci-lint version, for CI to install the same one
	@echo $(GOLANGCI_LINT_VERSION)

golangci-lint-pinned: ## Verify the pinned golangci-lint is the one on PATH
	installed="$$(golangci-lint version --short 2>/dev/null || echo none)"
	# `version --short` prints 2.13.1 while the pin is written v2.13.1 — the form the install URL
	# and the install command both need. Strip a leading v from each side rather than from the
	# variable, so the comparison survives golangci-lint changing which form it reports.
	expected="$(GOLANGCI_LINT_VERSION)"
	if [ "$${installed#v}" != "$${expected#v}" ]; then
		echo "ERROR: golangci-lint $$installed on PATH, expected $(GOLANGCI_LINT_VERSION)."
		echo "  Install: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh | \\"
		echo "             sh -s -- -b \"\$$(go env GOPATH)/bin\" $(GOLANGCI_LINT_VERSION)"
		exit 1
	fi

# Routed through star, not a bare `golangci-lint run`, because that is the gate CI carries
# (.github/workflows/ci.yaml "Lint Go"). star execs the same pinned binary against the same
# .golangci.yaml; what it adds is a JSON report written to its own file rather than parsed off a
# shared stdout -- the 2026-08-04 silent-pass incident, where a polluted stream made every lint run
# appear to succeed. Running anything else locally means the local gate and CI can disagree.
lint: golangci-lint-pinned | $(STAR) ## Run golangci-lint through star, as CI does
	$(STAR) lint go ./...

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

dist-all: ## Build distribution archives for PLATFORM (default: every supported platform)
	# The stamp proof is NOT optional and NOT subject to PLATFORM.
	#
	# `build` proves on the host that the -X flags bind to real symbols — the failure that shipped
	# unnoticed until 2026-08-16. A plain `dist-all: build` prerequisite lost that proof the moment
	# `build` learned to skip the check when PLATFORM excludes the host: `make dist PLATFORM=linux/amd64`
	# would then ship archives with nothing having verified the stamp. Forcing a host build here restores
	# the invariant regardless of what the caller selected.
	$(MAKE) build PLATFORM=$(HOST_GOOS)/$(HOST_GOARCH)
	# Depends on `build` for its stamp check, not for its binaries. This is the path that ships
	# (.github/workflows/release.yaml runs `make dist`), it cross-compiles with the same LDFLAGS and
	# the same VERSION, and it cannot run what it produces for other platforms. `build` proves on the
	# host that those flags bind to real symbols — which is the failure that shipped unnoticed until
	# 2026-08-16. See docs/plans/version-stamping.md.
	mkdir -p dist
	for platform in $(call select,all); do
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
	$(STAR) devlore actions generate $(P)/json

$(P)/platform/action_names.gen.go \
$(P)/platform/gen/receiver_type.gen_test.go \
$(P)/platform/gen/action.gen_test.go \
$(P)/platform/gen/module.gen_test.go \
$(P)/platform/gen/provider.gen.go &: $(P)/platform/provider.go | $(STAR)
	$(STAR) devlore actions generate $(P)/platform

$(P)/regex/action_names.gen.go \
$(P)/regex/gen/receiver_type.gen_test.go \
$(P)/regex/gen/action.gen_test.go \
$(P)/regex/gen/module.gen_test.go \
$(P)/regex/gen/provider.gen.go &: $(P)/regex/provider.go | $(STAR)
	$(STAR) devlore actions generate $(P)/regex

$(P)/template/action_names.gen.go \
$(P)/template/gen/receiver_type.gen_test.go \
$(P)/template/gen/action.gen_test.go \
$(P)/template/gen/module.gen_test.go \
$(P)/template/gen/provider.gen.go &: $(P)/template/provider.go | $(STAR)
	$(STAR) devlore actions generate $(P)/template

$(P)/yaml/action_names.gen.go \
$(P)/yaml/gen/receiver_type.gen_test.go \
$(P)/yaml/gen/action.gen_test.go \
$(P)/yaml/gen/module.gen_test.go \
$(P)/yaml/gen/provider.gen.go &: $(P)/yaml/provider.go $(P)/yaml/resource.go | $(STAR)
	$(STAR) devlore actions generate $(P)/yaml

# --- access=planned providers ---

$(P)/appnet/action_names.gen.go \
$(P)/appnet/gen/receiver_type.gen_test.go \
$(P)/appnet/gen/action.gen_test.go \
$(P)/appnet/gen/provider.gen.go \
$(P)/appnet/gen/resource.gen.go &: $(P)/appnet/provider.go $(P)/appnet/resource.go | $(STAR)
	$(STAR) devlore actions generate $(P)/appnet

$(P)/archive/action_names.gen.go \
$(P)/archive/gen/receiver_type.gen_test.go \
$(P)/archive/gen/action.gen_test.go \
$(P)/archive/gen/provider.gen.go &: $(P)/archive/provider.go | $(STAR)
	$(STAR) devlore actions generate $(P)/archive

$(P)/encryption/action_names.gen.go \
$(P)/encryption/gen/receiver_type.gen_test.go \
$(P)/encryption/gen/action.gen_test.go \
$(P)/encryption/gen/provider.gen.go &: $(P)/encryption/provider.go | $(STAR)
	$(STAR) devlore actions generate $(P)/encryption

$(P)/file/action_names.gen.go \
$(P)/file/gen/receiver_type.gen_test.go \
$(P)/file/gen/action.gen_test.go \
$(P)/file/gen/module.gen_test.go \
$(P)/file/gen/provider.gen.go &: $(P)/file/provider.go $(P)/file/resource.go | $(STAR)
	$(STAR) devlore actions generate $(P)/file

$(P)/git/action_names.gen.go \
$(P)/git/gen/receiver_type.gen_test.go \
$(P)/git/gen/action.gen_test.go \
$(P)/git/gen/provider.gen.go \
$(P)/git/gen/resource.gen.go &: $(P)/git/provider.go $(P)/git/resource.go | $(STAR)
	$(STAR) devlore actions generate $(P)/git

$(P)/pkg/action_names.gen.go \
$(P)/pkg/gen/receiver_type.gen_test.go \
$(P)/pkg/gen/action.gen_test.go \
$(P)/pkg/gen/provider.gen.go \
$(P)/pkg/gen/resource.gen.go &: $(P)/pkg/provider.go $(P)/pkg/resource.go | $(STAR)
	$(STAR) devlore actions generate $(P)/pkg

$(P)/service/action_names.gen.go \
$(P)/service/gen/receiver_type.gen_test.go \
$(P)/service/gen/action.gen_test.go \
$(P)/service/gen/provider.gen.go \
$(P)/service/gen/resource.gen.go &: $(P)/service/provider.go $(P)/service/resource.go | $(STAR)
	$(STAR) devlore actions generate $(P)/service

$(P)/shell/action_names.gen.go \
$(P)/shell/gen/receiver_type.gen_test.go \
$(P)/shell/gen/action.gen_test.go \
$(P)/shell/gen/provider.gen.go &: $(P)/shell/provider.go | $(STAR)
	$(STAR) devlore actions generate $(P)/shell

$(P)/powershell/action_names.gen.go \
$(P)/powershell/gen/receiver_type.gen_test.go \
$(P)/powershell/gen/action.gen_test.go \
$(P)/powershell/gen/provider.gen.go &: $(P)/powershell/provider.go | $(STAR)
	$(STAR) devlore actions generate $(P)/powershell

$(P)/function/action_names.gen.go \
$(P)/function/gen/receiver_type.gen_test.go \
$(P)/function/gen/action.gen_test.go \
$(P)/function/gen/provider.gen.go \
$(P)/function/gen/resource.gen.go &: $(P)/function/provider.go $(P)/function/resource.go | $(STAR)
	$(STAR) devlore actions generate $(P)/function

$(P)/flow/action_names.gen.go \
$(P)/flow/gen/receiver_type.gen_test.go \
$(P)/flow/gen/action.gen_test.go \
$(P)/flow/gen/provider.gen.go &: $(P)/flow/provider.go | $(STAR)
	$(STAR) devlore actions generate $(P)/flow

# --- access=immediate providers ---

$(P)/plan/gen/receiver_type.gen_test.go \
$(P)/plan/gen/module.gen_test.go \
$(P)/plan/gen/provider.gen.go &: $(P)/plan/provider.go | $(STAR)
	$(STAR) devlore actions generate $(P)/plan

# --- star-specific providers (cmd/star/provider, access=immediate) ---

$(SP)/staranalysis/gen/receiver_type.gen_test.go \
$(SP)/staranalysis/gen/module.gen_test.go \
$(SP)/staranalysis/gen/provider.gen.go &: $(SP)/staranalysis/provider.go | $(STAR)
	$(STAR) devlore actions generate $(SP)/staranalysis

$(SP)/starcode/gen/receiver_type.gen_test.go \
$(SP)/starcode/gen/module.gen_test.go \
$(SP)/starcode/gen/provider.gen.go &: $(SP)/starcode/provider.go | $(STAR)
	$(STAR) devlore actions generate $(SP)/starcode

$(SP)/starcomplexity/gen/receiver_type.gen_test.go \
$(SP)/starcomplexity/gen/module.gen_test.go \
$(SP)/starcomplexity/gen/provider.gen.go &: $(SP)/starcomplexity/provider.go | $(STAR)
	$(STAR) devlore actions generate $(SP)/starcomplexity

$(SP)/starindex/gen/receiver_type.gen_test.go \
$(SP)/starindex/gen/module.gen_test.go \
$(SP)/starindex/gen/provider.gen.go &: $(SP)/starindex/provider.go | $(STAR)
	$(STAR) devlore actions generate $(SP)/starindex

$(SP)/starstats/gen/receiver_type.gen_test.go \
$(SP)/starstats/gen/module.gen_test.go \
$(SP)/starstats/gen/provider.gen.go &: $(SP)/starstats/provider.go | $(STAR)
	$(STAR) devlore actions generate $(SP)/starstats

$(SP)/commands/gen/receiver_type.gen_test.go \
$(SP)/commands/gen/module.gen_test.go \
$(SP)/commands/gen/provider.gen.go &: $(SP)/commands/provider.go | $(STAR)
	$(STAR) devlore actions generate $(SP)/commands

$(SP)/config/gen/receiver_type.gen_test.go \
$(SP)/config/gen/module.gen_test.go \
$(SP)/config/gen/provider.gen.go &: $(SP)/config/provider.go | $(STAR)
	$(STAR) devlore actions generate $(SP)/config

$(SP)/goast/gen/receiver_type.gen_test.go \
$(SP)/goast/gen/module.gen_test.go \
$(SP)/goast/gen/provider.gen.go &: $(SP)/goast/provider.go | $(STAR)
	$(STAR) devlore actions generate $(SP)/goast

$(SP)/lint/gen/receiver_type.gen_test.go \
$(SP)/lint/gen/module.gen_test.go \
$(SP)/lint/gen/provider.gen.go &: $(SP)/lint/provider.go | $(STAR)
	$(STAR) devlore actions generate $(SP)/lint

$(SP)/setup/gen/receiver_type.gen_test.go \
$(SP)/setup/gen/module.gen_test.go \
$(SP)/setup/gen/provider.gen.go &: $(SP)/setup/provider.go | $(STAR)
	$(STAR) devlore actions generate $(SP)/setup

$(SP)/shellcheck/gen/receiver_type.gen_test.go \
$(SP)/shellcheck/gen/module.gen_test.go \
$(SP)/shellcheck/gen/provider.gen.go &: $(SP)/shellcheck/provider.go | $(STAR)
	$(STAR) devlore actions generate $(SP)/shellcheck

$(P)/ui/gen/receiver_type.gen_test.go \
$(P)/ui/gen/module.gen_test.go \
$(P)/ui/gen/provider.gen.go &: $(P)/ui/provider.go | $(STAR)
	$(STAR) devlore actions generate $(P)/ui

# --- resource-only packages ---

$(P)/mem/gen/resource.gen.go: $(P)/mem/resource.go | $(STAR)
	$(STAR) devlore actions generate $(P)/mem

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

# Codegen is mtime-gated, and on a fresh clone every file is stamped at checkout -- so whether a
# provider looks stale is decided by write order rather than by its source. Measured on CI run
# 33024633546, one commit across six jobs: 30 providers regenerated on one, 1 on two, and 0 on three.
# Every generator validation rides along with that, so which defects get caught is luck (#702).
#
# Touching the SOURCES rather than deleting the outputs, because the outputs are also inputs: the
# generated files hold the op.Announce*() call sites that devlore-inventory scans, inventory is a
# prerequisite of the star binary, and `build/star: FORCE` rebuilds star on every run. Deleting the
# generated files therefore breaks the bootstrap that would regenerate them -- verified 2026-08-27 by
# doing exactly that and watching inventory find 8 packages instead of 19, then fail with
# `no op.Announce*() call sites found`.
#
# `make -B generate` would also force the rules, and forces far more than the question needs: it
# propagates into the `build/star: FORCE` sub-make and rebuilds targets unrelated to codegen.
regenerate: ## Regenerate every generated file from scratch, ignoring mtimes
	find $(P) $(SP) -maxdepth 2 \( -name provider.go -o -name resource.go \) -exec touch {} +
	$(MAKE) generate
