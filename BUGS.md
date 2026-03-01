# Known Bugs

## #165: compensateWrite missing nil guard on undo parameter — RESOLVED

`compensateWrite` correctly uses `op.ExtractUndo[Tombstone]`, which handles nil maps without panic and returns an error on missing/invalid state. This is the intended behavior — silent no-ops on bad compensation state hide real problems.

The original bug report compared `compensateWrite` against `CompensateBackup`, which silently returns nil on empty state. `CompensateBackup` is the anachronistic one — it predates the `ExtractUndo` pattern and should be updated to match. See issue #168.

## #166: CompensateCopy does not restore file mode on existing files — RESOLVED

Resolved by the tombstone recovery rewrite. `CompensateCopy` now delegates to `compensateWrite`, which uses `os.Rename` (move-to-recovery / restore-from-recovery) instead of `os.WriteFile`. `os.Rename` preserves the original file's metadata including permissions.

## #167: TestBuild_WithNativePMPackage panics — unregistered action: pkg.install — RESOLVED

Resolved by updating `register.go` to import `provider/pkg/gen` instead of `provider/pkg`. The `init()` registrations moved to `gen/` subdirectories during binding unification but the import paths were not updated.

## #168: TestLoadIntegration fails — undefined: ui

`internal/starlark.TestLoadIntegration` fails at `load_test.star:11:31` with `undefined: ui`. The test's Starlark globals do not include the `ui` provider binding. The `ui` provider was removed from the old hand-coded global set during binding unification but is not yet re-registered via the new `BindingSet` API in the test harness.

**Branch**: `feature/binding-unification`

## #169: TestBuildPhased_LorePackageMultiPhase fails — shell.exec missing argument for output

`internal/lore.TestBuildPhased_LorePackageMultiPhase` fails with `exec: missing argument for output`. The shell provider's reflected planned binding requires all params from the `Params` map (including `output`), but the Starlark test script only passes `command=`.

**Branch**: `feature/binding-unification`

## #170: WrapReceiver does not expose WalkTree — callback params unsupported

`TestImmediateBindings` fails with `file has no .walk_tree attribute`. `WalkTree` takes a Starlark callable (`fn`) as a parameter, which the params generator cannot handle. Methods with callable params need an `Override` or the reflection bridge needs `starlark.Callable` support.

**Branch**: `feature/binding-unification`

## #171: Planned bindings missing for non-error methods (Exists, IsDir, IsFile, Name, Parent, Join)

`TestPlannedBindings` fails with `plan.file has no .exists attribute`. `RegisterReflectedActions` skips methods without `error` return. These methods are immediate-only but the old planned receiver exposed them as graph node creators. The planned layer needs an alternative path for non-error methods.

**Branch**: `feature/binding-unification`
