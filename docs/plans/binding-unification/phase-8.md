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

6. **Vocabulary cleanup**: Renamed "operation" to "action" across all non-test
   and test files in the execution, lore, and writ layers.

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
| 9a | Regenerate plan receivers (8 providers) | Done |
| 9b | Regenerate graph actions (9 providers + UI) | Done |
| 10 | Rename `ctx.Logger`→`ctx.Writer` in noblefactor-ops (type mapping + tests) | Done |
| 11 | Rename `ctx.Logger`→`ctx.Writer` in devlore-cli (action.go, executor.go, gather.go, all tests, hand-written net/content actions) | Done |
| 12 | Delete all hand-written realtime receivers (15 files) + `bindings.go` | Done |
| 13 | Create `internal/execution/provider/ui/provider.go` (Note, Warn, Error, Success, Fail) | Done |
| 14 | Generate `ui/actions_gen.go` + register UI in `register_gen.go` | Done |
| 15 | Delete `plan_ui_gen.go` (UI is not plannable) | Done |
| 16 | Remove `LogReceiver` usage from `builder.go` | Done |
| 17 | Rebuild `star` binary from worktree source (twice: after Provider rename, after ctx.Writer rename) | Done |
| 18 | Compensation classification: split `Action`/`CompensableAction`, add `NotCompensableError`, update executor, recovery, flow actions, graph_actions.go.template, regenerate all actions_gen.go | Done |
| 19 | Vocabulary cleanup: rename "operation"→"action" across all files (tests, doc comments, field names, parameter names) | Done |
| 20 | Add `//devlore:plannable` directive to 10 plannable providers (not UI) | Done |
| 21 | Add `go.type_doc()` to noblefactor-ops GoReceiver + `commentGroupRaw` helper for directive preservation + tests | Done |
| 22 | Update `generate.star`: auto-detect plannable directive, derive template selection (plan_receiver+graph_actions vs realtime_receiver) | Done |

## Remaining

### Part 23: Redesign UI provider + Go bindings

See "Design: Go Bindings to Providers" below. Steps:

1. Redesign `ui.Provider` — add configuration fields, rich formatting
2. Update `cli/output.go` — delegate to `ui.Provider` (convenience wrappers)
3. Update `actions_gen.go` for UI — construct from `ctx`

### Part 24: Wire UI as realtime receiver in lore/writ (Starlark)

The UI provider is not plannable. Lore scripts need `note()`, `warn()`,
`error()`, `success()`, `fail()` as immediate output during plan construction.

Generate a realtime receiver for UI via `star devlore ops generate
--source=.../ui --templates=realtime_receiver` and wire it into `builder.go`
globals alongside `plan`:

```go
uiReceiver := NewUIReceiver(&ui.Provider{
    Writer:      os.Stderr,
    ProgramName: "lore",
    Color:       true,
})
globals["note"] = MakeAttr("note", uiReceiver.note)
```

### Part 25: Doc comments in templates + knowledge extract

1. Add `{{.Doc}}` to `plan_receiver.go.template` and `realtime_receiver.go.template`
2. Add `--format` flag to knowledge extract
3. Regenerate all `_gen.go` files
4. Regenerate knowledge artifacts (reference.yaml, reference.md)

### Part 26: Refactor filesystem bypass callers

Lower priority. Replace direct `os.*` calls with `file.Provider.*` calls:

- `writ adopt` (`commands.go:1238`) — `os.Rename`/`os.Symlink` → `file.Provider.Move`/`Link`
- `writ migrate execute` (`execute.go:71`) — `os.Rename` → `file.Provider.Move`
- `writ migrate_cmd.go` (layer setup) — `os.MkdirAll`/`os.Symlink`/`os.Rename` → `file.Provider.*`
- `writ/secrets/crypto.go` — assess; may stay direct (crypto output, not deployment)

### Part 27: Verification

1. `go build ./...`
2. `go test ./...`
3. Inspect regenerated files for doc comments
4. Inspect reference.yaml for populated doc fields

## Cross-Repo Coordination

| Repo | Parts | Notes |
|------|-------|-------|
| noblefactor-ops | 21 (`go.type_doc()`) | Must rebuild `star` before Part 22 |
| devlore-cli | 20, 22, 23, 24, 25, 26, 27 | After noblefactor-ops changes |

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

**Status**: Designed. Implementation in Parts 20-22.

## Design: Go Bindings to Providers

### Problem

Providers are the canonical implementation of every action. Four binding
surfaces exist:

| Surface | Mechanism | Status |
|---------|-----------|--------|
| Starlark plan bindings | `plan.file.link()` → graph node → executor → `actions_gen` → `Provider` | 10 providers |
| Starlark realtime bindings | globals in script env → `Provider` method directly | 0 (Part 24 adds UI) |
| Go graph execution | writ deploy/upgrade → build graph → executor → `actions_gen` → `Provider` | Works |
| **Go direct calls** | `cli.Note()`, `os.Rename()`, `os.Symlink()` — **bypass providers** | **Broken** |

### What bypasses providers today

**UI output (136 calls across 9 files):**

`cli.Note/Warn/Error/Success/Failure` in `internal/cli/output.go` are
package-level functions with global mutable state (`silent`, `programName`,
ANSI color codes) that write to `os.Stderr`. The `ui.Provider` in
`internal/execution/provider/ui/provider.go` is a separate implementation
with different formatting that writes to an `io.Writer` parameter.

| Caller | Count |
|--------|-------|
| `writ/commands.go` | 59 |
| `model/config.go` | 26 |
| `lore/commands.go` | 24 |
| `writ/migrate_cmd.go` | 14 |
| `writ/migrate/execute.go` | 8 |
| `writ/migrate/session.go` | 2 |
| 3 others | 1 each |

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

The current `ui.Provider` is a stateless struct with plain formatting. It
needs to carry configuration:

```go
// Provider provides user-facing terminal messaging.
type Provider struct {
    Writer      io.Writer  // Output destination (default: os.Stderr)
    ProgramName string     // Prefix for messages (default: "devlore")
    Silent      bool       // Suppress all output when true
    Color       bool       // Enable ANSI color codes (default: true)
}
```

Methods produce the rich formatted output currently in `cli/output.go`:

```go
func (p *Provider) Note(format string, args ...any) {
    if p.Silent { return }
    msg := fmt.Sprintf(format, args...)
    fmt.Fprintf(p.Writer, "[%s] [%s+%s] %s\n", p.ProgramName, gray, reset, msg)
}
```

### Convenience Wrappers

`cli/output.go` becomes a thin wrapper around a package-level `ui.Provider`
instance:

```go
var output = &ui.Provider{
    Writer:      os.Stderr,
    ProgramName: "devlore",
    Color:       true,
}

func SetProgramName(name string) { output.ProgramName = name }
func SetSilent(s bool)           { output.Silent = s }

func Note(format string, args ...any)    { output.Note(format, args...) }
func Warn(format string, args ...any)    { output.Warn(format, args...) }
func Error(format string, args ...any)   { output.Error(format, args...) }
func Success(format string, args ...any) { output.Success(format, args...) }
func Failure(format string, args ...any) error { return output.Fail(format, args...) }
```

All 136 call sites remain unchanged. The behavior is identical. The difference
is that `ui.Provider` is now the single implementation, and Starlark scripts
(Part 24) use the same `Provider` with `ctx.Writer` as the destination.

### Action Generation Impact

The generated `actions_gen.go` for UI currently instantiates `&Provider{}`
(stateless). With configuration fields, it needs a configured instance.

The `actions_gen.go` `Do()` method reads Writer/DryRun from `ctx` and
constructs a transient Provider. This preserves the stateless registration
pattern. `Do()` already has `ctx.Writer`.

**Status**: Designed. Implementation in Parts 23-24.

## Follow-up: Delete `content.literal`

`content.literal` is a pure pass-through (`[]byte` in, `[]byte` out). It was
part of the old content pipeline deleted in Phase 2C. With plan receivers
setting slot values directly (`plan.copy(content="hello")`), there is no need
for a separate action to "produce" a literal value — the string is already in
the slot.

**Delete in a follow-up PR:**
- `internal/execution/provider/content/` (entire package)
- `internal/starlark/plan_content_gen.go`
- Remove `content.Register(reg)` from `register_gen.go`
- Remove content tests from `provider_test.go` and `execution_test.go`

## Known Limitations

**slot_docs remain empty.** Provider doc comments describe the method, not
individual parameters. Populating `slot_docs` would require either structured
parameter annotations in Provider source or a parser that extracts per-parameter
descriptions from prose. This is a future enhancement — the method-level doc
is the important win for LLM consumers.
