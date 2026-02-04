# Unified Segment Resolution — Update Plan

**Date:** 2026-01-26
**Status:** Draft

## Summary

Lore and writ segments use the same resolution mechanism. Pipelines are constructed by merging scripts from matching platform directories in general-to-specific order. The `all/` directory executes everywhere.

## Key Design Points

1. **Unified resolution** — Lore and writ use identical segment matching
2. **Multi-layer merge** — Scripts from multiple matching directories run in order
3. **General-to-specific** — Resolution order: `all/` → OS → OS.Distro
4. **Accumulation semantics** — Each layer adds to the pipeline (no override)
5. **Graph optimization** — Package operations (install, remove) deduplicated and batched

## Resolution Order

For a Debian system running Deploy:

```
all/Deploy/*           → runs first  (universal)
Linux/Deploy/*         → runs second (OS-level)
Linux.Debian/Deploy/*  → runs last   (distro-specific)
```

For each phase (e.g., `install`), scripts from all matching directories execute in order:

```
all/Deploy/install.star        → plan.package.install("common-tool")
Linux/Deploy/install.star      → plan.package.install("linux-tool")
Linux.Debian/Deploy/install.star → plan.package.install("debian-tool")
```

Result: All three packages queued. The plan builder deduplicates and batches.

## Authoring Warnings

| Risk | Description | Mitigation |
|------|-------------|------------|
| **Conflicting writes** | Multiple layers write same file with different content | Last wins; author must ensure override is intentional |
| **Order dependency** | Specific layer depends on state from general layer | Document dependencies; this is a feature, not a bug |
| **No undo primitive** | Cannot cancel an action from a general layer | Don't use general layer for that phase if you need different behavior |
| **Debugging complexity** | Multiple scripts contribute to one phase | Execution trace shows source file for each action |

---

## Part 1: Documentation Updates

### 1.1. RFC Section 9.3 — Package Directory Structure

**File:** `noblefactor/devlore/design/02-devlore-rfc.md`

**Changes:**

1. Add subsection on **Resolution Order** explaining multi-layer merge
2. Update ABNF comment to note that `all/` is not required (packages may be platform-specific only)
3. Add table showing resolution examples:

```markdown
#### 9.3.1. Platform Resolution

Scripts from all matching platform directories execute in general-to-specific order:

| Target System | Resolution Order |
|---------------|------------------|
| macOS | `Common/` → `Darwin/` |
| Ubuntu | `Common/` → `Linux/` → `Linux.Debian/` |
| Fedora 40 | `Common/` → `Linux/` → `Linux.Fedora/` |
| Windows 11 | `Common/` → `Windows/` |

**Merge semantics:** Each layer's scripts for a given phase ALL run, in order. Actions accumulate. Package operations (install, remove, upgrade) are deduplicated and batched during plan optimization.
```

4. Add note about graph optimization

### 1.2. RFC Section 7 — Phase Pipeline

**File:** `noblefactor/devlore/design/02-devlore-rfc.md`

**Changes:**

1. Update to reference multi-layer resolution from Section 9.3
2. Add note that phases may have multiple contributing scripts

### 1.3. ADR-006 — Phase Pipeline Design

**File:** `noblefactor/devlore/design/adr/006-phase-pipeline.md`

**Changes:**

1. Update phase contract to show three-input API:
   ```python
   def install(system, package, plan):
   ```
2. Add section on **Multi-Layer Execution**
3. Update Docker example to show `Linux/` + `Linux.Debian/` split

### 1.4. New ADR — Unified Segment Resolution

**File:** `noblefactor/devlore/design/adr/053-unified-segment-resolution.md`

**Purpose:** Document the decision to unify lore and writ segment matching.

**Sections:**
- Context: Both tools need platform-specific behavior
- Decision: Unified resolution algorithm
- Resolution order: general-to-specific
- Merge semantics: accumulation, not override
- Graph optimization: dedup and batch package operations
- Warnings and authoring guidance

### 1.5. Writ RFC Section 4 — Segment Matching

**File:** `noblefactor/devlore/design/writ/03-writ-rfc.md`

**Changes:**

1. Reference ADR-053 for unified resolution
2. Note that writ uses same algorithm as lore
3. Update examples to show multi-layer matching

### 1.6. AUTHORING.md — Package Authoring Guide

**File:** `devlore-registry/AUTHORING.md`

**Changes:**

1. Update Section 3.1 to explain multi-layer structure
2. Add examples of when to use `all/` vs OS vs OS.Distro
3. Add warnings section about layer conflicts
4. Update docker example to show three-tier structure:
   ```
   docker/
   ├── all/
   │   └── Deploy/
   │       └── verify.star       # Same verification everywhere
   ├── Linux/
   │   └── Deploy/
   │       ├── provision.star    # Common Linux config
   │       └── cleanup.star
   ├── Linux.Debian/
   │   └── Deploy/
   │       ├── prepare.star      # apt repo setup
   │       └── install.star      # apt packages
   └── Linux.Fedora/
       └── Deploy/
           ├── prepare.star      # dnf repo setup
           └── install.star      # dnf packages
   ```

---

## Part 2: Implementation Updates

### 2.1. Platform Resolution Module

**File:** `internal/lore/pipeline/resolver.go` (new)

**Purpose:** Resolve platform directories for a package.

```go
// Resolver finds and orders platform directories for execution.
type Resolver struct {
    os     string // runtime.GOOS
    distro string // detected from /etc/os-release
}

// Resolve returns ordered list of matching platform directories.
// Order: all → OS → OS.Distro (general to specific)
func (r *Resolver) Resolve(packageDir, pipeline string) ([]string, error)

// Example for Linux.Debian + Deploy:
// Returns: ["all/Deploy", "Linux/Deploy", "Linux.Debian/Deploy"]
// (only directories that exist)
```

### 2.2. Phase Collector

**File:** `internal/lore/pipeline/collector.go` (new)

**Purpose:** Collect all phase scripts from resolved directories.

```go
// PhaseScripts maps phase name to ordered list of scripts.
type PhaseScripts map[string][]string

// Collect gathers all phase scripts from platform directories.
func Collect(dirs []string) (PhaseScripts, error)

// Example output:
// {
//   "install": ["all/Deploy/install.star", "Linux/Deploy/install.star", "Linux.Debian/Deploy/install.star"],
//   "verify": ["all/Deploy/verify.star"],
// }
```

### 2.3. Executor Updates

**File:** `internal/lore/pipeline/executor.go`

**Changes:**

1. Update `executePhase` to run multiple scripts per phase
2. Track source file in execution trace
3. Accumulate plan actions across scripts

```go
func (e *Executor) executePhase(lifecycle *Lifecycle, phaseName string, scripts []string) PhaseResult {
    for _, script := range scripts {
        err := e.runPhaseScript(script, phaseName)
        if err != nil {
            // Record which script failed
            return PhaseResult{Error: fmt.Errorf("%s: %w", script, err)}
        }
    }
}
```

### 2.4. Lifecycle Struct Updates

**File:** `internal/lore/pipeline/lifecycle.go`

**Changes:**

1. Remove `Phases map[string]string` field — phases discovered from directories
2. Add `PackageDir` usage for directory-based resolution
3. Update `GetPhaseScript` → `GetPhaseScripts` returning `[]string`

### 2.5. Plan Builder — Deduplication

**File:** `internal/lore/plan/builder.go` (new or existing)

**Purpose:** Build execution graph with optimization.

```go
// Builder accumulates plan actions from phase scripts.
type Builder struct {
    installs []string
    removes  []string
    // ... other action types
}

// Optimize deduplicates and batches package operations.
func (b *Builder) Optimize() *Plan {
    // Deduplicate: plan.package.install("docker-ce") twice → one install
    // Batch: multiple plan.package.install() → single apt install docker-ce pkg2 pkg3
}
```

### 2.6. Distro Detection

**File:** `internal/platform/distro.go` (new or existing)

**Purpose:** Detect Linux distribution family.

```go
// DetectDistro returns the distro family (Debian, Fedora) or empty.
func DetectDistro() string {
    // Parse /etc/os-release
    // Map ID/ID_LIKE to family:
    //   ubuntu, debian, pop, mint → Debian
    //   fedora, rhel, centos, rocky, alma, amzn, azurelinux → Fedora
}
```

### 2.7. Shared Resolution Module (lore + writ)

**File:** `internal/segment/resolver.go` (new)

**Purpose:** Unified resolution used by both lore and writ.

```go
// This Go module is shared between lore and writ.
// Both import from internal/segment.

type Platform struct {
    OS     string // Darwin, Linux, Windows
    Distro string // Debian, Fedora, or empty
}

func (p Platform) MatchingSegments() []string {
    // Returns: ["all", "Linux", "Linux.Debian"] for Debian
}
```

### 2.8. Test Updates

**Files:**
- `internal/lore/pipeline/resolver_test.go` (new)
- `internal/lore/pipeline/collector_test.go` (new)
- `internal/lore/pipeline/pipeline_test.go` (update)

**Test cases:**
1. Single platform directory (all only)
2. Two layers (all + Darwin)
3. Three layers (all + Linux + Linux.Debian)
4. Missing intermediate layer (all + Linux.Debian, no Linux/)
5. Phase in some layers but not others
6. Deduplication of package operations

---

## Part 3: Migration

### 3.1. Existing Packages

The docker package already uses the directory structure. No migration needed for existing packages using the new structure.

All packages use the directory structure for phase scripts. There is no `phases:` field in lifecycle.yaml.

### 3.2. Schema Update

**File:** `devlore-cli/schema/lifecycle.json`

1. Remove `phases` from required fields
2. Add deprecation note in description
3. Platforms remain in lifecycle.yaml for metadata (supported platforms list)

---

## Implementation Order

| Phase | Task | Effort |
|-------|------|--------|
| 1 | Create ADR-053 (unified resolution) | S |
| 2 | Implement distro detection | S |
| 3 | Implement resolver module | M |
| 4 | Implement collector module | M |
| 5 | Update executor for multi-script phases | M |
| 6 | Implement plan builder with dedup | M |
| 7 | Update RFC Section 9.3 | S |
| 8 | Update ADR-006 | S |
| 9 | Update writ RFC Section 4 | S |
| 10 | Update AUTHORING.md | S |
| 11 | Update lifecycle.go (remove Phases field) | S |
| 12 | Write tests | M |

**Effort key:** S (small, < 1 hour) · M (medium, 1-4 hours)

---

## Open Questions

1. **Empty directories:** If `Linux/Deploy/` exists but is empty, is that an error or just no-op?
   - **Proposed:** No-op. Empty directory contributes nothing.

2. **Phase function name:** Must match filename (`install.star` → `def install()`)?
   - **Proposed:** Yes, enforced at runtime.

3. **Upgrade pipeline phases:** Same as Deploy, or different set?
   - **Current:** Same phases (prepare, install, provision, verify)
   - **Proposed:** Keep same. Upgrade is "deploy with existing state."
