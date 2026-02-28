# Known Bugs

## #165: compensateWrite missing nil guard on undo parameter — RESOLVED

`compensateWrite` correctly uses `op.ExtractUndo[Tombstone]`, which handles nil maps without panic and returns an error on missing/invalid state. This is the intended behavior — silent no-ops on bad compensation state hide real problems.

The original bug report compared `compensateWrite` against `CompensateBackup`, which silently returns nil on empty state. `CompensateBackup` is the anachronistic one — it predates the `ExtractUndo` pattern and should be updated to match. See issue #168.

## #166: CompensateCopy does not restore file mode on existing files — RESOLVED

Resolved by the tombstone recovery rewrite. `CompensateCopy` now delegates to `compensateWrite`, which uses `os.Rename` (move-to-recovery / restore-from-recovery) instead of `os.WriteFile`. `os.Rename` preserves the original file's metadata including permissions.

## #167: TestBuild_WithNativePMPackage panics — unregistered action: pkg.install

`internal/lore.TestBuild_WithNativePMPackage` panics at `builder.go:408` with `unregistered action: pkg.install`. The `pkg` provider's `ActionRegistrar` is not loaded in the test context. The test creates a `Planner` without registering the `pkg` provider actions, then calls `PlanByName` which reaches `addNativePMNodes` → `reg.MustGet("pkg.install")` → panic.

**Branch**: `feature/binding-unification`

## #168: TestLoadIntegration fails — undefined: ui

`internal/starlark.TestLoadIntegration` fails at `load_test.star:11:31` with `undefined: ui`. The test's Starlark globals do not include the `ui` provider binding. The `ui` provider was removed from the old hand-coded global set during binding unification but is not yet re-registered via the new `BindingSet` API in the test harness.

**Branch**: `feature/binding-unification`
