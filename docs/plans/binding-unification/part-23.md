# Phase 8 Part 23: Programmatic AttrNames + CI Fixes

## Context

Phase 8 PR #155 fails CI on two checks:

1. **quality-gate** — Two test failures in `internal/lore`:
   - `TestBuildPhased_LorePackageMultiPhase`: `plan.shell has no .exec attribute`
   - `TestBuildPhased_OutputFunctions`: `undefined: note`

2. **knowledge-extract** — 7 contract violations: `_ATTR_NAMES` in `extract.star`
   is a hand-maintained dict that went stale after Part 9a regeneration.

Root cause of (2): `_ATTR_NAMES` is the only hand-maintained authoritative
cross-reference in the pipeline. It MUST be replaced with programmatic extraction.

## Governing Principle

**ALL code in `*_gen.go` files MUST be generated. NO hand-editing. NO secondary
files.** Predicates are bool-returning methods on providers — the generator
handles them like any other method.

## Completed Work

### Done: Generator fixes (noblefactor-ops)

Changes to `receiver_go_gen.go`:
- `validateReturnSignature` accepts `bool` as valid return type
- `tplGraphReturn` handles bool predicate case: `return <call>, nil, nil`
- `generateDescriptor` field renamed `Category` → `Provider`
- `descriptorFromValue` accepts `"provider"` key
- Added `needsImport` template function to `genTemplateFuncs`

Changes to `receiver_go.go`:
- Added `go.type_doc(path, name)` — extracts doc comment for named type
  (preserves directive lines like `//devlore:plannable` that `Text()` strips)
- Registered in `Attr()` and `AttrNames()`

Changes to test files:
- `receiver_go_ast_test.go`: Added `TestGoTypeDoc`
- `receiver_go_gen_test.go`: Fixed `Category` → `Provider` in templates,
  `"category"` → `"provider"` in descriptors, updated error message assertions

All noblefactor-ops tests pass.

### Done: Rename `shell.Provider.Shell` → `Exec`

Produces `plan.shell.exec` and `shell.exec` instead of redundant `plan.shell.shell`.

### Done: Wire UI output functions in `builder.go`

`prepareScriptEnv()` wires `note`, `warn`, `error`, `success`, `fail` via
`ui.Provider` with `makeUIBuiltin` / `makeUIErrorBuiltin` helpers.

### Done: Predicate methods on providers

Added bool-returning methods to providers. The generator produces plan
bindings and graph actions for these automatically — no hand-editing.

- `file/provider.go`: `Exists(path string) bool`, `IsDir(path string) bool`
- `service/provider.go`: `Exists(name string) bool`, `Running(name string) bool`, `Enabled(name string) bool`
- `pkg/provider.go`: `Installed(name string) bool`, `NotInstalled(name string) bool`, `VersionGTE(name, version string) bool`

Deleted `predicate_runtime.go` (factory functions replaced by provider methods).
Deleted `predicate.go` (Predicate interface replaced by bool slot on Choose).

### Done: Flow control cleanup

- `action.go`: Deleted `ChooseCase`, `ChooseUndoState`, `GatherUndoState`, `IterationUndo`
- `flow/choose.go`: Rewritten to slot-based boolean model (`when` bool, `then`/`else` phase IDs)
- `flow/gather.go`: Moved undo types here (unexported)
- `flow/wait_until.go`: Uses `PredicateFunc func(any) (bool, error)` instead of `Predicate` interface
- `plan_root.go`: `choose()` accepts `*Output` (predicate promise), wires via `FillSlot`
- `flow_test.go`: Rewritten for slot model

### Done: Regeneration (20 of 22 files)

All 10 plannable providers regenerated:
- 10 `plan_*_gen.go` files in `internal/starlark/`
- 10 `actions_gen.go` files in `internal/execution/provider/*/`

`content/provider.go`: `Literal` return changed to `([]byte, error)`.
`plan_package_gen.go` → `plan_pkg_gen.go` (filename derived from directory name).

## Remaining Work

### Step 1: Rebuild `star` binary

Rebuild from noblefactor-ops (has `go.type_doc`, bool support, Provider field,
`needsImport`, test fixes).

### Step 2: Regenerate UI provider + `register_gen.go`

UI provider has no `//devlore:plannable` directive, so `go.type_doc` auto-detects
it as `realtime_receiver`. This was blocked before `go.type_doc` existed.

`register_gen.go` aggregates all per-provider `Register()` functions. The UI
provider does NOT have a `Register()` — it uses the `realtime_receiver` template.
This file must be regenerated or restored to match the current set of plannable
providers.

### Step 3: Add `go.return_strings(scope)` to noblefactor-ops

New AST primitive for Step 4. Extracts `[]string{...}` literal elements from
a return statement. Follows `extractReturnString` pattern.

**`receiver_go.go`:**
1. Add `extractReturnStrings(body *ast.BlockStmt) []string`
2. Add `goReturnStrings` Starlark method
3. Register in `Attr()` and `AttrNames()`

**`receiver_go_ast_test.go`:**
4. Add `TestGoReturnStrings`

Rebuild `star` after.

### Step 4: Eliminate `_ATTR_NAMES` in `extract.star`

**`star/.../Knowledge/commands/extract.star`:**

1. Delete `_ATTR_NAMES` dict
2. Add `_build_attr_names_from_source(path)`:
   - `go.methods(path, name="AttrNames")` → for each, get `receiver_type` + `scope`
   - `go.return_strings(scope)` → extract `[]string{...}` literal
   - `go.methods(path, name="Attr")` → find `MakeAttr`/`NewBuiltin` calls to derive prefix
   - Correlate by `receiver_type` → `{prefix: [name, ...]}`
3. Update `_validate_attr_names` to accept `attr_names` parameter
4. Update `_parse_devlore_api` to call builder and pass result

### Step 5: Build + test + extract

1. `go build ./...`
2. `go test ./...`
3. `star devlore knowledge extract --domain all --source=. --target=../devlore-registry`

## Cross-Repo Coordination

| Repo | Steps | Dependency |
|------|-------|------------|
| noblefactor-ops | 1 (rebuild), 3 (`go.return_strings`) | Must rebuild before step 2, 4 |
| devlore-cli | 2 (UI + register) | Needs rebuilt `star` |
| devlore-cli | 4 (extract.star) | Needs `go.return_strings` |
| devlore-cli | 5 (verify) | After all steps |

## Notes

- `_KNOWN_PROPERTIES` stays — documentation metadata, not a cross-reference
- `compensation_test.go` restored and updated: Copy slots changed from `"source"` to `"content"` ([]byte) to match regenerated `file.Copy` action
- `hand-edited-predicate-source` is an untracked backup directory — delete before commit
