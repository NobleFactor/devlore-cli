---
title: "GoLand Inspection Cleanup — Go Code Issues"
issue: TBD
status: in-progress
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

### Phase 1: Potential Runtime Bugs (16 issues)

Fix issues that could cause panics or silently wrong behavior.

**GoMaybeNil (14)** — add nil guards or `require.NotNil` assertions:

- [ ] `internal/cli/selfinstall_test.go` lines 65, 89 — `err` dereference after nil check
- [ ] `internal/console/console_test.go` lines 155, 160, 173 — `session.Next()` may return nil
- [ ] `internal/execution/phase_test.go` lines 318, 321, 324, 327, 469 — `graph.PhaseByID()` may return nil
- [ ] `pkg/op/provider/starcomplexity/provider.go` line 99 — `opts.Parse()` may return nil
- [ ] `pkg/op/provider/starindex/provider.go` line 132 — `opts.Parse()` may return nil

**GoBoolExpressions (2)** — conditions that are always false:

- [ ] `internal/cli/output_test.go` line 84 — `DefaultFormat != "json"` always false
- [ ] `internal/writ/segment/segment_test.go` line 398 — `EnvVarPrefix != "WRIT_SEGMENT_"` always false

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

### Phase 3: Missing Switch Cases (11 issues)

Add exhaustive `case` branches (or explicit `default` with panic) for iota-const switches.

- [ ] `internal/cli/output.go` lines 319, 356 — two switches
- [ ] `internal/console/model.go` lines 126, 168, 223, 293 — four switches
- [ ] `internal/execution/executor.go` line 804 — one switch
- [ ] `internal/writ/commands.go` line 614 — one switch
- [ ] `internal/writ/migrate/session.go` line 113 — one switch
- [ ] `internal/writ/migrate/session_test.go` line 304 — one switch
- [ ] `pkg/op/provider/file/gitignore/tracker.go` line 118 — one switch

**Files**:

- `internal/cli/output.go` — Modify
- `internal/console/model.go` — Modify
- `internal/execution/executor.go` — Modify
- `internal/writ/commands.go` — Modify
- `internal/writ/migrate/session.go` — Modify
- `internal/writ/migrate/session_test.go` — Modify
- `pkg/op/provider/file/gitignore/tracker.go` — Modify

### Phase 4: Unhandled Error Results (117 issues)

The largest category. Many are `defer f.Close()` or `fmt.Fprintf` where the error is
discarded. Strategy:

- **`Close` errors (defer)**: Use a helper or assign to named return: `defer func() { _ = f.Close() }()` or check with
  `closeErr`.
- **`fmt.Fprintf/Fprintln/Fprint`**: Assign to `_` explicitly where the write target is stdout/stderr. For other
  writers, check the error.
- **`os.Remove/RemoveAll`**: Check the error or assign to `_` with comment if best-effort.
- **`t.Setenv` / `os.Setenv` in tests**: These are legitimate `os.Setenv` calls — check the error.

**Production code (77 errors across 18 files)**:

- [ ] `internal/lore/onboard/onboard.go` — 11 (Close, Fprintf)
- [ ] `internal/writ/migrate/session.go` — 10 (Fprintf, Fprintln)
- [ ] `internal/writ/tree/output.go` — 10 (Fprintf)
- [ ] `pkg/op/provider/archive/provider.go` — 10 (Close, Remove)
- [ ] `cmd/indexgen/main.go` — 5 (Fprintf, Fprintln)
- [ ] `internal/model/config.go` — 5 (Fprint)
- [ ] `internal/devloretest/commands.go` — 4 (Close)
- [ ] `pkg/op/provider/file/provider.go` — 4 (Close)
- [ ] `pkg/op/provider/mem/extract.go` — 4 (Fprintf)
- [ ] `internal/execution/executor.go` — 3 (Close)
- [ ] `pkg/op/provider/file/gitignore/tracker.go` — 3 (Close)
- [ ] `internal/e2e/testrunner/runner.go` — 2 (Close, RemoveAll)
- [ ] `cmd/docgen/main.go` — 1 (Fprintf)
- [ ] `internal/cli/man.go` — 1 (Remove)
- [ ] `internal/cli/receipts.go` — 1 (Remove)
- [ ] `internal/execution/flow/degraded.go` — 1 (Fprintln)
- [ ] `pkg/op/provider/appnet/provider.go` — 1 (Close)
- [ ] `pkg/op/provider/file/resource.go` — 1 (Close)

**Test code (40 errors across 10 files)**:

- [ ] `internal/credentials/credentials_test.go` — 12 (Setenv)
- [ ] `pkg/op/provider/archive/provider_test.go` — 5 (Close)
- [ ] `pkg/op/root_test.go` — 5 (Close)
- [ ] `cmd/devlore-test/cli_test.go` — 4 (Fprintf, RemoveAll)
- [ ] `internal/lore/builder_test.go` — 4 (Close)
- [ ] `internal/execution/preflight_test.go` — 3 (Close, Shadow)
- [ ] `internal/cli/config_test.go` — 2 (Close, ReadFrom)
- [ ] `pkg/op/provider/file/provider_test.go` — 2 (Close)
- [ ] `pkg/op/triad_test.go` — 2 (Close, RemoveAll)
- [ ] `pkg/op/recovery_site_test.go` — 1 (RemoveAll)

**Files**: All 28 files listed above.

### Phase 5: Dead Code Removal (92 issues)

Delete unused exported functions, unexported functions, constants, types, parameters, and
global variables. Per the governing principle: this is greenfield — no legacy users.

**GoUnusedExportedFunction (35)** (excluding 5 in `prototype/bindgen/`):

- [ ] `internal/cli/output.go` — `RenderMutationTo`, `RenderTo`
- [ ] `internal/cli/viper.go` — `BindFlagsWithPrefix`, `Get`, `GetBool`, `GetInt`, `GetStringSlice`, `GetStringMap`,
  `ToolConfigPath`, `ConfigFileUsed`, `AllSettings`, `Debug`
- [ ] `internal/cli/xdg.go` — `BashCompletionPath`, `ZshCompletionPath`, `FishCompletionPath`
- [ ] `internal/document/document.go` — `WithIndent`
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

### Phase 6: Style & Modernization (42 issues) — `in-progress`

**GoRedundantConversion (15)** — `complete`:

- [x] `internal/console/styles.go` lines 22–28 — 7 redundant `lipgloss.Color` conversions
- [x] `internal/starlark/integration_test.go` line 154 — redundant `bool`
- [x] `pkg/op/provider/mem/extract.go` lines 175, 225 — 4 redundant `int32`
- [x] `pkg/op/provider/starcode/integration_test.go` line 129 — redundant `bool`
- [x] `pkg/op/receiver_reflect_test.go` lines 198, 239 — 2 redundant `starlark.Tuple`

**GoSimplifyWithNew (11)** — Go 1.26 `new()` syntax:

- [ ] `internal/cli/help.go` line 60, `internal/cli/man.go` line 63, `internal/cli/selfinstall.go` line 325 — `&now`
- [ ] `internal/execution/preflight_test.go` line 83 — `&base`
- [ ] `internal/signing/azure_kv.go` line 204 — `&algorithm`
- [ ] `pkg/op/output_test.go` line 694 — `&base`
- [ ] `pkg/op/resource_catalog.go` line 42 — `&base`
- [ ] `pkg/op/resource_catalog_test.go` lines 196, 267 — `&base`
- [ ] `pkg/op/resource_test.go` line 69 — `&base`
- [ ] `pkg/op/starvalue_marshal_test.go` line 221 — `&s`

**GoMixedReceiverTypes (6)** — `Path` struct in `pkg/op/root.go`:

- [ ] `pkg/op/root.go` lines 49, 52, 55, 58, 61, 81 — standardize receivers to pointer

**GoErrorsAsToAsType (5)** — Go 1.26 `errors.AsType`:

- [ ] `internal/cli/output.go` line 82
- [ ] `internal/cli/viper.go` line 87
- [ ] `internal/pwsh/pwsh.go` line 273
- [ ] `internal/signing/gcp_kms_test.go` line 217
- [ ] `pkg/op/platform_helpers.go` line 36

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
