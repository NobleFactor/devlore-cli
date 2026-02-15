# Phase 2A: Provider Extraction (Flat Names)

## Context

Phase 1 (PR #128) renamed Operation→Action, Execute→Do/Undo across the repo.
Phase 2 is split into two PRs:
- **Phase 2A** (this plan): Move services into provider subpackages, create new
  providers for unregistered pseudo-operations, wire from call sites, delete old files
- **Phase 2B** (next): Template provider, manifest provider, delete Operation enum

Nine action names exist in plan receivers but have no registered Action
implementation — the executor returns "unknown action" for them. This PR creates
real Action implementations in their respective provider packages.

**Worktree**: `devlore-cli.resource-provider`
**Branch**: `feat/provider-extraction`
**Base**: `develop`
**PR**: #129 (merged)

## Provider Layout

```
internal/execution/provider/
  file/         — 10 actions: link, copy, render, backup, unlink, remove, write, move, mkdir, source
  encryption/   — 1 action:  decrypt
  pkg/          — 4 actions: package-install, package-upgrade, package-remove, package-update
  shell/        — 2 actions: shell, powershell
  service/      — 5 actions: service-start, service-stop, service-restart, service-enable, service-disable
  content/      — 1 action:  literal
  net/          — 1 action:  download
  archive/      — 1 action:  archive-extract
  git/          — 3 actions: git-clone, git-checkout, git-pull
```

**28 total registered actions** (was 20 registered + 8 unregistered).
All keep flat names — no dotted names in this PR.

## Steps

### 1. Create provider/file (move FileService + add mkdir, source)

**Create** `provider/file/provider.go`:
- Move `FileService` → `Provider` with all 8 existing methods
- Add `Source(path string) ([]byte, error)` — `os.ReadFile(path)`
- Add `Mkdir(path string, mode os.FileMode) error` — `os.MkdirAll(path, mode)`

**Create** `provider/file/helpers.go`:
- Move `isSubpath()`, `pruneParents()` from `file_service.go`

**Create** `provider/file/actions_gen.go`:
- Move 8 action structs from `ops_file_gen.go`, rename `FileLinkOp`→`Link`, etc.
- Rename `impl *FileService` → `Impl *Provider`
- Add `Mkdir` action: calls `p.Mkdir(path, mode)`, Name=`"mkdir"`
- Add `Source` action: reads file via `p.Source(path)`, stores via `ctx.StoreContent(node, content)`, Name=`"source"`
- Add `Register(reg *execution.ActionRegistry)` — registers all 10

### 2. Create provider/encryption (move EncryptionService)

**Create** `provider/encryption/provider.go` — `Provider` with `Decrypt()` method
**Create** `provider/encryption/actions_gen.go` — `Decrypt` action + `Register()`

### 3. Create provider/pkg (move PackageService)

**Create** `provider/pkg/provider.go` — `Provider` with 4 methods
**Create** `provider/pkg/helpers.go` — 6 helpers (resolvePM*, runBrewCask*)
**Create** `provider/pkg/actions_gen.go` — 4 actions + `Register()`

### 4. Create provider/shell (move ShellService)

**Create** `provider/shell/provider.go` — `Provider` with 2 methods
**Create** `provider/shell/actions_gen.go` — `Exec` (Name=`"shell"`), `PowerShell` + `Register()`

### 5. Create provider/service (move ServiceManagerService)

**Create** `provider/service/provider.go` — `Provider` with 5 methods
**Create** `provider/service/helpers.go` — `run()` helper
**Create** `provider/service/actions_gen.go` — 5 actions + `Register()`

### 6. Create provider/content (new — literal)

**Create** `provider/content/provider.go` — `Provider` with `Literal()` method
**Create** `provider/content/actions_gen.go` — `Literal` action + `Register()`

### 7. Create provider/net (new — download)

**Create** `provider/net/provider.go` — `Provider` with `Download()` method
**Create** `provider/net/actions_gen.go` — `Download` action + `Register()`

### 8. Create provider/archive (new — archive-extract)

**Create** `provider/archive/provider.go` — `Provider` with `Extract()` method
**Create** `provider/archive/actions_gen.go` — `Extract` action + `Register()`

### 9. Create provider/git (new — git-clone, git-checkout, git-pull)

**Create** `provider/git/provider.go` — `Provider` with `Clone()`, `Checkout()`, `Pull()`
**Create** `provider/git/actions_gen.go` — 3 actions + `Register()`

### 10. Create provider/register_gen.go — centralized RegisterAll()

**Create** `provider/register_gen.go`:
- `RegisterAll(reg)` imports all 9 sub-packages and calls their `Register()` functions
- All 4 caller sites use `provider.RegisterAll(reg)` instead of individual imports

### 11. Update caller sites

| File | Change |
|---|---|
| `writ/graph_builder.go` | `provider.RegisterAll(registry)` |
| `writ/commands.go` | `provider.RegisterAll(reg)` |
| `lore/commands.go` | `provider.RegisterAll(registry)` |
| `writ/migrate/session.go` | `provider.RegisterAll(reg)` (bug fix: was registering NO actions) |

### 12. Delete old files (git rm)

11 files:
- `file_service.go`, `encryption_service.go`, `package_service.go`, `shell_service.go`, `service_manager_service.go`
- `ops_file_gen.go`, `ops_encryption_gen.go`, `ops_package_gen.go`, `ops_shell_gen.go`, `ops_service_manager_gen.go`
- `ops_registry.go`

### 13. Delete Plan.Validate()

Delete the `Validate` method from `plan.go`. No callers exist.

### 14. Converge platform service names

| File | Change |
|---|---|
| `platform/darwin.go` | `"launchd-" + actionStr` → `"service-" + actionStr` |
| `platform/linux.go` | `"systemd-" + action.String()` → `"service-" + action.String()` |
| `platform/windows.go` | `"winservice-" + action.String()` → `"service-" + action.String()` |

### 15. Fix service action name bug

`starlark/plan.go` Service method had `Action: "service"` instead of
`Action: "service-" + action.String()`.

### 16. Update tests and verify

- `execution_test.go`: external test package, provider imports, 28-action count
- `phase_test.go`: external test package, `file.Register(reg)`
- `lore/builder_test.go`: `provider.RegisterAll(reg)`

## What This PR Does NOT Touch

- No dotted names — all actions keep flat names
- No flow package (Choose/Gather/Elevate) — Phase 2B
- No engine/build subpackage restructuring
- No ComputeSummary or Preflight changes
- No plan receiver changes (except platform service name convergence)
