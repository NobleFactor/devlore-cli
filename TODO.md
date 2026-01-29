# Implementation TODO

**Scope:** Implementation gaps, product documentation, code issues
**Sister file:** [noblefactor/TODO.md](https://github.com/NobleFactor/noblefactor/blob/main/TODO.md) — Design docs, business strategy, process items

> **RULE:** All implementation and documentation work MUST be checked against the design docs in `noblefactor/devlore/`. Goal: Keep implementation and design in sync. Find and resolve inconsistencies, redundancies, and gaps as we progress.

This file tracks documentation gaps, incomplete features, and pending decisions identified during a comprehensive review on 2025-01-25. Each item includes sufficient context for remediation without requiring additional research.

---

## 1. Missing Documentation

### 1.1 Troubleshooting Guide

**Status:** Not started
**Priority:** High
**Location:** Should be created at `docs/guides/troubleshooting.md`

**Context:** No standalone troubleshooting document exists. Users encountering errors have no guidance.

**Should cover:**
- `lore deploy` failures: permission errors, network failures, package not found
- `writ add` conflicts: symlink already exists, file conflicts between layers
- Broken symlinks: detection and repair
- Rollback procedures: how to undo a bad deployment
- State file corruption: detection and recovery
- Debugging techniques: verbose output flags, log locations

**Cross-references needed:**
- `docs/guides/lore/deploy-packages.md` mentions receipts but not failure recovery
- `docs/guides/writ/manage-environments.md` mentions conflicts but not resolution

---

### 1.2 Writ + Lore Integration Workflow

**Status:** Not started
**Priority:** High
**Location:** Should be created at `docs/guides/integration.md` or added to getting-started.md

**Context:** Both tools document their side of the integration separately:
- `docs/guides/writ/manage-environments.md` (lines 142-159): shows `writ add` calling `lore deploy`
- `docs/guides/lore/deploy-packages.md` (lines 142-159): shows same flow from lore's perspective

**Should cover:**
- Complete end-to-end workflow: writ add → lore deploy → verify → writ status
- When to use lore standalone vs integrated with writ
- Error propagation: what happens if lore fails during writ add?
- Partial success handling: some packages deployed, some failed
- Manual intervention points

**Gap:** Users cannot understand the full system without reading both tool guides and mentally merging them.

---

### 1.3 Migration Guide from Other Tools

**Status:** Not started
**Priority:** Medium
**Location:** Should be created at `docs/guides/migration.md`

**Context:** No documentation for users coming from:
- **GNU Stow**: symlink-based dotfiles manager
- **chezmoi**: template-based dotfiles manager with state tracking
- **dotbot**: YAML-configured dotfiles bootstrapper
- **Manual scripts**: users with custom install.sh scripts

**Should cover:**
- Conceptual mapping: how stow concepts map to writ concepts
- Import procedures: how to convert existing configuration repo to writ structure
- Coexistence: can writ manage some files while stow manages others?
- One-time migration scripts or commands if applicable

---

### 1.4 Receipt Format and Schema

**Status:** Not started
**Priority:** Medium
**Location:** Should be added to `docs/guides/lore/deploy-packages.md` or new `docs/reference/receipts.md`

**Context:** Receipts are mentioned in multiple places but never defined:
- `docs/guides/lore/deploy-packages.md` (lines 95-104): "Every deployment produces a receipt"
- Line 101: `--receipt=~/deployments/workstation.yaml`
- Line 104: "Receipts are stored in `~/.local/state/lore/receipts/` by default"

**Should cover:**
- YAML schema for receipt files
- Fields: package name, version, timestamp, phase results, platform, features enabled
- How receipts interact with `lore reconcile`, `lore upgrade`, `lore decommission`
- Backup and archiving guidance
- Machine portability: can receipts be shared across machines?

---

### 1.5 Audit Log API and Storage

**Status:** Not started
**Priority:** Medium
**Location:** Should be added to `docs/guides/lore/registry.md` or new `docs/reference/audit.md`

**Context:** Audit features documented sparsely in `docs/guides/lore/registry.md` (lines 106-133):
- Commands shown: `lore audit`, `lore audit --since 7d`, `lore audit --package docker`
- Event types listed in table (lines 117-123): pmm.fetch, pmm.verify, pmm.install, etc.

**Should cover:**
- Complete event type catalog with descriptions
- Event schema (JSON/YAML structure)
- Storage location and format
- Retention configuration
- Querying and filtering syntax
- Export formats for compliance reporting
- Integration with external logging systems (syslog, etc.)

---

### 1.6 State File Management and Recovery

**Status:** Not started
**Priority:** Medium
**Location:** Should be added to `docs/guides/writ/repositories.md` or new `docs/reference/state.md`

**Context:** State files are implied throughout writ documentation but never explained:
- How writ tracks which files are managed
- Where state is stored
- State file format/schema

**Should cover:**
- State file locations (per-repo, global)
- Schema and format
- Synchronization across machines (git-based? manual?)
- Corruption detection and recovery
- State rebuild from filesystem (if possible)

---

### 1.7 Bundle Format for Air-Gapped Environments

**Status:** Not started
**Priority:** Low
**Location:** Should be added to `docs/guides/lore/registry.md`

**Context:** Bundle feature introduced in `docs/guides/lore/registry.md` (lines 135-150):
- Command: `lore bundle @manifest -o workstation-bundle.sh --platform linux/fedora`
- Creates self-extracting archive for offline installation

**Should cover:**
- Bundle internal structure (what's included: binaries, manifests, checksums?)
- Size considerations and optimization
- Signature verification for security
- Manual extraction procedure (if self-extractor fails)
- Platform-specific bundle requirements
- Troubleshooting extraction failures

---

### 1.8 Bindgen User Guide

**Status:** Not started
**Priority:** Low
**Location:** Should be created at `docs/guides/bindgen.md` or `docs/tools/bindgen.md`

**Context:** Bindgen exists but is only documented in internal READMEs:
- `cmd/bindgen/README.md` (line 7): "Proof of concept. This is an experiment..."
- `internal/bindgen/cobra/README.md` (line 5): "Status: Proof of Concept (Working)"
- Line 58: "Not production-ready — use as development accelerator"

**Current issues documented internally (lines 42-58):**
- Extractor captures all 144 commands but misses ~60% of flags
- 4 open issues with Medium to Critical severity

**Should cover:**
- What bindgen does: generates lore manifests from existing CLI tools
- When to use it: bootstrapping new manifests, not production use
- Workflow: generate → review → edit → validate
- Known limitations and what to check in output
- Relationship to main writ/lore tools

**Decision needed:** Should bindgen be promoted to user-facing tool or remain internal?

---

## 2. Incomplete Features

### 2.1 Starlark API Documentation

**Status:** Incomplete
**Priority:** High
**Location:** `docs/guides/lore/create-manifests.md` (lines 78-94)

**Context:** Table shows `ctx` methods but entries are minimal:
- `ctx.run()` - run shell commands
- `ctx.pmm.install()` - install via package manager
- `ctx.pmm.remove()` - remove via package manager
- `ctx.feature()` - check feature flags
- Others listed but not explained

**Missing:**
- Complete PMM method list (what besides install/remove?)
- Return types for `ctx.run()` (only mentioned in verify examples)
- Error handling patterns (try/catch? return codes?)
- Chaining operations
- Which methods require elevation vs unprivileged
- Available built-in functions beyond ctx

**Action:** Expand table with full method signatures, return types, and examples.

---

### 2.2 Manifest Validation Criteria

**Status:** Incomplete
**Priority:** Medium
**Location:** `docs/guides/lore/create-manifests.md` (lines 126-141)

**Context:** Validation checks listed:
- Schema validation
- Phase file existence
- Starlark syntax
- Contract compliance
- Feature consistency
- Platform coverage

**Missing:**
- What "contract compliance" means (no definition anywhere)
- Error message examples for each validation failure
- How to fix common validation errors
- Validation strictness levels (warnings vs errors)

**Action:** Add subsections explaining each validation check with failure examples.

---

### 2.3 Onboarding from Documentation Feature

**Status:** Incomplete (possibly aspirational)
**Priority:** Low
**Location:** `docs/guides/lore/registry.md` (lines 152-172)

**Context:** Advanced AI-assisted feature:
- Command: `lore onboard --from https://wiki.acme.com/backend-setup`
- Claims AI will parse docs, match packages, flag org-specific items

**Missing:**
- What makes documentation parseable (format requirements?)
- Confidence levels and thresholds
- Review workflow before deployment
- Real test case (example uses fictional URL)
- Current implementation status (working? planned?)

**Decision needed:** Is this feature implemented or aspirational documentation?

---

### 2.4 Windows Support

**Status:** Inconsistent
**Priority:** Medium
**Location:** Multiple files

**Context:**
- `docs/guides/writ/platform-awareness.md` (line 48): "Windows (via WSL or native)"
- `docs/guides/lore/create-manifests.md` (lines 57-59): manifest example shows `platforms: [darwin, linux]` — no windows
- `docs/guides/lore/pipeline.md`: no Windows examples
- `.goreleaser.yaml`: builds Windows binaries

**Missing:**
- Clear statement of current Windows support level
- WSL vs native distinction
- Which features work on Windows, which don't
- Windows-specific installation instructions

**Action:** Audit all platform references and create consistent Windows support statement.

---

### 2.5 Migration Detection Improvements

**Status:** Not started
**Priority:** Medium
**Location:** `internal/writ/migrate/`

**Context:** During smoke testing of `writ migrate`, two detection gaps were identified:

**2.5.1 Git-crypt encrypted files not detected**

Files encrypted via git-crypt clean/smudge filters are flagged as unencrypted secrets. The detection logic (`DetectEncryptedFile`) only checks file signatures (magic bytes), not `.gitattributes` patterns.

**Should add:**
- Parse `.gitattributes` for `filter=git-crypt` patterns
- Check `.git-crypt/` directory existence
- Mark matching files as encrypted with `git-crypt` system

**2.5.2 Tuckr structure not detected**

The `Home/Configs/` directory structure with platform suffixes (`all-Darwin`, `all-Linux`, `thenobles-Darwin`) is Tuckr's convention, but detection returns `native` instead of `tuckr`.

**Should add:**
- Detect `Configs/` subdirectories with platform suffixes as Tuckr indicator
- Pattern: `Configs/*-{Darwin,Linux,Windows}` or `Configs/all-*`

---

## 3. Pending Decisions

### 3.1 apt/yum Repository Service

**Status:** Decision needed
**Location:** `wiki/Releasing.md` (line 135)

**Context:** Line states: "For apt/yum repos, consider services like Gemfury or Packagecloud"

**Options:**
- **Gemfury**: Free for public packages, simple API, supports apt/yum
- **Packagecloud**: Similar features, different pricing
- **Self-hosted**: Full control, more maintenance
- **GitHub Releases only**: No repo, users download directly

**Considerations:**
- Cost (public vs private packages)
- API for automation (GoReleaser integration)
- User experience (apt-get install vs curl | bash)
- Maintenance burden

**Decision needed before:** Enabling auto-publish in `.goreleaser.yaml`

---

### 3.2 Authentication Removal Timeline

**Status:** Decision needed
**Location:** `wiki/Releasing.md` (line 99)

**Context:** Table states:
```
| Production | devlore.noblefactor.com | Currently yes, later no |
```

**Questions:**
- When will auth be removed? (Version milestone? Date?)
- What triggers the change? (Public launch? Documentation complete?)
- Should docs mention this temporary state or be written for post-auth world?

**Action needed:** Define timeline or remove ambiguous "later" reference.

---

### 3.3 Bindgen Promotion Decision

**Status:** Decision needed
**Location:** `cmd/bindgen/README.md`, `internal/bindgen/cobra/README.md`

**Context:** Tool exists and partially works but is marked experimental:
- "Proof of concept"
- "Not production-ready"
- Known issues: misses ~60% of flags

**Options:**
- **Promote**: Add to main docs, fix known issues, release as supported tool
- **Keep internal**: Development aid only, not user-facing
- **Remove**: If not useful, delete to reduce maintenance

**Considerations:**
- User demand for manifest generation assistance
- Maintenance cost of supporting another tool
- Risk of users relying on buggy output

---

### 3.4 XDG Path Naming Convention

**Status:** ✓ RESOLVED (2025-01-27)
**Location:** `internal/cli/xdg.go`, `internal/cli/selfinstall.go`

**Decision:** Unified `devlore` namespace with config.d model.

**Structure:**
```
~/.config/devlore/           # XDG_CONFIG_HOME
├── config.yaml              # Shared settings (secrets)
└── config.d/
    ├── writ.yaml            # Writ: segments, vars
    └── lore.yaml            # Lore settings

~/.cache/devlore/            # XDG_CACHE_HOME
├── registry/                # Lore package registry (git clone)
└── downloads/               # Downloaded installers, tarballs

~/.local/share/devlore/      # XDG_DATA_HOME
└── writ/
    └── layers/              # Environment trees (directories)
        ├── base/            #   base layer (org-wide)
        ├── team/            #   team layer
        └── personal/        #   personal layer

~/.local/state/devlore/      # XDG_STATE_HOME
├── writ/
│   └── receipts/            # Deployment receipts
└── lore/
    └── receipts/            # Installation receipts
```

**Rationale:** `devlore` matches project name, provides shared config location for both tools, config.d model allows tool-specific overrides. Layers are directories by default in XDG_DATA_HOME; users can replace any layer directory with a symlink to use a different location.

**Implementation:** PR #36 - Added `DevloreConfigHome()`, `DevloreCacheHome()`, `DevloreDataHome()`, `DevloreStateHome()`, `WritLayersDir()` to xdg.go.

---

## 4. Inconsistencies (To Address Separately)

The following inconsistencies were identified and should be addressed one at a time after deduplication is complete:

1. **Installation methods**: README vs getting-started use different approaches
2. ~~**XDG paths**: `lore/` vs `devlore/` naming~~ ✓ RESOLVED (PR #36 - unified `devlore` namespace)
3. ~~**Config paths**: `$XDG_CONFIG_HOME/lore/` vs `~/.config/devlore/`~~ ✓ RESOLVED (PR #36 - unified `~/.config/devlore/`)
4. **Windows support**: mentioned inconsistently across docs
5. **Auth requirement**: "Currently yes, later no" needs timeline
6. **Command naming**: `lore manifest create` vs potential `lore create`
7. ~~**XDG variables**: mentioned but not used consistently~~ ✓ RESOLVED (PR #36 - consistent devlore namespace)

These are documented here for reference. Address after deduplication work is complete.

---

## 5. Execution Graph Engine

This section tracks the implementation work needed for the lore execution engine. The engine must process `packages-manifest.{yaml,json}` files and execute the four-phase pipeline (prepare → install → provision → verify) for each package.

### 5.1 Current State (Completed)

**Packages Manifest Foundation:**
- `packages-manifest.{yaml,json}` format defined and documented
- Schema embedded in writ (`schema/packages-manifest.json`)
- Manifest loading and validation (`internal/writ/manifest/manifest.go`)
- Graph builder implements `engine.GraphBuilder` interface (`internal/writ/manifest/builder.go`)
- Two entry points:
  - `BuildGraph(ctx, manifestPath, opts)` — load, validate, build from file
  - `BuildGraphFromManifest(ctx, manifest, opts)` — build from pre-parsed manifest

**Engine Infrastructure:**
- `engine.GraphBuilder` interface defined (`internal/engine/builder.go`)
- `engine.ExpandDelegates()` replaces delegate nodes with subgraphs (`internal/engine/compose.go`)
- Basic operation registry and pipeline execution (`internal/engine/`)
- Writ receipt and state management (`internal/writ/receipt/`, `internal/writ/state/`)

### 5.2 Engine Pipeline (To Implement)

```
packages-manifest.yaml
        │
        ▼
┌───────────────────┐
│  manifest.Builder │  ✓ COMPLETE
│  (parse manifest) │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Registry Resolver │  ⊘ BLOCKED (DESIGN-001)
│ (find package)    │
└────────┬──────────┘
         │
    ┌────┴────┐
    │ OR      │
    ▼         ▼
┌───────────────────┐
│Interactive Console│  ○ NOT STARTED (5.8)
│ (AI-assisted      │
│  manifest author) │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Pipeline Loader   │  ⚠️ PARTIAL (lifecycle loading works)
│ (deploy/upgrade/  │
│  decommission)    │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Starlark Bindings │  ⚠️ NEEDS REDESIGN (ADR-051 graph model)
│ (build exec graph)│
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Engine.Run()      │  ✓ EXISTS (needs lore ops)
│ (execute graph)   │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Receipt Writer    │  ✓ EXISTS (writ receipts work)
│ (save results)    │
└───────────────────┘
```

### 5.3 Registry Resolver

**Status:** Blocked on design
**Priority:** High
**Location:** Should be `internal/lore/registry/` or `internal/registry/`
**Design dependency:** [noblefactor/TODO.md DESIGN-001](https://github.com/NobleFactor/noblefactor/blob/main/TODO.md) — AI-Assisted Manifest Authoring

**Context:**
The resolver handles exact package lookups with federated resolution through the registry to native package managers.

**Resolution order (federated search):**
1. **Registry first** (highest priority) — lore packages with cross-platform mappings
2. **Native PM fallback** — if not in registry, try native PM directly

**Cross-platform mapping:**
The registry contains lore packages that know their native PM equivalents:
```yaml
# docker lore package knows:
packages:
  brew: docker
  apt: docker.io
  winget: Docker.Desktop
```
User says `lore add docker` → resolver finds lore package → uses `apt: docker.io` on Ubuntu.

**Resolver requirements:**
- Given a package name (e.g., "docker"), find it in registry first
- Registry location: `$XDG_CACHE_HOME/devlore/registry/` or remote
- If in registry: use lore package with cross-platform mappings
- If not in registry: fall back to native PM (exact name match)
- Handle package versioning (latest, pinned, ranges)
- Support local registry overrides for development

**Non-match behavior:**
- No match in registry or native PM: Error with message
- No fuzzy suggestions — discovery is via AI-assisted authoring (DESIGN-001)

**Design rationale (2025-01-25):**
Fuzzy search was rejected as a solution. Users who don't know package names need AI-assisted conversation to build a manifest, not better string matching. The resolver does exact lookup; discovery is a separate concern handled by the interactive console (5.8).

**Interface sketch:**
```go
type Resolver interface {
    Resolve(ctx context.Context, name string, opts ResolveOptions) (*Package, error)
}

type Package struct {
    Name        string
    Version     string
    ManifestDir string   // Path to lifecycle.yaml and phase scripts
    Platforms   []string // Supported platforms
    Features    []string // Available features
    NativeName  string   // Platform-specific name (e.g., "docker.io" on apt)
}
```

### 5.4 Pipeline Loader

**Status:** ✓ PARTIAL — Lifecycle loading works, but executor needs graph-building integration
**Priority:** High
**Location:** `internal/lore/pipeline/lifecycle.go`, `internal/lore/pipeline/executor.go`

**What's implemented:**
- `Lifecycle` type loads and parses `lifecycle.yaml`
- `LoadLifecycle(packageDir)` loads from package directory
- `LoadLifecycleFromRegistry(registryDir, pkgName)` loads from registry
- Operations defined: `OpDeploy`, `OpUpgrade`, `OpDecommission`
- Phase order: `PhaseOrder = []string{"prepare", "install", "provision", "verify"}`
- Decommission order: `DecommissionPhaseOrder = []string{"unprovision", "uninstall", "cleanup"}`
- Features: `EnabledFeatures(explicit)` merges explicit enables with defaults
- Settings: `ResolvedSettings(explicit)` merges explicit settings with defaults

**What's missing (per ADR-051):**
- Executor should collect graph from Starlark scripts, not execute immediately
- Graph should be passed to `Engine.Run()` for execution
- Receipt should be the completed execution graph

### 5.5 Starlark Bindings — Analysis Then Execute Model

**Status:** ⚠️ NEEDS REDESIGN — Current implementation executes immediately; ADR-051 requires graph building
**Priority:** High
**Location:** `internal/starlark/bindings.go` (needs rewrite)
**Design reference:** [ADR-051 Section 11.2](https://github.com/NobleFactor/noblefactor/blob/main/devlore/design/adr/051-receipt-as-execution-graph.md)

**Problem:** Current bindings execute side effects immediately (`package.install()` runs `apt install`). ADR-051 specifies a two-phase model where Starlark builds a graph, then the engine executes it.

#### Phase Function Signature

Each pipeline phase receives three inputs representing distinct concerns:

```python
def install(system, package, plan):
    #        ↓        ↓       ↓
    #      where    what     how
```

| Input | Purpose | Access |
|-------|---------|--------|
| `system` | Query target environment | Read-only (immediate) |
| `package` | Package metadata and features | Read-only (immediate) |
| `plan` | Build execution graph | Write (deferred execution) |

#### Binding Methods

| Input | Methods | Description |
|-------|---------|-------------|
| `system` | `has(pm)`, `installed(pkg)`, `version(pkg)`, `path(p)`, `which(cmd)`, `platform()` | Query environment |
| `package` | `name`, `version`, `feature(name)`, `setting(name, default)` | Package metadata |
| `plan` | `install()`, `remove()`, `download()`, `extract()`, `write_file()`, `configure()`, `verify()` | Add operations to graph |

#### Scripts Express Intent, Not Commands

**CRITICAL:** Phase scripts NEVER invoke package managers or shell commands directly.

```python
# CORRECT: Express intent
plan.install("docker-ce")
plan.remove("docker.io")

# WRONG: Never shell out to package managers
plan.run("apt install docker-ce")      # ❌
plan.run("brew install docker")        # ❌
```

The host binding layer maps intent to platform-specific commands. The engine
batches operations (one `apt install` for all packages). Platform selection
happens via writ segments (`install.star.Debian` vs `install.star.Fedora`),
not conditionals in scripts.

#### Example

```python
def install(system, package, plan):
    # Query system state
    if system.has("apt"):
        plan.install("docker.io")
    elif system.has("brew"):
        plan.install("docker", cask=True)

    # Check package features (enabled via --with)
    if package.feature("rootless"):
        plan.install("uidmap")
        plan.run("dockerd-rootless-setuptool.sh --install")

    # Promise-based data flow
    tarball = plan.download(url="https://example.com/app.tar.gz")
    plan.extract(tarball, dest="/usr/local")  # depends on tarball
```

#### Implementation Tasks

1. Create `SystemBindings` struct — read-only environment queries
2. Create `PackageBindings` struct — package metadata and feature checks
3. Create `PlanBindings` struct — graph-building operations that return promises
4. Update executor to pass three inputs to phase functions
5. Collect graph from `plan` after Starlark completes
6. Pass graph to `Engine.Run()` for execution

**What exists today (needs rework):**
- `internal/starlark/bindings.go` — Immediate execution model (wrong)
- `internal/lore/pipeline/executor.go` — Runs scripts but doesn't collect graph
- `cmd/pipeline/main.go` — Standalone PoC with immediate execution

**Open design item:** Phase-to-phase transitions (state passing between prepare → install → provision → verify)

### 5.6 Lore Operations

**Status:** Not started
**Priority:** High
**Location:** `internal/engine/ops.go` (extend existing)

**Requirements:**
Add operations for lore's four-phase pipeline:
- `PrepareOp` — run prepare.star
- `InstallOp` — run install.star (delegates to PMM or custom)
- `ProvisionOp` — run provision.star
- `VerifyOp` — run verify.star

Each operation:
1. Loads the phase script
2. Executes via Starlark executor
3. Captures results in `PipelineState`
4. Records to audit log

### 5.7 Lore Receipt Format

**Status:** Not started (writ receipts exist, lore needs its own)
**Priority:** Medium
**Location:** `internal/lore/receipt/`

**Requirements:**
- Record what was installed, which phases ran, results
- Store in `~/.local/state/lore/receipts/`
- Enable `lore upgrade`, `lore reconcile`, `lore decommission`
- Include: package name, version, timestamp, platform, features enabled, phase results

### 5.8 Interactive Console (Bubble Tea)

**Status:** Not started
**Priority:** High (enables AI-assisted manifest authoring)
**Location:** `internal/console/` or `internal/tui/`
**Design dependency:** [noblefactor/TODO.md DESIGN-001](https://github.com/NobleFactor/noblefactor/blob/main/TODO.md) — AI-Assisted Manifest Authoring

**Context:**
AI-assisted manifest authoring requires a Claude Code-style interactive console: streaming AI responses, scrollable conversation history, multi-line user input. This is the UX layer for DESIGN-001's stepwise refinement workflow.

**Stack:**
```
charmbracelet/bubbletea   — Core TUI framework (Elm architecture)
charmbracelet/bubbles     — Components: viewport (scroll), textarea (input)
charmbracelet/glamour     — Markdown rendering in terminal
charmbracelet/lipgloss    — Styling (colors, borders, layout)
```

**Requirements:**
- Streaming text display (AI responses appear incrementally)
- Scrollable conversation history (viewport)
- Multi-line user input (textarea)
- Markdown rendering (code blocks, lists, emphasis)
- Mode switching: flip into interactive console when `lore init --assist` or similar

**Interface sketch:**
```go
// Console is the interactive conversation interface
type Console struct {
    viewport viewport.Model  // Scrollable history
    textarea textarea.Model  // User input
    history  []Message       // Conversation log
    // ...
}

// Run enters interactive mode, returns when user exits
func (c *Console) Run(ctx context.Context) (*Result, error)

// Result contains the outcome of the conversation
type Result struct {
    Manifest *manifest.Manifest  // Generated packages-manifest
    Aborted  bool                // User cancelled
}
```

**Commands that use the console:**
- `lore init --assist` — AI-assisted manifest creation
- `lore manifest create` — Interactive manifest authoring
- Future: `lore add --assist` — AI help finding packages

**References:**
- [Chat-TUI](https://www.nickhedberg.com/blog/projects/chat-tui.md) — Example chat interface with Bubble Tea
- [Bubble Tea docs](https://github.com/charmbracelet/bubbletea)
- [Glamour](https://github.com/charmbracelet/glamour) — Terminal markdown rendering

---

## 6. Multi-Layer Repository Processing

This section tracks the implementation work needed for writ to process multiple repository layers (base → team → personal) with proper precedence.

**Status:** ✓ COMPLETE (2025-01-25)
**Priority:** High

### 6.1 Commands Requiring Multi-Layer Processing

| Command | Needs Layers | Reason |
|---------|--------------|--------|
| `writ add` | Yes | Deploys from all layers, personal overrides team overrides base |
| `writ remove` | Yes | Must know which layer(s) deployed each file |
| `writ regenerate` | Yes | Re-processes templates/secrets from all layers |
| `writ status` | Yes | Shows status across all layers |
| `writ projects` | Yes | Lists projects from all configured repos |
| `writ adopt` | Partial | Targets single layer (via `--layer` flag) but must check for conflicts |

**Commands NOT needing layer processing:**
- `writ migrate` — single repo migration
- `writ config` — system configuration
- `writ secrets *` — operates within single repo
- `writ receipt *` — reads historical data

### 6.2 Processing Order

**Layer order:** base → team → personal (personal wins conflicts)

**Within each repo:** System → Home
- `<repo>/System/` → target `/` (if exists)
- `<repo>/Home/` → target `$HOME` (if exists)

### 6.3 Implementation Plan

#### Phase 1: Data Structures

**6.3.1 LayerSource type** (`internal/writ/layer.go` — NEW)
```go
type LayerSource struct {
    Layer  string // "base", "team", "personal"
    Path   string // Repo root path
    Order  int    // 0=base, 1=team, 2=personal (for sorting)
}

var LayerOrder = []string{"base", "team", "personal"}

type TargetSpec struct {
    SourceDir  string  // "System" or "Home"
    TargetRoot string  // "/" or "$HOME"
}

var TargetOrder = []TargetSpec{
    {"System", "/"},
    {"Home", os.Getenv("HOME")},
}
```

**6.3.2 Update BuildConfig** (`internal/writ/tree/builder.go`)
```go
type BuildConfig struct {
    Sources    []LayerSource  // Replaces single SourceRoot
    TargetRoot string         // Default target (for backwards compat)
    Projects   []string
    Segments   segment.Segments
}
```

**6.3.3 Update BuildResult** (`internal/writ/tree/builder.go`)
```go
type BuildResult struct {
    Graph       *engine.Graph
    Sources     []LayerSource  // All sources processed
    TargetRoot  string
    // ... existing fields ...

    // New: track which layer each node came from
    NodeLayers  map[string]string  // node.ID → layer name
}
```

#### Phase 2: Collection Logic

**6.3.4 collectLayerSources()** (`internal/writ/layer.go`)
```go
func collectLayerSources() ([]LayerSource, error) {
    var sources []LayerSource
    for i, layer := range LayerOrder {
        if path := getConfiguredRepo(layer); path != "" {
            sources = append(sources, LayerSource{
                Layer: layer,
                Path:  path,
                Order: i,
            })
        }
    }
    return sources, nil
}
```

#### Phase 3: Build Graph Updates

**6.3.5 Modify tree.Build()** — Process all layers and targets
```go
func Build(cfg BuildConfig) (*BuildResult, error) {
    result := &BuildResult{
        Graph:      &engine.Graph{},
        Sources:    cfg.Sources,
        NodeLayers: make(map[string]string),
    }

    nodesByTarget := make(map[string]nodeEntry)

    // Process layers in order: base → team → personal
    for _, source := range cfg.Sources {
        // Process targets in order: System → Home
        for _, target := range TargetOrder {
            sourceDir := filepath.Join(source.Path, target.SourceDir)
            if !dirExists(sourceDir) {
                continue
            }

            nodes, err := walkDirectoryWithLayer(sourceDir, target.TargetRoot, source)
            // ... collision detection with layer precedence ...
        }
    }

    return result, nil
}
```

**6.3.6 Layer-aware collision resolution**
```go
// personal > team > base, regardless of specificity
if newLayer.Order > existing.Layer.Order {
    // New layer wins (e.g., personal beats team)
    winner = new
} else if newLayer.Order == existing.Layer.Order {
    // Same layer: use specificity (segment suffix count)
    if newSpecificity > existing.Specificity {
        winner = new
    }
}
```

#### Phase 4: Command Updates

**6.3.7 writ add** — Use collectLayerSources()
**6.3.8 writ remove** — Load state to determine which layer deployed each file
**6.3.9 writ status** — Build graph from all layers, show layer info in output
**6.3.10 writ projects** — List projects from all configured repos with layer indicator
**6.3.11 writ regenerate** — Process all layers for templates/secrets

#### Phase 5: State & Receipt Updates

**6.3.12 Track layer in state** (`internal/writ/state/state.go`)
```go
type FileEntry struct {
    // ... existing fields ...
    Layer string `json:"layer" yaml:"layer"`  // NEW
}
```

**6.3.13 Track layer in receipts** (`internal/writ/receipt/receipt.go`)
```go
type WritContext struct {
    // ... existing fields ...
    Layers []string `json:"layers" yaml:"layers"`  // Layers processed
}
```

### 6.4 Files to Modify

| File | Changes |
|------|---------|
| `internal/writ/layer.go` | NEW: LayerSource, LayerOrder, TargetOrder, collectLayerSources() |
| `internal/writ/tree/builder.go` | BuildConfig.Sources, BuildResult.NodeLayers, layer-aware collision |
| `internal/writ/commands.go` | runAdd, runRemove, runStatus, runProjects, runRegenerate |
| `internal/writ/state/state.go` | FileEntry.Layer field |
| `internal/writ/receipt/receipt.go` | WritContext.Layers field |
| `internal/writ/status/status.go` | Layer-aware status reporting |

### 6.5 Test Cases

1. **Single layer** — backwards compatible, works like today
2. **Two layers, no conflict** — base + personal, disjoint files
3. **Two layers, conflict** — personal overrides base for same target
4. **Three layers** — base → team → personal cascade
5. **System + Home** — System files deployed before Home files
6. **Layer + specificity** — personal/all beats team/project.Darwin

### 6.6 Design Reference

From `02-writ-prd.md`:
> Writ supports layered repositories with precedence: base → team → personal.
> When files conflict, the higher-precedence layer wins (personal > team > base).

From `commands.go:1807-1808`:
```go
Writ supports layered repositories with precedence: base → team → personal.
When files conflict, the higher-precedence layer wins (personal > team > base).
```

---

## 7. Bindgen Tool

This section consolidates all bindgen-related issues from `cmd/bindgen/README.md` and `internal/bindgen/cobra/README.md`. Bindgen is a proof-of-concept tool for generating Starlark bindings from CLI metadata.

### 7.1 Current Status

**Status:** Proof of concept (not production-ready)
**Decision needed:** Promote to user-facing tool, keep internal, or remove? (See Section 3.3)

**What works:**
- Package discovery and AST loading via `golang.org/x/tools/go/packages`
- Extracts `cobra.Command` struct literals (Use, Short, Long, Deprecated, Hidden)
- Extracts flag definitions from `flags.StringVarP()`, `BoolVar()`, etc.
- Type inference from method names (StringSlice → string_list)
- Qualified command names using package directory prefixes

**Test results (docker-cli v27.4.1):**
- Extracted: 144 commands, 272 flags
- Missing: ~60% of flags due to receiver detection limitations

### 7.2 Extractor Issues

| Issue | Severity | Status | Location |
|-------|----------|--------|----------|
| Limited receiver detection | Medium | Open | `extractor.go:373-384` |
| Helper function flags missed | Medium | Open | N/A |
| No subcommand hierarchy tree | Low | Open | N/A |
| Silent unquote failures | Low | Open | `extractor.go:332, 455` |
| Dead `fset` parameter | Low | Open | `extractor.go:168` |

**Limited receiver detection:** `isFlagReceiver()` only matches `flags` or `f` as variable names. Misses `opts.Flags()`, `copts.AddFlags()`, `fs`, `flagSet`, etc.

**Helper function flags:** Flags added via `addFlags(cmd)` or `addCommonFlags(cmd)` helper functions are not captured.

### 7.3 Generator Issues

| Issue | Severity | Status | Location |
|-------|----------|--------|----------|
| Command name not split | Critical | Open | `codegen.go` template |
| No positional args | Critical | Open | `codegen.go` template |
| Stub template broken | Medium | Open | `stubgen.go:9` |

**Command name not split:** Emits `container_run` as single arg instead of `container`, `run` as separate args. Commands don't execute.

**No positional args:** Generated code can't pass image names, container IDs, file paths, etc.

**Stub template broken:** `title` function undefined, IDE stubs don't generate.

### 7.4 Next Steps (Priority Order)

1. **Improve receiver detection** — Track variable assignments or match more patterns (`fs`, `flagSet`, `opts.Flags()`)
2. **Handle helper functions** — Track flags added via helper functions like `addFlags()`
3. **Fix command name splitting** — Split `container_run` → `["container", "run"]`
4. **Add positional argument support** — Generate code to handle required positional args
5. **Fix stub template** — Define or import `title` function
6. **Add tests** — Unit tests for extraction logic

### 7.5 Future Directions

From `cmd/bindgen/README.md`:
- Parse fish/zsh completions for better flag enumeration
- Support OpenAPI specs where CLI wraps an API (e.g., gh, kubectl)
- Man page parsing for richer documentation
- Integration with existing binding definitions (merge parsed + manual)

### 7.6 Files

```
cmd/bindgen/
├── main.go              # CLI with extract-cobra subcommand
└── README.md            # Usage documentation

internal/bindgen/cobra/
├── extractor.go         # Main extraction logic (~430 lines)
├── codegen.go           # Go binding generation
├── stubgen.go           # Starlark stub generation
└── README.md            # Detailed technical notes
```

---

## 8. Package Directory Structure Migration

**Status:** Not started
**Priority:** High
**Blocking:** Pipeline loader, registry packages

The package directory structure was formalized in RFC Section 9.3 (2025-01-26). Existing code and packages use the old flat structure and must be migrated.

### 8.1 Formal Structure (ABNF)

```abnf
package         = "lifecycle.yaml" 1*platform-dir
platform-dir    = platform "/" 1*pipeline-dir
platform        = "Common" / "Darwin" / "Linux" / "Unix" / "Windows" / "Linux.Debian" / "Linux.Fedora"
pipeline-dir    = pipeline "/" 1*phase-script
pipeline        = "Deploy" / "Upgrade" / "Decommission"
phase-script    = phase ".star"
deploy-phase    = "prepare" / "install" / "provision" / "verify"
decom-phase     = "unprovision" / "uninstall" / "cleanup"
```

### 8.2 Migration Tasks

| Task | Location | Status |
|------|----------|--------|
| Remove `Phases map[string]string` from Lifecycle struct | `internal/lore/pipeline/lifecycle.go` | Not started |
| Add phase discovery from directory structure | `internal/lore/pipeline/lifecycle.go` | Not started |
| Update `GetPhaseScript()` to use platform/pipeline path | `internal/lore/pipeline/lifecycle.go` | Not started |
| Migrate docker package to `Linux.Debian/Deploy/` structure | `devlore-registry/packages/docker/` | Not started |
| Migrate kubectl package to `all/Deploy/` structure | `devlore-registry/packages/kubectl/` | Not started |
| Migrate remaining 8 packages | `devlore-registry/packages/*/` | Not started |
| Remove `phases:` from lifecycle.yaml files | `devlore-registry/packages/*/lifecycle.yaml` | Not started |
| Update platform names to capitalized | `devlore-registry/packages/*/lifecycle.yaml` | Not started |

### 8.3 Documents Updated (2025-01-26)

| Document | Section | Change |
|----------|---------|--------|
| RFC (02-devlore-rfc.md) | 9.2 | Removed `phases:` from example, capitalized platforms |
| RFC (02-devlore-rfc.md) | 9.3 | NEW: ABNF grammar, platform/pipeline directories, examples |
| RFC (02-devlore-rfc.md) | 17.2 | Updated Fetch paths for nested structure |
| RFC (02-devlore-rfc.md) | 18.1 | Updated registry structure examples |
| ADR-051 | 11.2 | Three-input API, segment directory syntax (not suffix) |
| lifecycle.json | NEW | JSON Schema for lifecycle.yaml (no `phases:` field) |
| schema.go | | Added `LifecycleSchema` embed |
| AUTHORING.md | 3.1-3.5 | Complete rewrite for new structure and three-input API |

### 8.4 Documents NOT YET Updated

| Document | Issue |
|----------|-------|
| Lore PRD (02-lore-prd.md) | May have old package structure references |
| Existing lifecycle.yaml files | Still have `phases:` section and lowercase platforms |

**Note:** ADR-051 Section 11.2 updated (2025-01-26) — fixed segment directory syntax.

---

## 9. Change Log

| Date | Action | Details |
|------|--------|---------|
| 2025-01-25 | Created | Initial documentation review findings |
| 2025-01-25 | Updated | Added packages-manifest format, schema, validation |
| 2025-01-25 | Updated | Added Section 5: Engine Development Roadmap |
| 2025-01-25 | Updated | Added Section 6: Multi-Layer Repository Processing |
| 2025-01-25 | Updated | Added Section 7: Bindgen Tool (consolidated from READMEs) |
| 2025-01-25 | Completed | Section 6: Multi-layer processing implemented (layer.go, builder.go, commands.go, state.go, receipt.go) |
| 2025-01-25 | Updated | Added prune empty dirs to remove/unlink operations (engine/ops.go, writ/commands.go) |
| 2025-01-25 | Updated | Added design sync rule at top of file |
| 2025-01-25 | Synced | Updated ADR-051 with `layer` field for multi-layer support (context.layers, node.layer) |
| 2025-01-25 | Updated | Registry Resolver (5.3) blocked on DESIGN-001: AI-Assisted Manifest Authoring |
| 2025-01-25 | Added | Section 5.8: Interactive Console (Bubble Tea) for AI-assisted manifest authoring |
| 2025-01-25 | Partial | Sections 5.4-5.5: Lifecycle loading implemented, but bindings need redesign per ADR-051 (graph-building model) |
| 2025-01-25 | Design | Refined Starlark API: `def install(system, package, plan)` — three inputs for read/read/write concerns. Updated ADR-051 Section 11.2 |
| 2025-01-27 | Resolved | Section 3.4: XDG Path Naming Convention — unified `devlore` namespace with config.d model (PR #36) |
| 2025-01-27 | Resolved | Section 4 items 2, 3, 7: XDG inconsistencies resolved via unified devlore namespace (PR #36) |
| 2025-01-27 | Removed | Standalone `completion` command removed — `self-install` is single entry point (PR #36) |
