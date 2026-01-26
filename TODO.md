# Implementation TODO

**Scope:** Implementation gaps, product documentation, code issues
**Sister file:** [noblefactor/TODO.md](https://github.com/NobleFactor/noblefactor/blob/main/TODO.md) — Design docs, business strategy, process items

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
- Import procedures: how to convert existing dotfiles repo to writ structure
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

**Status:** Decision needed (see Inconsistencies section)
**Location:** Multiple files

**Context:** Documentation uses both:
- `~/.local/share/lore/` and `$XDG_DATA_HOME/lore/`
- `~/.local/share/devlore/` and `~/.config/devlore/`

**Question:** Should paths use `lore`, `devlore`, or `devlore-cli`?

**Considerations:**
- `lore` is shorter but may conflict with other tools
- `devlore` matches project name
- Changing now may break existing users (if any)

**Action needed:** Choose one convention and update all documentation.

---

## 4. Inconsistencies (To Address Separately)

The following inconsistencies were identified and should be addressed one at a time after deduplication is complete:

1. **Installation methods**: README vs getting-started use different approaches
2. **XDG paths**: `lore/` vs `devlore/` naming
3. **Config paths**: `$XDG_CONFIG_HOME/lore/` vs `~/.config/devlore/`
4. **Windows support**: mentioned inconsistently across docs
5. **Auth requirement**: "Currently yes, later no" needs timeline
6. **Command naming**: `lore manifest create` vs potential `lore create`
7. **XDG variables**: mentioned but not used consistently

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
│ Registry Resolver │  ○ NOT STARTED
│ (find package)    │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Pipeline Loader   │  ○ NOT STARTED
│ (deploy/upgrade/  │
│  decommission)    │
└────────┬──────────┘
         │
         ▼
┌───────────────────┐
│ Starlark Executor │  ○ PARTIAL (internal/starlark/ exists)
│ (run phase code)  │
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

**Status:** Not started
**Priority:** High
**Location:** Should be `internal/lore/registry/` or `internal/registry/`

**Requirements:**
- Given a package name (e.g., "docker"), find it in the registry
- Registry location: `$XDG_DATA_HOME/devlore/registry/` or remote
- Return the package manifest path or error if not found
- Handle package versioning (latest, pinned, ranges)
- Support local registry overrides for development

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
}
```

### 5.4 Pipeline Loader

**Status:** Not started
**Priority:** High
**Location:** Should be `internal/lore/pipeline/`

**Requirements:**
- Load `lifecycle.yaml` from package manifest directory
- Select correct pipeline based on operation:
  - `deploy`: prepare → install → provision → verify
  - `upgrade`: prepare → install → provision → verify (with version diff)
  - `decommission`: unprovision → uninstall → cleanup
- Load Starlark phase scripts (`prepare.star`, `install.star`, etc.)
- Validate phase scripts exist and parse correctly

**Interface sketch:**
```go
type Loader interface {
    LoadPipeline(ctx context.Context, pkg *Package, operation Operation) (*Pipeline, error)
}

type Operation int
const (
    OpDeploy Operation = iota
    OpUpgrade
    OpDecommission
)

type Pipeline struct {
    Phases []Phase
}

type Phase struct {
    Name   string           // "prepare", "install", "provision", "verify"
    Script string           // Path to .star file
    Code   *starlark.Program // Parsed Starlark
}
```

### 5.5 Starlark Executor

**Status:** Partial (`internal/starlark/` exists but needs work)
**Priority:** High
**Location:** `internal/starlark/`

**Requirements:**
- Execute phase scripts with `ctx` object providing:
  - `ctx.run(cmd)` — run shell commands
  - `ctx.pmm.install(packages)` — install via package manager
  - `ctx.pmm.remove(packages)` — remove via package manager
  - `ctx.feature(name)` — check if feature is enabled
  - `ctx.os`, `ctx.arch`, `ctx.distro` — platform info
  - `ctx.user`, `ctx.home` — user info
- Capture stdout/stderr for audit logging
- Handle errors and return structured results
- Support dry-run mode (no side effects)

**Context object (Starlark builtins):**
```python
# prepare.star example
def main(ctx):
    if ctx.os == "linux":
        ctx.run("curl -fsSL https://example.com/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/example.gpg")
        ctx.run("sudo apt-get update")

    if ctx.feature("compose"):
        ctx.log("Compose feature enabled")
```

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

---

## 6. Multi-Layer Repository Processing

This section tracks the implementation work needed for writ to process multiple repository layers (base → team → personal) with proper precedence.

### 6.1 Current State

**What exists:**
- `writ repo` commands support `--layer` flag with values: base, team, personal
- `writ repo list` displays repos in precedence order: base, team, personal
- `getConfiguredRepos()` retrieves all configured repos with their layer info
- Design docs describe layer merging and collision resolution

**What's missing:**
- Deploy only processes a single repo (`cli.GetString("writ", "repo", true)`)
- No code to iterate repos in layer order
- No cross-layer collision detection during build
- No layer-aware receipts/state tracking

### 6.2 Required Changes

**Status:** Not started
**Priority:** High
**Location:** `internal/writ/commands.go`, `internal/writ/tree/builder.go`

**Requirements:**

1. **Collect repos in order:**
   ```go
   // commands.go - replace single repo lookup
   layerOrder := []string{"base", "team", "personal"}
   var repos []RepoConfig
   for _, layer := range layerOrder {
       if repo := getConfiguredRepo(layer); repo != "" {
           repos = append(repos, RepoConfig{Layer: layer, Path: repo})
       }
   }
   ```

2. **Build merged graph:**
   - Process repos in order: base first, personal last
   - Later layers override earlier layers for same target path
   - Track which layer each node came from

3. **Update tree.BuildConfig:**
   ```go
   type BuildConfig struct {
       // Current: single SourceRoot
       // Needed: multiple sources with layer info
       Sources    []LayerSource  // Ordered: base, team, personal
       TargetRoot string
       Projects   []string
       Segments   segment.Segments
   }

   type LayerSource struct {
       Layer string  // "base", "team", or "personal"
       Path  string  // Repo path
   }
   ```

4. **Layer-aware collision detection:**
   - Current: specificity-based (segment suffix count)
   - Needed: layer takes precedence over specificity
   - personal > team > base, regardless of segment specificity

5. **Update receipts and state:**
   - Track which layer each deployed file came from
   - Enable layer-specific removal (`writ remove --layer=team`)

### 6.3 Design Reference

From `02-writ-prd.md`:
> Writ supports layered repositories with precedence: base → team → personal.
> When files conflict, the higher-precedence layer wins (personal > team > base).

From `commands.go:1807-1808`:
```go
Writ supports layered repositories with precedence: base → team → personal.
When files conflict, the higher-precedence layer wins (personal > team > base).
```

The help text documents the feature, but the implementation is incomplete.

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

## 8. Change Log

| Date | Action | Details |
|------|--------|---------|
| 2025-01-25 | Created | Initial documentation review findings |
| 2025-01-25 | Updated | Added packages-manifest format, schema, validation |
| 2025-01-25 | Updated | Added Section 5: Engine Development Roadmap |
| 2025-01-25 | Updated | Added Section 6: Multi-Layer Repository Processing |
| 2025-01-25 | Updated | Added Section 7: Bindgen Tool (consolidated from READMEs) |
