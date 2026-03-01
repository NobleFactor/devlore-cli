# Code Complexity Assessment & Package Coherence Plan

## Context

The codebase is ~44K LOC across ~160 Go files. The dependency graph is clean (no circular imports), and the overall architecture is sound. However, several coherence problems have accumulated:

1. **Scattered boilerplate** — identical compensation, filesystem, and state-casting patterns repeated 17+ times across providers with no shared utility
2. **Monolithic command files** — `writ/commands.go` (1,431 lines), `lore/commands.go` (808 lines), and `cli/config.go` (658 lines) each bundle multiple unrelated commands
3. **Parameter threading** — `buildPackageNodes`, `executeScriptAction`, `prepareScriptEnv` each pass 6-7 identical parameters instead of using a session struct
4. **Complexity violations** — `Gather.Do()` is at cyclomatic complexity 21 (threshold: 20); `RunPhased()` has 6+ nesting levels in rollback logic
5. **21 stub functions** returning `"not yet implemented"` — violates greenfield principle
6. **Magic strings** — `executor.go` uses bare string literals for statuses that already have constants in `pkg/projection`

This plan gathers scattered logic into coherent homes and splits monoliths by responsibility.

---

## Phase 1: Execution Utilities (foundation, no behavior change)

**Goal:** Gather repeated provider boilerplate into two focused files in `internal/execution/` where every provider already imports from.

### 1a. Create `internal/execution/compensate.go`

Typed compensation state extraction. Replaces the 17+ instances of:
```go
s, _ := state.(map[string]any)
if s == nil { return nil }
path, _ := s["path"].(string)
```

Provide `ParseCompensation(state any) *CompensationState` and typed accessors: `String(key)`, `Bool(key)`, `Bytes(key)`, `FileMode(key)`.

### 1b. Create `internal/execution/fsutil.go`

Two helpers used 16+ times across providers:
- `EnsureParentDir(path string) error` — wraps `os.MkdirAll(filepath.Dir(path), 0755)`
- `DefaultFileMode(mode os.FileMode) os.FileMode` — returns 0644 when mode is zero

### 1c. Add status constants to `pkg/projection/phase.go`

`executor.go` uses `"skipped"`, `"failed"`, `"completed"` as bare strings for `RollbackEntry.Status` and `Attempt.Status`. Add:
```go
const (
    AttemptCompleted  = "completed"
    AttemptFailed     = "failed"
    RollbackCompleted = "completed"
    RollbackFailed    = "failed"
    RollbackSkipped   = "skipped"
)
```

**Files created:** `internal/execution/compensate.go`, `internal/execution/fsutil.go`
**Files modified:** `pkg/projection/phase.go`
**Dependency impact:** None — all consumers already import `execution` or `projection`

---

## Phase 2: Provider Deduplication

**Goal:** Apply Phase 1 utilities to eliminate boilerplate across all 5 provider packages.

### 2a. Refactor compensation methods

Replace state-casting boilerplate in every `Compensate*` method with `execution.ParseCompensation()` + typed accessors.

**Files:** `provider/file/provider.go` (6 methods), `provider/service/provider.go` (2), `provider/pkg/provider.go` (2), `provider/git/provider.go` (1), `provider/archive/provider.go` (1)

### 2b. Refactor filesystem calls

Replace all `os.MkdirAll(filepath.Dir(...), 0755)` with `execution.EnsureParentDir()`.
Replace all `if mode == 0 { mode = 0644 }` with `execution.DefaultFileMode()`.

**Files:** `provider/file/provider.go`, `provider/archive/provider.go`

### 2c. Consolidate package manager helpers

In `provider/pkg/helpers.go`:
- Merge `resolvePMForInstall`, `resolvePMForUpgrade`, `resolvePMForRemove` into a single `resolvePM(mode, override, packages)` function
- Merge `runBrewCaskInstall`, `runBrewCaskUpgrade`, `runBrewCaskRemove` into `runBrewCask(action, packages)`

**Files:** `provider/pkg/helpers.go`, `provider/pkg/provider.go`

### 2d. Unify signing error types

`SignError` and `VerifyError` in `signing/errors.go` are structurally identical (same fields, same `Error()` and `Unwrap()` methods). Unify into `CryptoError` with an `Operation string` field.

**Files:** `internal/signing/errors.go`, plus callers in `gpg.go`, `gcp_kms.go`, `aws_kms.go`, `azure_kv.go`

### 2e. Replace magic strings in executor

Replace bare `"skipped"`, `"failed"`, `"completed"` in `executor.go` lines 249-337 with the Phase 1 constants.

**Files:** `internal/execution/executor.go`

---

## Phase 3: Command File Splitting (writ)

**Goal:** Split `internal/writ/commands.go` (1,431 lines) into files with one command per file.

| New File | Responsibility | Key Functions |
|---|---|---|
| `deploy_cmd.go` | Deploy command | `newDeployCmd`, `runDeployV2`, `reportGraphContext`, `reportCollisions` |
| `decommission_cmd.go` | Decommission command | `newDecommissionCmd`, `runDecommission`, `loadStateView`, `projectSet` |
| `upgrade_cmd.go` | Upgrade command | `newUpgradeCmd`, `runUpgrade`, `loadViewAndCopiedFiles`, `prepareUpgradeEngine`, `executeUpgrades`, `upgradeFile`, `buildUpgradeChain` |
| `reconcile_cmd.go` | Reconcile command | `newReconcileCmd`, `runReconcile`, all `buildReport*`, all `reconcile*`, all `output*`, `classify*` |
| `adopt_cmd.go` | Adopt command | `newAdoptCmd`, `runAdopt`, `adoptFiles`, `adoptItem`, `adoptDirectory`, `adoptFile`, `copyFile` |
| `template_data.go` | Shared template helpers | `builtinTemplateData`, `xdgPath`, `expandPath`, `hasDecryptOp` |

Stub commands (`newInspectCmd`, `newListCmd`, `newReceiptCmd` + graph builder stubs) — see Phase 5.

All files remain `package writ`. No import changes. No API changes.

---

## Phase 4: Command File Splitting (lore + cli)

### 4a. Split `internal/lore/commands.go` (808 lines)

| New File | Responsibility | Key Functions |
|---|---|---|
| `deploy_cmd.go` | Deploy command | `newDeployCmd`, `runDeploy`, `parseLoreDeployConfig`, `resolvePackages`, `filterLowConfidence`, `executeDeployments`, `mergeFeatures` |
| `search_cmd.go` | Search command | `newSearchCmd`, `runSearch` |
| `onboard_cmd.go` | Onboard command | `newOnboardCmd`, `runOnboard` |

Stub commands (upgrade, decommission, reconcile, bundle, manifest/*, list, resolve, update, inspect, publish, audit) — see Phase 5.

### 4b. Split `internal/cli/config.go` (658 lines)

| New File | Responsibility |
|---|---|
| `config.go` | `NewConfigCmd` + 8 `newConfig*Cmd` constructors, `configKeyCompletion` |
| `config_nested.go` | `loadConfig`, `saveConfig`, `getNestedValue`, `setNestedValue`, `deleteNestedValue`, `printFlattened`, `formatValue`, `getSchemaKeys`, `extractKeys`, `coerceValue`, `schemaTypeForKey` |

---

## Phase 5: Stub Removal (greenfield principle)

Per the governing principle: stub functions returning `"not yet implemented"` are a critical bug.

**Go stubs to remove (24 total):**
- `internal/lore/commands.go`: 15 stubs (upgrade, decommission, reconcile, bundle, manifest/create/validate/test/show/update, list, resolve, update, inspect, publish, audit)
- `internal/writ/commands.go`: 5 stubs (inspect, list, receipt/show, receipt/list, adopt --from-receipt)
- `internal/writ/graph_builder.go`: 4 stubs (upgrade, reconcile, adopt, migrate graph builders)

For each: remove the `new*Cmd()` function, remove the `AddCommand()` call in `root.go`, remove any stub graph builder. Features get added when implemented, not before.

**Starlark stubs** in `star/extensions/` (sign.star, extract.star): leave as-is — these are extension scripts, not Go code, and are in active development.

---

## Phase 6: Builder Parameter Reduction

**Goal:** Eliminate 6-7 parameter threading in `internal/lore/builder.go`.

The `Planner` struct already holds `Platform`, `ActionRegistry`, `RegistryClient`, `Features`, `Settings`, `DryRun`. But the free functions `buildPackageNodes(graph, pkg, h, plat, cfg, reg)` receive overlapping values unpacked from the caller.

**Approach:** Convert the free functions to methods on `Planner`, adding `Host` and `Graph` as call-scoped fields on a `planSession` receiver:

```go
type planSession struct {
    planner *Planner
    graph   *projection.Graph
    host    host.Host
    plat    string
    reg     *execution.ActionRegistry
    cfg     BuildConfig
}
```

The three 6-7 parameter functions become:
- `(s *planSession) buildPackageNodes(pkg *lorepackage.Release) error`
- `(s *planSession) executeScriptAction(pkg *lorepackage.Release, action *lorepackage.ScriptAction) (*projection.RetryPolicy, error)`
- `(s *planSession) prepareScriptEnv(pkg *lorepackage.Release, action *lorepackage.ScriptAction) (...)`

Each takes only the per-call-varying parameter(s).

**Files:** `internal/lore/builder.go` only. No dependency changes.

---

## Phase 7: Executor Complexity Reduction

### 7a. Extract rollback from `RunPhased()`

Move the rollback block (lines 221-284 of `executor.go`) into `internal/execution/rollback.go`:
- `(e *GraphExecutor) rollbackPhases(...)` — marks remaining phases skipped, unwinds node stack, executes compensating phases
- `phaseRecovery` type definition moves to `rollback.go`

`RunPhased()` reduces from ~120 to ~60 lines. Nesting drops from 6+ to 3.

### 7b. Split `Gather.Do()` (complexity 21 -> ~12)

In `internal/execution/flow/gather.go`, extract:
- `(a *Gather) runSequential(...)` — the `limit <= 1` branch
- `(a *Gather) runConcurrent(...)` — the semaphore/goroutine branch
- `(a *Gather) collectResults(outcomes)` — common tail

`Do()` becomes a dispatcher that validates inputs and delegates to the appropriate mode.

**Files:** `internal/execution/rollback.go` (new), `internal/execution/executor.go`, `internal/execution/flow/gather.go`

---

## Verification

After each phase:
1. `make check` — vet, lint, complexity (gocyclo <= 20), tests
2. Confirm no new imports or circular dependencies
3. Grep for `legacy|backward|compat|deprecated` — remove any matches

After all phases:
- `make check` passes cleanly
- `gocyclo -over 15 .` shows zero functions above 20, reduced count above 15
- No `"not yet implemented"` stubs remain in Go code
- All provider `Compensate*` methods use `CompensationState`
- No bare status strings in `executor.go`
