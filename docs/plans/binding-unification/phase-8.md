# Phase 8: Plannable Directive, Generator Fixes, Regeneration

**Status**: In Progress

## Context

Phase 8 originally scoped doc comments through the code generation pipeline.
During implementation, scope expanded to address several interrelated issues:

1. **Category removal**: The generator derived `category` from the struct name,
   which broke when all providers unified on `Provider`. Fixed by deriving
   `provider` from the source path.

2. **Receiver cleanup**: Lore/writ planners use plan receivers, not realtime
   receivers. All hand-written realtime receivers were deleted from the planning
   layer. A new UI provider was created for user-facing messaging.

3. **Logger→Writer**: `ctx.Logger` renamed to `ctx.Writer` across both repos.
   The field is an `io.Writer` for user-facing terminal output, not logging.

4. **Plannable directive**: Introduce `//devlore:plannable` comment directive on
   provider structs to declare whether a provider participates in planning. This
   drives template selection in the code generator.

5. **Compensation classification**: Split `Action` into `Action` (forward-only)
   and `CompensableAction` (extends Action with Undo). Added `NotCompensableError`
   sentinel for actions that acknowledge rollback but cannot undo.

6. **Vocabulary cleanup**: Renamed "operation" to "action" across all layers.
   `lorepackage.Operation` → `lorepackage.Action` with four lifecycle actions:
   Deploy, Upgrade, Decommission, Reconcile.

7. **Go bindings**: Design for ensuring all providers are callable from Go, with
   convenience wrappers where needed (UI output functions).

## Completed

| Part | Description | Status |
|------|-------------|--------|
| 1 | Rename `Category`→`Provider` in noblefactor-ops (`receiver_go_gen.go` + tests) | Done |
| 2 | Rename `{{.Category}}`→`{{.Provider}}` in all 3 devlore-cli templates | Done |
| 3 | Fix `generate.star`: derive provider from source path, remove `STRIP_SUFFIXES`, `--category`, `--service` flags; hardcode `struct_name = "Provider"` | Done |
| 4 | Fix `extract.star`: same derivation fix | Done |
| 5 | Fix `graph_actions.go.template`: conditional `os` import, `execution.ChecksumBytes()` | Done |
| 6 | Fix `net.Provider` contract (delete `CompensateDownload`) | Done |
| 7 | Delete junk files (`plan_pkg_gen.go`, `actions_gen.go`) | Done |
| 8 | Add doc comments to hand-written `plan_package_gen.go` | Done |
| 9a | Regenerate plan receivers (9 providers) | Done |
| 9b | Regenerate graph actions (9 providers) | Done |
| 10 | Rename `ctx.Logger`→`ctx.Writer` in noblefactor-ops (type mapping + tests) | Done |
| 11 | Rename `ctx.Logger`→`ctx.Writer` in devlore-cli (action.go, executor.go, gather.go, all tests, hand-written net/content actions) | Done |
| 12 | Delete all hand-written realtime receivers (15 files) + `bindings.go` | Done |
| 13 | Create `internal/execution/provider/ui/provider.go` (Note, Warn, Error, Success, Fail) | Done |
| 14 | ~~Generate `ui/actions_gen.go`~~ Removed — UI is not plannable, no actions. | Struck |
| 15 | Delete `plan_ui_gen.go` (UI is not plannable) | Done |
| 16 | Remove `LogReceiver` usage from `builder.go` | Done |
| 17 | Rebuild `star` binary from worktree source (twice: after Provider rename, after ctx.Writer rename) | Done |
| 18 | Compensation classification: split `Action`/`CompensableAction`, add `NotCompensableError`, update executor, recovery, flow actions, graph_actions.go.template, regenerate all actions_gen.go | Done |
| 19 | Vocabulary cleanup: rename "operation"→"action" across execution, lore, and lorepackage layers (tests, doc comments, field names, parameter names) | Done |
| 19a | Rename `lorepackage.Operation` → `Action`, `OpDeploy/OpUpgrade/OpDecommission` → `Deploy/Upgrade/Decommission`; add `Reconcile` action with phase order scan→repair→verify | Done |
| 19b | Rename `PMOperation` → `PMCommand`, `NativePMAction.Operation` → `.Command`, `phaseToNativePMOp` → `phaseToNativePMCmd` | Done |
| 20 | Add `//devlore:plannable` directive to 9 plannable providers (not UI) | Done |
| 21 | Add `go.type_doc()` to noblefactor-ops GoReceiver + `commentGroupRaw` helper for directive preservation + tests | Done |
| 22 | Update `generate.star`: auto-detect plannable directive, derive template selection (plan_receiver+graph_actions vs realtime_receiver) | Done |
| 23 | Redesign `ui.Provider` (4 config fields: Writer, ProgramName, Silent, Color) + `cli/output.go` convenience wrappers delegating to `ui.Provider` | Done |
| 23a | StringDict purge: migrate all `NewBuiltin` registrations from `StringDict` to `Attr` receivers | Done |
| 24 | Wire UI as realtime receiver in lore (`receiver_ui_gen.go` generated, `builder.go` wires `ui` namespace into globals). Writ uses `cli.Note` directly, which already delegates to `ui.Provider`. | Done |
| 25 | Doc comments in templates (`{{.Doc}}` in all 3 templates) + `--format` flag in `extract.star` | Done |
| 26 | Refactor filesystem bypass callers to use `file.Provider` (`adoptFile`, `linkToLayer`, `moveToLayer`, `Execute`) | Done |

## Remaining

### ~~Part 26: Refactor filesystem bypass callers~~ — Done

Replaced direct `os.*` calls with `file.Provider.*` calls in four functions:

- `adoptFile` (`commands.go`) — `os.MkdirAll`→`fp.Mkdir`, `os.Rename`→`fp.Move`, `os.Symlink`→`fp.Link`
- `linkToLayer` (`migrate_cmd.go`) — `os.MkdirAll`→`fp.Mkdir`, `os.Symlink`→`fp.Link`
- `moveToLayer` (`migrate_cmd.go`) — `os.MkdirAll`→`fp.Mkdir`, `os.Rename`→`fp.Move`
- `Execute` (`execute.go`) — `os.Rename`→`fp.Move`

Not changed (by design): `secrets/crypto.go` (crypto output), `WriteMigratedMarker` (CLI artifact).

### Part 27: Verification

1. `make build`
2. `make test`
3. Inspect regenerated files for doc comments
4. Inspect reference.yaml for populated doc fields

### Part 28: Parameter documentation → slot_docs in reference.yaml ✅

Part 25 added method-level doc comments flowing through code generation to
`reference.yaml`. But `slot_docs` was empty for every `plan.*` entry — the
reference tables showed slot names with no descriptions.

**What was done:**

1. Added `Slots:` sections to all 35 provider methods across 9 providers
2. Added `docComment`/`docSummary` template functions in noblefactor-ops
3. Removed `ctx.TargetChecksum`/`execution.ChecksumBytes` from generator
   (consumer content model no longer references removed checksum fields)
4. Updated `validate.star` to detect `MakeAttr` calls (not just `NewBuiltin`)
5. Changed all predicate methods from `bool` to `(bool, error)` return
6. Changed `shell.Exec`/`shell.PowerShell` from `error` to `(string, error)`
7. Regenerated all 9 `plan_*_gen.go` + 9 `actions_gen.go` + `reference.yaml`

**Verification results:**
- `grep -c 'slot_docs: {}' reference.yaml` → 2 (only `choose` and `gather`, flow actions)
- `plan.file.link` has `slot_docs: {source: "...", path: "..."}`
- `actions_gen.go` struct comments remain single-line (via `docSummary`)
- `plan_file_gen.go` handler methods have multi-line doc with Slots:

## Cross-Repo Coordination

Remaining parts only (Parts 1-25 are completed):

| Repo | Parts | Notes |
|------|-------|-------|
| noblefactor-ops | 21 (`go.type_doc()`), 28 (`docComment`/`docSummary`) | Must rebuild `star` before Parts 22, 28 |
| devlore-cli | 26, 27, 28 | After noblefactor-ops changes |

## Design Decision: Compensation Classification

**Problem**: Plannable actions have three compensation behaviors:

1. **Compensable**: `CompensateFoo` exists and undoes Foo (e.g., `file.Write` →
   `CompensateWrite` removes the file).
2. **Non-compensable**: `CompensateFoo` exists and warns that Foo cannot be
   undone (e.g., `service.Start` → `CompensateStart` logs a warning).
3. **No compensation required**: No `CompensateFoo` at all because undo makes
   no sense (e.g., `encryption.Decrypt` — memory-to-memory, nothing to undo).

**Solution**: Interface splitting + sentinel error.

```go
type Action interface {
    Name() string
    Do(ctx *Context, slots map[string]any) (Result, UndoState, error)
}

type CompensableAction interface {
    Action
    Undo(ctx *Context, slots map[string]any, state UndoState) error
}

var NotCompensableError = errors.New("action is not compensable")
```

| Category | Implements | Undo behavior | Executor behavior |
|----------|-----------|--------------|-------------------|
| Compensable | `CompensableAction` | Real undo logic | Push to recovery stack, call Undo on rollback |
| Non-compensable | `CompensableAction` | Returns `NotCompensableError` | Push to stack, log warning on rollback, continue |
| No compensation | `Action` only | N/A | Do NOT push to recovery stack |

**Generator detection**: Has `CompensateFoo` → generate `Undo` method (implements
`CompensableAction`). No `CompensateFoo` → omit `Undo` (implements `Action` only).

**Status**: Implemented (Part 18). All tests pass.

## Design Decision: `//devlore:plannable`

**Pattern**: Comment directive on the Provider struct's doc comment, following
Go's `//tool:directive` convention.

**Semantics**: A provider with `//devlore:plannable` participates in planning.
Its actions can be deferred into an execution graph via plan receivers and
executed via action nodes. A provider without the directive is realtime-only —
its methods execute immediately when called.

**Template selection matrix**:

| Directive | Star (realtime tool) | Lore/Writ (planning tools) |
|-----------|---------------------|---------------------------|
| `//devlore:plannable` | realtime receiver | plan receiver + action nodes |
| (none) | realtime receiver | realtime receiver |

**Status**: Implemented (Parts 20-22). All 9 plannable providers have the directive.

## Design: Go Bindings to Providers

### Problem

Providers are the canonical implementation of every action. Four binding
surfaces exist:

| Surface | Mechanism | Status |
|---------|-----------|--------|
| Starlark plan bindings | `plan.file.link()` → graph node → executor → `actions_gen` → `Provider` | 9 providers |
| Starlark realtime bindings | `ui.note()` in script env → `Provider` method directly | UI (Part 24) |
| Go graph execution | writ deploy/upgrade → build graph → executor → `actions_gen` → `Provider` | Works |
| **Go direct calls** | `os.Rename()`, `os.Symlink()` — **bypass file provider** (Part 26) | **Partial** |

### What bypasses providers today

**UI output — RESOLVED (Part 23).**
`cli.Note/Warn/Error/Success/Failure` now delegate to a package-level
`ui.Provider` instance. All 136 call sites are unchanged but route through
the provider.

**Direct filesystem operations (bypass file provider):**

| Caller | Operations | Should use |
|--------|-----------|------------|
| `writ adopt` (`commands.go:1238`) | `os.MkdirAll`, `os.Rename`, `os.Symlink`, `os.Remove` | `file.Provider.Move` + `file.Provider.Link` |
| `writ migrate execute` (`execute.go:71`) | `os.Rename` | `file.Provider.Move` |
| `writ migrate_cmd.go` (layer setup) | `os.MkdirAll`, `os.Remove`, `os.Symlink`, `os.Rename` | `file.Provider.Link`/`Mkdir` |
| `writ/secrets/crypto.go` | `os.MkdirAll`, `os.WriteFile` | `file.Provider.Copy` |
| `lore/commands.go` (manifest write) | `os.WriteFile` | Out of scope (CLI artifact, not deployment) |
| `lore/onboard/onboard.go` | `os.WriteFile` | Out of scope (CLI artifact) |

### Design Principle

Every action that changes the filesystem or produces user-visible output must
go through a provider. Go code calls providers the same way Starlark does —
just without the graph indirection when immediate execution is needed.

**Providers are already callable from Go.** `file.Provider{}.Link(source, path)`
works today. The missing piece is that Go code doesn't use them — it calls
`os.Symlink` and `cli.Note` instead.

### Go Binding Strategy

Three categories of Go-side provider usage:

**Category 1: Graph-mediated (no change needed)**

Deploy, upgrade, decommission, and reconcile already build execution graphs
and run them through the executor. The executor calls `actions_gen.Do()` which
delegates to `Provider.*()`. This is correct.

**Category 2: Direct provider calls (adopt, migrate, secrets)**

Code that does immediate filesystem operations should call `file.Provider`
methods directly instead of `os.Rename`/`os.Symlink`. No graph needed — just
instantiate and call:

```go
fp := &file.Provider{}
state, err := fp.Move(nil, source, path)  // nil gitMv
state, err = fp.Link(source, path)
```

This gets compensation receipts for free (the returned `map[string]any`),
which adopt currently lacks.

**Category 3: Convenience wrappers (UI provider)**

`cli.Note("message")` is called 136 times. Replacing every call site with
`ui.Provider{}.Note(msg, os.Stderr)` would be hostile to readability.
The solution: make `cli.Note` a **convenience wrapper** that delegates to
`ui.Provider`.

The UI provider needs to absorb the CLI output concerns:

| Concern | Current owner | Should own |
|---------|--------------|------------|
| ANSI color codes | `cli/output.go` globals | `ui.Provider` configuration |
| Program name prefix | `cli/output.go` `programName` var | `ui.Provider` configuration |
| Silent mode suppression | `cli/output.go` `silent` var | `ui.Provider` configuration |
| Output destination | Hardcoded `os.Stderr` | `ui.Provider` `io.Writer` field |
| Message formatting | `[program] [symbol] message` | `ui.Provider` |

### UI Provider Redesign

`ui.Provider` carries configuration and provides rich formatted output:

```go
// Provider provides user-facing terminal messaging.
type Provider struct {
    Writer      io.Writer  // Output destination (default: os.Stderr)
    ProgramName string     // Prefix for messages (default: "devlore")
    Silent      bool       // Suppress all output when true
    Color       bool       // Enable ANSI color codes (default: true)
}

func (p *Provider) Note(msg string) {
    if p.Silent { return }
    fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorGray, symbolNote), msg)
}
```

**Status**: Implemented (Part 23).

### Convenience Wrappers

`cli/output.go` is a thin wrapper around a package-level `ui.Provider`
instance. The wrappers handle `fmt.Sprintf` formatting before delegating
to the provider's `msg string` methods:

```go
var output = &ui.Provider{
    Writer: os.Stderr,
    Color:  true,
}

func SetProgramName(name string) { output.ProgramName = name }
func SetSilent(s bool)           { output.Silent = s }

func Note(format string, args ...interface{})    { output.Note(fmt.Sprintf(format, args...)) }
func Warn(format string, args ...interface{})    { output.Warn(fmt.Sprintf(format, args...)) }
func Error(format string, args ...interface{})   { output.Error(fmt.Sprintf(format, args...)) }
func Success(format string, args ...interface{}) { output.Success(fmt.Sprintf(format, args...)) }
func Failure(format string, args ...interface{}) error {
    return output.Fail(fmt.Sprintf(format, args...))
}
```

All 136 call sites remain unchanged. The behavior is identical. The difference
is that `ui.Provider` is now the single implementation, and Starlark scripts
(Part 24) use the same `Provider` with `os.Stdout` as the destination.

**Status**: Implemented (Part 23).

## Follow-up: Delete `content.literal` — DONE

`content.literal` was a pure pass-through (`[]byte` in, `[]byte` out). It was
part of the old content pipeline deleted in Phase 2C. The entire `content/`
package, `plan_content_gen.go`, and `content.Register` references have been
removed. No follow-up PR needed.

## Known Limitations

None remaining.
