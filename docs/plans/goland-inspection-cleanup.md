---
title: "GoLand Inspection Cleanup — Go Code Issues"
issue: https://github.com/NobleFactor/devlore-cli/issues/238
status: complete
created: 2026-03-15
updated: 2026-03-16
---

# Plan: GoLand Inspection Cleanup — Go Code Issues

## Summary

Systematically fix 291 Go code issues identified by GoLand 2025.3.3 inspections on the
devlore-cli project. Issues span potential nil dereferences, unhandled errors, incorrect
error handling, missing switch cases, dead code, and style/modernization items. All issues
are in hand-written source — generated files (`pkg/op/provider/*/gen/`) are clean.

## Goals

1. **Eliminate runtime risks**: Fix nil dereferences, always-false conditions, and
   incorrect error comparisons that could cause panics or silent bugs.
2. **Enforce error discipline**: Handle or explicitly discard every error return value.
3. **Remove dead code**: Delete unused functions, constants, parameters, types, and
   variables — this is greenfield, no legacy to preserve.

## Current State

| Category                          | Count   | Severity                |
|-----------------------------------|---------|-------------------------|
| GoUnhandledErrorResult            | 117     | WARNING                 |
| GoUnusedExportedFunction          | 40      | WARNING                 |
| GoUnusedParameter                 | 30      | WARNING                 |
| GoUnusedConst                     | 16      | WARNING                 |
| GoRedundantConversion             | 15      | WEAK WARNING            |
| GoMaybeNil                        | 14      | WARNING                 |
| GoSwitchMissingCasesForIotaConsts | 11      | WARNING                 |
| GoSimplifyWithNew                 | 11      | SYNTAX_UPDATE (Go 1.26) |
| GoDirectComparisonOfErrors        | 7       | WEAK WARNING            |
| GoMixedReceiverTypes              | 6       | WEAK WARNING            |
| GoErrorsAsToAsType                | 5       | SYNTAX_UPDATE (Go 1.26) |
| GoUnusedFunction                  | 3       | WARNING                 |
| GoBoolExpressions                 | 2       | WARNING                 |
| GoDeprecation                     | 2       | WARNING                 |
| GoPreferNilSlice                  | 2       | WEAK WARNING            |
| GoUnusedGlobalVariable            | 2       | WARNING                 |
| GoTypeAssertionOnErrors           | 1       | WEAK WARNING            |
| GoUnusedType                      | 1       | WARNING                 |
| GoReservedWordUsedAsName          | 1       | WARNING                 |
| **Total**                         | **286** |                         |

Note: 5 items from `prototype/bindgen/internal/` (unused exported functions) are excluded
from the phase plan below — that directory is a standalone prototype and will be cleaned up
or removed separately.

## Implementation Phases

### Phase 1: Potential Runtime Bugs (16 issues) — `complete`

Fix issues that could cause panics or silently wrong behavior.

**GoMaybeNil (14)** — add nil guards or `require.NotNil` assertions:

- [x] `internal/cli/selfinstall_test.go` lines 65, 89 — `t.Error` → `t.Fatal` to prevent nil dereference
- [x] `internal/console/console_test.go` lines 155, 160, 173 — nil guard after `session.Next()`
- [x] `internal/execution/phase_test.go` lines 318, 321, 324, 327, 469 — nil guard after `graph.PhaseByID()`
- [x] `pkg/op/provider/starcomplexity/provider.go` line 99 — nil guard after `opts.Parse()`
- [x] `pkg/op/provider/starindex/provider.go` line 132 — nil guard after `opts.Parse()`

**GoBoolExpressions (2)** — conditions that are always false:

- [x] `internal/cli/output_test.go` line 84 — deleted tautological test (const can never differ)
- [x] `internal/writ/segment/segment_test.go` line 398 — deleted tautological test (const can never differ)

**Files**:

- `internal/cli/selfinstall_test.go` — Modify
- `internal/console/console_test.go` — Modify
- `internal/execution/phase_test.go` — Modify
- `pkg/op/provider/starcomplexity/provider.go` — Modify
- `pkg/op/provider/starindex/provider.go` — Modify
- `internal/cli/output_test.go` — Modify
- `internal/writ/segment/segment_test.go` — Modify

### Phase 2: Error Handling Correctness (8 issues) — `complete`

Fix error comparisons that fail on wrapped errors.

**GoDirectComparisonOfErrors (7)** — replace `==`/`!=` with `errors.Is()`:

- [x] `pkg/op/action_reflect_test.go` lines 473, 501 — `err != testErr`
- [x] `pkg/op/root_test.go` line 526 — `err != op.ErrReadOnly`
- [x] `pkg/op/triad_test.go` lines 357, 360, 363, 366 — `err != op.ErrReadOnly`

**GoTypeAssertionOnErrors (1)** — replace type assertion with `errors.As`:

- [x] `cmd/devlore-test/cli_test.go` line 82 — `err.(*exec.ExitError)`

**Files**:

- `pkg/op/action_reflect_test.go` — Modify
- `pkg/op/root_test.go` — Modify
- `pkg/op/triad_test.go` — Modify
- `cmd/devlore-test/cli_test.go` — Modify

### Phase 3: Missing Switch Cases (11 issues) — `complete`

Add exhaustive `case` branches (or explicit `default` with panic) for iota-const switches.

- [x] `internal/cli/output.go` lines 319, 356 — added `default:` to both reflect.Kind switches
- [x] `internal/console/model.go` lines 126, 168, 223, 293 — added StepProgress case, default for key types, exhaustive StepType cases
- [x] `internal/execution/executor.go` line 804 — added ResultPending, ResultRunning cases
- [x] `internal/writ/commands.go` line 614 — added upgradeResultError case with error reporting
- [x] `internal/writ/migrate/session.go` line 113 — added missing SessionState cases
- [x] `internal/writ/migrate/session_test.go` line 304 — added default case
- [x] `pkg/op/provider/file/gitignore/tracker.go` line 118 — added NoMatch case

**Files**:

- `internal/cli/output.go` — Modify
- `internal/console/model.go` — Modify
- `internal/execution/executor.go` — Modify
- `internal/writ/commands.go` — Modify
- `internal/writ/migrate/session.go` — Modify
- `internal/writ/migrate/session_test.go` — Modify
- `pkg/op/provider/file/gitignore/tracker.go` — Modify

### Phase 4: Unhandled Error Results (117 issues) — `complete`

The largest category. Many are `defer f.Close()` or `fmt.Fprintf` where the error is
discarded. Strategy:

- **`Close` errors (defer)**: Use `iox.Close` helper (adopted in PR #232) or `defer func() { _ = f.Close() }()`.
  Check error on write-file Close in success paths.
- **`fmt.Fprintf/Fprintln/Fprint`**: Assign to `_` explicitly: `_, _ = fmt.Fprintf(...)`.
- **`os.Remove/RemoveAll`**: Check the error or assign to `_` for best-effort.
- **`os.Setenv` in tests**: Replace with `t.Setenv()` (idiomatic, auto-restores).

**Production code (77 errors across 18 files)**:

- [x] `internal/lore/onboard/onboard.go` — 11 (Close via iox.Close in prior PR, Fprintf: `_, _ =`)
- [x] `internal/writ/migrate/session.go` — 10 (Fprintf: `_, _ =`, Fprintln: `_, _ =`)
- [x] `internal/writ/tree/output.go` — 10 (Fprintf: `_, _ =`)
- [x] `pkg/op/provider/archive/provider.go` — 10 (Close: checked on success, `_ =` on error; Remove: `//nolint:errcheck`)
- [x] `cmd/indexgen/main.go` — 5 (Fprintf/Fprintln: `_, _ =`)
- [x] `internal/model/config.go` — 5 (Fprint: `_, _ =`)
- [x] `internal/devloretest/commands.go` — 4 (Close: already `iox.Close` in prior PR)
- [x] `pkg/op/provider/file/provider.go` — 4 (Close: already `iox.Close` in prior PR)
- [x] `pkg/op/provider/mem/extract.go` — 4 (Fprintf: `_, _ =`)
- [x] `internal/execution/executor.go` — 3 (Close: `defer func() { _ = execCtx.Root.Close() }()`)
- [x] `pkg/op/provider/file/gitignore/tracker.go` — 3 (Close: already `iox.Close` in prior PR)
- [x] `internal/e2e/testrunner/runner.go` — 2 (Close: already `iox.Close`; RemoveAll: `defer func() { _ = ... }()`)
- [x] `cmd/docgen/main.go` — 1 (Fprintf: `_, _ =`)
- [x] `internal/cli/man.go` — 1 (Remove: already `//nolint:errcheck` + `_ = tmpFile.Close()`)
- [x] `internal/cli/receipts.go` — 1 (Remove: already `//nolint:errcheck`)
- [x] `internal/execution/flow/degraded.go` — 1 (Fprintln: `_, _ =`)
- [x] `pkg/op/provider/appnet/provider.go` — 1 (Close: already `iox.Close` in prior PR)
- [x] `pkg/op/provider/file/resource.go` — 1 (Close: already `iox.Close` in prior PR)

**Test code (40 errors across 10 files)**:

- [x] `internal/credentials/credentials_test.go` — 12 (Setenv → `t.Setenv`)
- [x] `pkg/op/provider/archive/provider_test.go` — 5 (Close: already `defer func() { _ = f.Close() }()`)
- [x] `pkg/op/root_test.go` — 5 (Close: `_ = f.Close()`, `_ = cr.Close()` in Cleanup)
- [x] `cmd/devlore-test/cli_test.go` — 4 (Fprintf: `_, _ =`; RemoveAll: `defer func() { _ = ... }()`)
- [x] `internal/lore/builder_test.go` — 4 (Close: already `defer func() { _ = root.Close() }()`)
- [x] `internal/execution/preflight_test.go` — 3 (Close: `_ = f.Close()`; Shadow: `_, _ =`)
- [x] `internal/cli/config_test.go` — 2 (Close: `_ = w.Close()`; ReadFrom: `_, _ =`)
- [x] `pkg/op/provider/file/provider_test.go` — 2 (Close: `_ = f.Close()`)
- [x] `pkg/op/triad_test.go` — 2 (Close: `_ = root.Close()` in Cleanup; RemoveAll: `_ =`)
- [x] `pkg/op/recovery_site_test.go` — 1 (RemoveAll: `_ =`)

**Files**: All 28 files listed above.

### Phase 5: Dead Code Removal (92 issues) — `deferred` (human-only)

Delete unused exported functions, unexported functions, constants, types, parameters, and
global variables. Per the governing principle: this is greenfield — no legacy users.

> **Status note:** This phase requires human judgment to determine which exports are
> genuinely dead vs. needed for upcoming features. Claude's static analysis was unreliable
> here — deferring to humans.

**GoUnusedExportedFunction (35)** (excluding 5 in `prototype/bindgen/`):

- [ ] `internal/cli/output.go` — `RenderMutationTo`, `RenderTo`
- [ ] `internal/cli/viper.go` — `BindFlagsWithPrefix`, `Get`, `GetBool`, `GetInt`, `GetStringSlice`, `GetStringMap`,
  `ToolConfigPath`, `ConfigFileUsed`, `AllSettings`, `Debug`
- [ ] `internal/cli/xdg.go` — `BashCompletionPath`, `ZshCompletionPath`, `FishCompletionPath`
- [ ] `pkg/document/document.go` — `WithIndent`
- [ ] `internal/e2e/harness.go` — `DefaultTestConfig`, `LoadTestConfig`, `CreateProvider`
- [ ] `internal/execution/hooks.go` — `NewHookRegistry`
- [ ] `internal/lore/onboard/onboard.go` — `WriteManifest`
- [ ] `internal/lorepackage/search.go` — `DefaultSearchOptions`
- [ ] `internal/pwsh/pwsh.go` — `Bootstrap`
- [ ] `internal/signing/aws_kms.go` — `VerifyAWSKMS`
- [ ] `internal/signing/azure_kv.go` — `VerifyAzureKV`
- [ ] `internal/signing/gpg.go` — `VerifyGPG`
- [ ] `internal/writ/graph_builder.go` — `BuildTree`, `NewUpgradeGraphBuilder`, `NewReconcileGraphBuilder`,
  `NewAdoptGraphBuilder`, `NewMigrateGraphBuilder`
- [ ] `internal/writ/identity/identity.go` — `LoadIdentitiesFromPaths`
- [ ] `internal/writ/secrets/crypto.go` — `DecryptFile`
- [ ] `internal/writ/segment/matcher.go` — `MatchAllProjects`, `GroupByProject`, `ProjectNames`
- [ ] `pkg/op/convert.go` — `AnyToStarlarkValue`, `StringSliceToList`

**GoUnusedParameter (30)**:

- [ ] `internal/execution/compensation_test.go` line 25 — `p *file.Provider` (×2)
- [ ] `internal/lore/commands.go` line 630 — `args`
- [ ] `internal/lorepackage/git.go` line 120 — `opts SyncOptions`
- [ ] `internal/model/anthropic.go` line 53 — `ctx`
- [ ] `internal/model/gemini.go` line 55 — `ctx`
- [ ] `internal/model/groq.go` line 44 — `ctx`
- [ ] `internal/model/openai.go` line 59 — `ctx`
- [ ] `internal/starlark/plan_root.go` line 196 — `kwargs`
- [ ] `internal/writ/commands.go` line 1361 — `receiptPath`, `layer`, `project`, `verbose`, `dryRun`
- [ ] `pkg/op/action_reflect_test.go` — 10 params across lines 129, 143, 152, 161, 1174, 1179, 1207, 1212, 1242, 1247,
  1275, 1279
- [ ] `pkg/op/announce_test.go` lines 59, 79 — `reg`
- [ ] `pkg/op/platform_darwin.go` line 128 — `url`, `keyURL`
- [ ] `pkg/op/provider/file/resource.go` line 186 — `size int64`

**GoUnusedConst (16)**:

- [ ] `internal/config/config.go` — `VerbosityQuiet`, `VerbosityNormal`, `VerbosityVerbose`
- [ ] `internal/execution/stateview.go` — `EntryPackage`, `EntryFile`
- [ ] `pkg/op/access.go` — `AccessImmediate`, `AccessPlanned`, `AccessBoth`
- [ ] `pkg/op/lifetime.go` — `LifetimeStateless`, `LifetimePhase`, `LifetimeSession`
- [ ] `pkg/op/resource.go` — `SchemeAppNet`, `SchemeGit`, `SchemeMem`, `SchemePackage`, `SchemeService`

**GoUnusedFunction (3)**:

- [ ] `pkg/op/action_reflect_test.go` line 50 — `newActionResource`
- [ ] `pkg/op/provider/file/gitignore/tracker_test.go` lines 200, 210 — `assertContains`, `assertNotContains`

**GoUnusedGlobalVariable (2)**:

- [ ] `internal/lorepackage/schema.go` line 13 — `LifecycleSchema`
- [ ] `pkg/op/provider/file/provider.go` line 29 — `SkipAll`

**GoUnusedType (1)**:

- [ ] `pkg/op/planned_reflect_test.go` line 37 — `stubReadAction`

**Files**: ~30 files across internal/ and pkg/.

### Phase 6: Style & Modernization (42 issues, 6 skipped) — `complete`

**GoRedundantConversion (15)** — `complete`:

- [x] `internal/console/styles.go` lines 22–28 — 7 redundant `lipgloss.Color` conversions
- [x] `internal/starlark/integration_test.go` line 154 — redundant `bool`
- [x] `pkg/op/provider/mem/extract.go` lines 175, 225 — 4 redundant `int32`
- [x] `pkg/op/provider/starcode/integration_test.go` line 129 — redundant `bool`
- [x] `pkg/op/receiver_reflect_test.go` lines 198, 239 — 2 redundant `starlark.Tuple`

**GoSimplifyWithNew (11)** — Go 1.26 `new()` syntax — `complete`:

- [x] `internal/cli/help.go` line 60, `internal/cli/man.go` line 63, `internal/cli/selfinstall.go` line 325 — `&now` → `new(time.Now())`
- [x] `internal/execution/preflight_test.go` line 83 — `&base` → `new(op.NewResourceBase(...))`
- [x] `internal/signing/azure_kv.go` line 204 — N/A (file removed in prior cleanup)
- [x] `pkg/op/output_test.go` line 694 — `&base` → `new(NewResourceBase(...))`
- [x] `pkg/op/resource_catalog.go` line 42 — `&base` → `new(NewResourceBase(uri))`
- [x] `pkg/op/resource_catalog_test.go` lines 196, 267 — `&base` → `new(NewResourceBase(...))`
- [x] `pkg/op/resource_test.go` line 69 — `&base` → `new(NewResourceBase(...))`
- [x] `pkg/op/starvalue_marshal_test.go` line 221 — `&s` → `new("hello")`

**GoMixedReceiverTypes (6)** — `Path` struct in `pkg/op/root.go` — `skipped` (false positive):

- [x] `pkg/op/root.go` lines 49, 52, 55, 58, 61, 81 — value receivers on getters, pointer
  receiver on `UnmarshalJSON` is correct Go idiom. Changing to all-pointer would alter copy
  semantics. No action needed.

**GoErrorsAsToAsType (5)** — Go 1.26 `errors.AsType` — `complete`:

- [x] `internal/cli/output.go` line 82 — `errors.As` → `errors.AsType[*exitError]`
- [x] `internal/cli/viper.go` line 87 — `errors.As` → `errors.AsType[viper.ConfigFileNotFoundError]`
- [x] `internal/pwsh/pwsh.go` line 273 — `errors.As` → `errors.AsType[*exec.ExitError]`
- [x] `internal/signing/gcp_kms_test.go` line 217 — N/A (file removed in prior cleanup)
- [x] `pkg/op/platform_helpers.go` line 36 — `errors.As` → `errors.AsType[*exec.ExitError]`

**GoDeprecation (2)** — deprecated `Parse` calls — `complete`:

- [x] `pkg/op/provider/mem/extract.go` lines 163, 213

**GoPreferNilSlice (2)** — empty slice literal → nil — `complete`:

- [x] `internal/tools/docgen/template.go` lines 155, 170

**GoReservedWordUsedAsName (1)** — `complete`:

- [x] `pkg/op/triad_test.go` line 328 — variable named `new`

**Files**: ~15 files across internal/ and pkg/.

## Generated Code

All generated files (`pkg/op/provider/*/gen/*.go`) have **zero** inspection issues. The
files `pkg/op/receiver_reflect.go` and `pkg/op/planned_reflect.go` are hand-written (no
`DO NOT EDIT` header) despite containing references to "auto-generated" bridges in doc
comments. They may be edited directly.

If any future phase discovers issues in generated `gen/` files, the fix must go into the
code-generation templates — never into the generated output.

## Verification

After each phase:

1. `make check` — must pass (vet, lint, test, complexity)
2. Re-export GoLand inspections and confirm the addressed category counts drop to zero
3. Grep for `legacy|backward|compat|deprecated` — remove any matches (per CLAUDE.md)

## Open Questions

- [x] ~~Some "unused" exported functions (e.g., `VerifyAWSKMS`, `VerifyAzureKV`, `VerifyGPG`,
  graph builders) may be needed by upcoming features~~ — **Resolved:** All have zero callers;
  graph builders are stubs returning "not yet implemented." Safe to delete (greenfield).
- [x] ~~The `ctx` parameters flagged as unused in model providers (`anthropic.go`, `gemini.go`,
  `groq.go`, `openai.go`) may be interface-required~~ — **Resolved:** All four implement
  `Provider.Available(ctx context.Context) bool`. Keep params, prefix with `_`.
- [x] ~~The unused constants in `pkg/op/` (`Access*`, `Lifetime*`, `Scheme*`) may be part of
  iota groups where removing members changes values~~ — **Resolved:** All are string constants,
  not iota. Deletion is safe with no value-shift risk.
