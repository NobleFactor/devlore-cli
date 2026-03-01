# Wire Projection Library into Star

## Context

Binding unification (Phases 1-9) and the type ownership extraction established
`pkg/projection/` as the public API for Starlark binding infrastructure. Star
(in `noblefactor-ops`) has its own receiver infrastructure (`BaseReceiver`,
`MakeAttr`, `FileReceiver`, `GoReceiver`, UI builtins) that duplicates what the
projection library provides. This plan replaces star's `file.*`, `go.*`, and
`ui.*` receivers with receivers from `pkg/projection/` in devlore-cli.

This also implements the `//devlore:access=immediate|planned|both` directive
described in `docs/architecture/projected-provider-api.md`, replacing the
binary `//devlore:plannable` flag.

## Design Decisions

1. **`//devlore:access=` replaces `//devlore:plannable`.** Struct-level
   directive. Values: `immediate`, `planned`, `both`. The generator reads this
   and selects which templates to render.

2. **Template rename.** `realtime_receiver.go.template` →
   `immediate_receiver.go.template`, `plan_receiver.go.template` →
   `planned_receiver.go.template`.

3. **Merged file.Provider.** The union of star's file operations and
   devlore-cli's file.Provider becomes a single Provider with
   `//devlore:access=both`. It lives in `pkg/projection/provider/file/` so
   star can import it.

4. **ui.Provider moves to `pkg/`.** `pkg/projection/provider/ui/` with
   `//devlore:access=immediate`. Star imports and uses it.

5. **go receiver moves to `pkg/`.** `pkg/projection/provider/goast/` with
   `//devlore:access=immediate`. Hand-coded receiver (not generated) because
   its 15 methods return complex nested types (AST analysis → starlark.Dict)
   that don't fit the generator's type conversion. Embeds
   `projection.Receiver`. Package name is `goast` since `go` is reserved.
   Unlike file/ui Providers, this imports `go.starlark.net/starlark` because
   its methods work with Starlark types directly.

6. **Star replaces its receiver infrastructure.** `BaseReceiver` →
   `projection.Receiver`, `MakeAttr` → `projection.MakeAttr`, etc.
   `FileReceiver`, UI builtins, and `GoReceiver` are replaced by receivers
   from the imported Providers.

## Merged file.Provider Methods

The Provider in `pkg/projection/provider/file/` has `//devlore:access=both`.
All methods appear in both immediate and planned projections.

### Query methods
- `Read(path string) (string, error)` — reads file contents as string
- `Exists(path string) (bool, error)` — checks path existence
- `IsDir(path string) (bool, error)` — checks if path is a directory
- `IsFile(path string) (bool, error)` — checks if path is a regular file
- `Glob(pattern string) ([]string, error)` — finds matching files
- `List(path string) ([]DirEntry, error)` — lists directory contents

### Path utilities
- `Join(parts ...string) string` — joins path components
- `BaseName(path string) string` — returns filename (filepath.Base)
- `Parent(path string) string` — returns parent directory (filepath.Dir)

### Mutating methods (compensable)
- `Link(source, path string) (string, map[string]any, error)`
- `Copy(path string, mode os.FileMode, content []byte) (string, map[string]any, error)`
- `Write(content, path string, mode os.FileMode) (string, map[string]any, error)`
- `Backup(path, backupSuffix string) (string, map[string]any, error)`
- `Unlink(path string, prune bool, pruneBoundary string) (string, map[string]any, error)`
- `Remove(path string, prune bool, pruneBoundary string) (string, map[string]any, error)`
- `Move(gitMv func(src, dst string) error, source, path string) (string, map[string]any, error)`

### Mutating methods (non-compensable)
- `Mkdir(path string, mode os.FileMode) (string, error)`
- `RemoveAll(path string) error`

### Star-specific file extensions (stay in noblefactor-ops)
- `walk_tree(root, callback, gitignore?)` — requires Starlark callback, can't be generated
- gitignore filtering on `glob` and `list` — star wraps the Provider methods

## go Receiver Methods

The receiver in `pkg/projection/provider/goast/` has `//devlore:access=immediate`.
Hand-coded — not generated. Methods take/return Starlark types directly because
they produce complex nested structures (AST → starlark.Dict) that the generator
can't convert. Starlark namespace remains `go.*`.

### AST analysis
- `Funcs(path, name?) → []FuncInfo` — function declarations
- `Methods(path, name?, receiverType?, returns?) → []MethodInfo` — method declarations
- `Structs(path) → []StructInfo` — struct definitions with fields
- `Calls(scope, name?) → []CallInfo` — function/method calls in scope
- `Composites(scope, type?) → []CompositeInfo` — composite literals in scope
- `ConstGroups(path, type?) → []ConstGroupInfo` — typed constant groups
- `TypeDoc(path, name?) → string` — type documentation comments
- `RawString(scope) → string` — first backtick string literal in scope
- `ReturnString(scope) → string` — return statement string literal
- `ReturnStrings(scope) → []string` — return statement string slice

### Analysis
- `Deps(path) → DepsInfo` — import/dependency analysis
- `Metrics(path) → MetricsInfo` — code metrics (LOC, SLOC, etc.)

### Code generation
- `Generate(template, descriptor) → string` — render Go code from template
- `Mapping(descriptor) → string` — render YAML operation mapping
- `Template(name) → string` — get builtin template content

Note: `builtinTemplates` key `"realtime_receiver"` → `"immediate_receiver"`.
`RealtimeReceiverTemplate` → `ImmediateReceiverTemplate`. The embedded template
content references `Receiver`, `NewReceiver`, `MakeAttr`, `NoSuchAttrError` —
these become `projection.Receiver`, etc. in the generated output.

### Star-specific go extensions (stay in noblefactor-ops)
None — all methods move to the projection library.

## ui.Provider Methods

The Provider in `pkg/projection/provider/ui/` has `//devlore:access=immediate`.

- `Note(message string)` — informational output
- `Warn(message string)` — warning output
- `Error(message string)` — error output
- `Success(message string)` — success output
- `Fail(message string) error` — fatal error (returns error to halt)

## Implementation Steps

### Step 1: Move Providers to `pkg/` (devlore-cli)

Move `file.Provider`, `ui.Provider`, and create `go.Provider`:

- `internal/execution/provider/file/provider.go` →
  `pkg/projection/provider/file/provider.go`
- `internal/execution/provider/ui/provider.go` →
  `pkg/projection/provider/ui/provider.go`
- Create `pkg/projection/provider/goast/` — hand-coded receiver moved from
  noblefactor-ops `internal/starlark/receiver_go.go` + `receiver_go_gen.go`.
  Package `goast` (since `go` is reserved). Starlark namespace stays `go.*`.

`internal/execution/provider/file/` keeps `actions_gen.go` and imports the
Provider from `pkg/`. `internal/execution/provider/ui/` is removed (no actions).

### Step 2: Add immediate methods to file.Provider (devlore-cli)

Add to `pkg/projection/provider/file/provider.go`:
- `Read(path) (string, error)` — replaces `Source` (returns string not []byte)
- `IsFile(path) (bool, error)`
- `Join(parts ...string) string`
- `BaseName(path) string`
- `Parent(path) string`
- `Glob(pattern) ([]string, error)` — recursive ** support, no gitignore
- `List(path) ([]DirEntry, error)` — returns name, path, isDir
- `RemoveAll(path) error`

Rename `Source` → `Read` (returns string). The planned receiver slot name stays
`source` for backward compatibility — no, greenfield, rename the slot too.

### Step 3: Replace `//devlore:plannable` with `//devlore:access=` (devlore-cli)

Update `generate.star`:
- Parse `//devlore:access=immediate|planned|both` from type doc
- `planned` → generate planned receiver + graph actions (was `plannable`)
- `immediate` → generate immediate receiver only (was no directive)
- `both` → generate all three

Update all 9 Providers:
- `file.Provider` → `//devlore:access=both`
- `ui.Provider` → `//devlore:access=immediate`
- All others (git, pkg, service, shell, template, archive, encryption, net) →
  `//devlore:access=planned`

### Step 4: Rename templates (devlore-cli)

In `star/extensions/com.noblefactor.devlore.Actions/templates/`:
- `realtime_receiver.go.template` → `immediate_receiver.go.template`
- `plan_receiver.go.template` → `planned_receiver.go.template`

Update `generate.star` to reference new template filenames.

### Step 5: Regenerate all gen files (devlore-cli)

Rename generated files to reflect terminology:
- `plan_*_gen.go` → `planned_*_gen.go` (9 planned receivers)
- `receiver_*_gen.go` → `immediate_*_gen.go` (immediate receivers)
- New: `immediate_file_gen.go` (immediate receiver for file.Provider)
- Existing: `immediate_ui_gen.go` (renamed from `receiver_ui_gen.go`)
- 9 `actions_gen.go` in providers (unchanged)

### Step 6: Update internal consumers (devlore-cli)

- `internal/execution/provider/file/actions_gen.go` — import Provider from
  `pkg/projection/provider/file`
- `internal/execution/provider/file/provider_test.go` — import from `pkg/`
- All files that import `internal/execution/provider/file` or
  `internal/execution/provider/ui` — update import paths
- `internal/starlark/plan_root.go` — UI receiver uses imported Provider

### Step 7: Replace receiver infrastructure (noblefactor-ops)

Add `github.com/NobleFactor/devlore-cli` to `go.mod`.

In `internal/starlark/`:

- **`receiver.go`**: Delete `BaseReceiver`, `MakeAttr`, `NoSuchAttrError`,
  `BuiltinFunc`. Replace with imports from `projection`:
  ```go
  type Receiver = projection.Receiver
  var MakeAttr = projection.MakeAttr
  var NoSuchAttrError = projection.NoSuchAttrError
  type BuiltinFunc = projection.BuiltinFunc
  ```
  Or update all call sites directly.

- **`receivers.go`**: Remove `File` and `Go` singletons. Add Provider-based
  construction.

- **`receiver_file.go`**: Rewrite. The receiver embeds `projection.Receiver`
  and holds a `*file.Provider` from `pkg/projection/provider/file`. Most
  methods delegate to the Provider. Star-specific methods (`walk_tree`,
  gitignore-aware `glob`/`list`) stay as local extensions that wrap the
  Provider's base methods.

- **`receiver_go.go`** + **`receiver_go_gen.go`**: Delete. Replaced by
  `go.Provider` imported from `pkg/projection/provider/go/`. The hand-coded
  receiver now lives in devlore-cli.

- **`runtime.go`**: Replace `noteBuiltin`/`warnBuiltin`/`errorBuiltin`/
  `successBuiltin`/`failBuiltin` with a `UiReceiver` constructed from
  `ui.Provider`. Replace `Go` singleton with imported `go.Provider`.
  Update `buildPredeclared()`:
  ```go
  // Before:
  "note": starlark.NewBuiltin("note", noteBuiltin),
  // After (ui as receiver):
  "note": projection.MakeAttr("note", uiRecv.note),
  ```
  Or register `ui` as a namespace receiver like `file`.

### Step 8: Build and verify

**devlore-cli:**
1. `make build`
2. `make test`
3. Verify `pkg/projection/provider/file/` imports only stdlib
4. Verify `pkg/projection/provider/ui/` imports only stdlib
5. Verify `pkg/projection/provider/goast/` imports only stdlib + starlark + yaml
6. Verify zero `//devlore:plannable` remaining (replaced by `//devlore:access=`)
7. Verify zero `realtime_receiver` or `plan_receiver` template references

**noblefactor-ops:**
1. `go build ./...`
2. `go test ./...`
3. Run `star devlore actions generate` on all providers — verify output unchanged
4. Run `star devlore actions validate` — verify no contract violations
5. Verify `BaseReceiver` no longer exists locally
6. Verify `receiver_go.go` and `receiver_go_gen.go` are gone

## Files Modified

### devlore-cli

| File | Action |
|------|--------|
| `pkg/projection/provider/file/provider.go` | **Create** — merged file Provider |
| `pkg/projection/provider/ui/provider.go` | **Create** — moved from internal |
| `pkg/projection/provider/goast/` | **Create** — hand-coded receiver moved from noblefactor-ops |
| `internal/execution/provider/file/provider.go` | **Delete** — moved to pkg |
| `internal/execution/provider/ui/provider.go` | **Delete** — moved to pkg |
| `internal/execution/provider/file/actions_gen.go` | **Modify** — import from pkg |
| `star/.../commands/generate.star` | **Modify** — parse `//devlore:access=` |
| `star/.../templates/immediate_receiver.go.template` | **Rename** from realtime |
| `star/.../templates/planned_receiver.go.template` | **Rename** from plan |
| All 9 Provider source files | **Modify** — add `//devlore:access=` directive |
| ~20 gen files | **Regenerate** |
| ~10 consumer files | **Modify** — update import paths |

### noblefactor-ops

| File | Action |
|------|--------|
| `go.mod` | **Modify** — add devlore-cli dependency |
| `internal/starlark/receiver.go` | **Modify** — alias or delete local types |
| `internal/starlark/receivers.go` | **Modify** — Provider-based construction |
| `internal/starlark/receiver_file.go` | **Rewrite** — projection.Receiver + file.Provider |
| `internal/starlark/receiver_go.go` | **Delete** — moved to devlore-cli pkg |
| `internal/starlark/receiver_go_gen.go` | **Delete** — moved to devlore-cli pkg |
| `internal/starlark/runtime.go` | **Modify** — UiReceiver replaces builtins, Go receiver from pkg |

## Risks

**Import cycle**: `devlore-cli/pkg/projection/provider/file` → stdlib only.
`noblefactor-ops` → `devlore-cli/pkg/projection/`. No cycle possible.

**Licensing**: devlore-cli is SSPL-1.0, noblefactor-ops is MIT. Importing
SSPL code into an MIT project requires review.

**Generator bootstrap**: Star generates receivers for Providers it imports.
If the generated code is checked in, no bootstrap issue. If star needs to
regenerate itself, a two-phase build is needed.

**Star-specific features**: gitignore-aware glob/list and walk_tree with
Starlark callbacks stay in star. The Provider methods provide base functionality
that star extends.
