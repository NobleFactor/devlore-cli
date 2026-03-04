# Plan: Unit Test Coverage + Code Coalescing

## Context

25 packages have zero test files. Additionally, exploration revealed:
- Compensation state extraction boilerplate duplicated ~15 times across providers
- `pkg/op/provider/service/` has 8 `runtime.GOOS` switches duplicating `internal/host/`
- `pkg/op/provider/pkg/helpers.go` calls `host.NewHost()` on every invocation instead
  of using an injected host
- `cmd/indexgen/main.go` has 6 nearly identical merge functions
- `op.Context` has no `Host` field — providers can't access platform abstractions

**Goal**: Add unit tests to all packages with testable logic. Inject `host.Host`
into the execution pipeline. Coalesce duplicated patterns. Eliminate ad-hoc
`runtime.GOOS` and `host.NewHost()` calls in providers.

---

## Skip (no tests needed)

| Package | Reason |
|---------|--------|
| `cmd/lore`, `cmd/writ` | 3-line main(), pure delegation |
| `cmd/docgen` | 60-line main(), pure cobra wiring |
| `schema` | Only `//go:embed` constants |
| `pkg/op/provider` | Single `RegisterAll()`, tested via `execution_test.go` |
| `pkg/op/provider/encryption` | 5-line nil-check + delegation |
| `internal/lore/onboard` | AI pipeline, covered by `internal/e2e/` |

---

## Phase 1: Add `Host` to `op.Context`

The foundational change. Once `op.Context` carries a `host.Host`, all downstream
provider refactoring becomes straightforward.

### `pkg/op/action.go` — add field

```go
type Context struct {
    context.Context
    DryRun bool
    Writer io.Writer
    Data   map[string]any
    Graph  *Graph
    NodeID string
    Host   any // host.Host — typed as any to avoid import cycle
}
```

> `pkg/op` cannot import `internal/host` (public → internal). Use `any` with a
> type assertion in providers, or define a minimal `op.HostAccessor` interface
> in `pkg/op` that `internal/host.Host` satisfies. The interface approach is
> cleaner — lets providers type-assert once.

### `pkg/op/host.go` — new file, minimal interface

Define the subset of `host.Host` that providers actually need:

```go
// HostProvider exposes platform abstractions to action providers.
// Implemented by internal/host.Host.
type HostProvider interface {
    PackageManager() PackageManagerProvider
    ServiceManager() ServiceManagerProvider
    RunCommand(command string, sudo bool) CommandResult
}
```

Plus `PackageManagerProvider` and `ServiceManagerProvider` interfaces mirroring the
methods providers actually call. This breaks the import cycle while keeping type safety.

### `internal/execution/executor.go` — inject host

All three context-creation sites (`runFlat`, `RunPhased`, `RunNodes`) add:

```go
execCtx := &op.Context{
    // ... existing fields ...
    Host: e.options.Host, // new
}
```

Add `Host host.Host` to executor options (already passed from callers).

### Callers — pass host through

- `internal/lore/commands.go` — passes host when creating executor
- `internal/writ/commands.go` — passes host when creating executor
- `internal/writ/migrate/session.go` — passes host when creating executor

### Test: `pkg/op/host_test.go`

Verify interface satisfaction: `var _ op.HostProvider = (*host.Host)(nil)` equivalent.

---

## Phase 2: Refactor Service Provider → Host Abstraction

**File: `pkg/op/provider/service/provider.go`**

Currently: 8 `runtime.GOOS` switches in `startArgs()`, `stopArgs()`, `restartArgs()`,
`enableArgs()`, `disableArgs()`, `Exists()`, `isRunning()`, `isEnabled()`.

After: Provider reads `ctx.Host.(op.HostProvider).ServiceManager()` in `Do()` and
passes the service manager to its methods. All `runtime.GOOS` switches deleted.

```go
type Provider struct {
    svc op.ServiceManagerProvider // injected from context on first Do()
}

func (p *Provider) Start(svc op.ServiceManagerProvider, name string, output io.Writer) (...) {
    wasRunning := svc.IsRunning(name) // was: p.isRunning(name) with GOOS switch
    result := svc.Start(name)         // was: p.run(output, startArgs(name)...)
    // ...
}
```

**File: `pkg/op/provider/service/actions_gen.go`** — update generated Do() to
extract service manager from context and pass to provider methods.

**Template: `noblefactor-ops.binding-unification`** — update the code generator
so future regeneration produces the new pattern.

Test hooks remain for unit testing (mock the interface instead of function fields).

---

## Phase 3: Refactor Pkg Provider → Host Abstraction

**File: `pkg/op/provider/pkg/helpers.go`**

Currently: `resolvePMForInstall()`, `resolvePMForUpgrade()`, `resolvePMForRemove()`
each call `host.NewHost()` — instantiating platform detection on every invocation.

After: Provider receives `op.HostProvider` from context. Helpers accept the host's
package manager instead of creating their own.

**Brew cask helpers**: `runBrewCaskInstall`, `runBrewCaskUpgrade`, `runBrewCaskRemove`
(3 identical functions) → single `runBrewCask(action string, packages []string)`.

---

## Phase 4: Coalesce — State Extraction Helpers (REMOVED)

State extraction helpers (`pkg/op/state.go`) were removed as unnecessary abstraction.
All `Compensate*` methods use inline type assertions directly on `map[string]any` undo state.

## Phase 5: Coalesce — Indexgen Merge

**File: `cmd/indexgen/main.go`** — replace 6 identical merge functions with generic:

```go
func mergeEntries[T interface{ GetName() string }](
    files []string, existing []T, newFn func(string) T,
) []T
```

---

## Phase 6: Core Data Model Tests — `pkg/op/`

7 new test files, ~80 test functions.

| File | Tests | Key Coverage |
|------|-------|-------------|
| `graph_test.go` | ~30 | SlotValue types, ResolvedSlots (immediate/promise/proxy), JSON/YAML marshal round-trip, HydrateGraph, CanonicalContent, GitStyleChecksum, Summary |
| `convert_test.go` | ~15 | GoToStarlarkValue (8 types), StarlarkValueToGo (8 types), ListToSlice, DictToMap |
| `output_test.go` | ~15 | Output/Gather Starlark attrs, FillSlot (Output/Gather/string/list/dict/int/bool) |
| `phase_test.go` | ~8 | ComputeDelay (constant/linear/exponential), MaxDelay cap, ParseInitialDelay |
| `nodeid_test.go` | ~3 | GenerateNodeID prefix, uniqueness |
| `receiver_test.go` | ~5 | Starlark interface, MakeAttr, NoSuchAttrError, ListToStringSlice |
| `registry_test.go` | ~5 | Register/Get/MustGet(panic)/Names |

## Phase 7: Provider Unit Tests

5 new test files, ~57 test functions. Now using the injected host interfaces for
clean mocking instead of unexported function hooks.

| File | Tests | Key Coverage |
|------|-------|-------------|
| `file/provider_test.go` | ~20 | All Compensate* methods, Move git-mv fallback, Link idempotency, isSubpath, checksumBytes |
| `pkg/provider_test.go` | ~12 | Install/Upgrade/Remove via mock PM, CompensateInstall removes only new, predicates |
| `service/provider_test.go` | ~12 | Start/Stop/Enable/Disable via mock SM, smart compensation |
| `archive/provider_test.go` | ~8 | Extract tar.gz/zip, **zip slip protection**, CompensateExtract |
| `git/provider_test.go` | ~5 | Clone via hook, CompensateClone removes dir |

## Phase 8: Security + Domain Logic Tests

4 new test files, ~31 test functions.

| File | Tests | Key Coverage |
|------|-------|-------------|
| `secrets/detect_test.go` | ~10 | IsEncrypted (SOPS/age/plaintext), IsSecretFile |
| `secrets/crypto_test.go` | ~6 | detectFormat (yaml/json/sops-inner/content-sniff) |
| `secrets/secrets_test.go` | ~5 | Manager creation, HasConfig, .sops.yaml walk-up |
| `reconcile/reconcile_test.go` | ~10 | State string/label, ScanTarget, FromBuildResult |

## Phase 9: Tooling + Remaining Providers

5 new test files, ~24 test functions.

| File | Tests | Key Coverage |
|------|-------|-------------|
| `ui/provider_test.go` | ~8 | Output formatting, silent, color disable |
| `template/provider_test.go` | ~5 | Render with vars, Source/Target/Project, errors |
| `net/provider_test.go` | ~4 | Download (httptest), error codes |
| `shell/provider_test.go` | ~3 | Exec success/empty/fail |
| `docgen/generator_test.go` | ~4 | Output paths, skip hidden/built-in |

## Phase 10: Complex Packages

3 new test files, ~16 test functions.

| File | Tests | Key Coverage |
|------|-------|-------------|
| `cobra/extractor_test.go` | ~6 | Command extraction, flag parsing, collisions |
| `indexgen/main_test.go` | ~4 | Merge preserves metadata, adds new |
| `identity/identity_test.go` | ~5 | expandPath, GenerateIdentity, ParseRecipients |

---

## Cross-Repo Work (noblefactor-ops.binding-unification)

1. ~~**`internal/starlark/codegen.go`** — `templateFuncPlanFillSlots` emits
   `op.FillSlot(`~~ ✅ Done (committed in `c53865c`)
2. ~~**`tpl` → `templateFunc`** — rename all template function prefixes~~ ✅ Done
3. ~~**`config-show.star` / `config-sync.star`** — `success(` → `ui.success(`~~ ✅ Done
4. **File rename** — `receiver_go_gen.go` → `codegen.go` (pending)
5. **Host injection into `typeMappings`** — deferred; GoReceiver moving to provider model

---

## Interface Design Decision

`pkg/op` cannot import `internal/host` (public → internal). Two approaches:

**A. Minimal interfaces in `pkg/op`** (recommended):
Define `HostProvider`, `PackageManagerProvider`, `ServiceManagerProvider` in `pkg/op`.
`internal/host` types already satisfy these interfaces. Providers type-assert
`ctx.Host` to the relevant interface. Clean, no import cycle, compile-time safety.

**B. `ctx.Host any` with runtime type assertions**:
Simpler to implement but loses compile-time safety. Not recommended for a public API.

---

## Verification

After each phase:
1. `make build` — compiles
2. `make test` — all tests pass
3. `make check` — vet, lint (0 issues), shell-lint, test

After all phases:
- Only skip-listed packages show `[no test files]`
- `go test -cover ./pkg/op/...` reports coverage
- Grep for `runtime.GOOS` in `pkg/op/provider/` — zero matches
- Grep for `host.NewHost()` in `pkg/op/provider/` — zero matches
- No `[no test files]` for packages with logic

## Totals

| Phase | Focus | New Files | Tests |
|-------|-------|-----------|-------|
| 1: Host in Context | `pkg/op`, `internal/execution` | 1 new + ~5 modified | — |
| 2: Service → host | `provider/service` | ~2 modified | — |
| 3: Pkg → host | `provider/pkg` | ~2 modified | — |
| 4: State helpers | `pkg/op` | 2 new + 5 modified | ~12 |
| 5: Indexgen merge | `cmd/indexgen` | 1 modified | — |
| 6: Core tests | `pkg/op` | 7 new | ~80 |
| 7: Provider tests | `pkg/op/provider/*` | 5 new | ~57 |
| 8: Security tests | `internal/writ/*` | 4 new | ~31 |
| 9: Tooling tests | various | 5 new | ~24 |
| 10: Complex tests | various | 3 new | ~16 |
| **Total** | | **~28 new + ~15 modified** | **~220** |
