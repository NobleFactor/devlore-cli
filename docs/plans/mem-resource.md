---
title: "Memory Resources and Callables"
issue: TBD
status: in-progress
created: 2026-03-07
updated: 2026-03-08
---

# Plan: Memory Resources and Callables

## Summary

Implement the `mem:` resource scheme and its first application:
serializable Starlark callables.

`mem.Resource` is a typed byte buffer with a semantically-named opaque
URI — the first resource type backed by in-memory data rather than an
external system. It serializes fully, can be persisted to disk or
transferred to another machine, and compiles on demand at the execution
site.

`mem.Callable` is a `mem.Resource` that holds a Starlark function
extracted into a self-contained synthetic source file, compiled
to bytecode. It can be persisted to disk,
transferred to another machine, and compiled on demand at execution time.

The motivating use case is `file.WalkTree`, whose `Reducer` parameter is
a per-file callback. The same mechanism serves RuntimePredicate, filters,
validators, transformers — any action that needs user-supplied logic.

**Unification requirement**: RuntimePredicate and all other callable
patterns share one type, one serialization format, one compilation path,
and one execution path. Adapters convert the unified callable to specific
Go function types.

## Goals

1. **Unified callable type**: One resource type (`mem.Callable`) for all
   execution-time callables — reducers, predicates, filters, transforms.
   RuntimePredicate becomes an adapter over `mem.Callable`, not a separate
   type.
2. **Serializable**: Callables serialize as source text and compiled
   bytecode. A graph containing callables can be written to disk, shipped
   to another machine, and executed there.
3. **Self-contained extraction**: At plan time, extract the callable into
   a synthetic single-function file. Closure bindings are captured via
   `*starlark.Function.FreeVar(i)` and inlined as module-level constants.
   The synthetic file has no external dependencies.
4. **Three-tier storage**: Source text (human-readable, recompilable),
   compiled bytecode (fast load, version-pinned), and live callable
   (in-process, zero-cost invocation). All three tiers stored in the
   `mem.Resource`.
5. **Unblock WalkTree**: `file.walk_tree(root, fn)` works in immediate
   mode. `plan.file.walk_tree(root, fn)` works in planned mode. Resolves
   BUGS.md #170.

## URI Scheme Summary

Phase 0 corrects all resource URI schemes. See
[4.1-resource-identity.md](../architecture/4.1-resource-identity.md)
for the full design.

| Scheme | Form | Catalog Key | Shadow Key |
|---|---|---|---|
| `file` | Hierarchical | `file:///absolute/path` | N/A (external) |
| `pkg` | Opaque | `pkg:type/name` | Version via `@version` |
| `svc` | Opaque | `svc:name` | Fragment reserved for future instances |
| `appnet` | Opaque | `appnet:<escaped-url>` | N/A (external) |
| `git` | Opaque | `git:<encoded-repo>[?path=...]` | `#<commit-hash>` |
| `mem` | Opaque | `mem:callable/type/name` | Content hash as metadata field |

## Current State (updated after Phase 5)

| Component | Status | Notes |
|---|---|---|
| `mem:` scheme | Implemented | `SchemeMem = "mem"` in `pkg/op/resource.go`; `mem.Resource` type complete |
| `mem.Resource` | Done (Phase 1) | ContentType, Qualifier, Data, Hash, opaque URI, constructor registered |
| `mem.Callable` | Done (Phases 1–4) | Embeds `mem.Resource`. Extract, Compile, Init, Fn lifecycle complete |
| Extraction | Done (Phase 2) | Lambda + def extraction, closure capture, synthetic file generation |
| Compilation | Done (Phase 3) | `Compile()` → bytecode, `Init(thread)` with version fallback |
| `CallableResource` interface | Done (Phase 4) | In `pkg/op/callable.go`, with extractor registry |
| `op.Context.Thread` | Done (Phase 4) | Starlark thread on context, created by executor |
| Planned bridge callable | Done (Phase 4) | `buildPlannedBridge` detects func-typed params, extracts callable |
| WalkTree Go method | Working | Accepts `Reducer` callable, compensable |
| Generic callable coercion | Done (Phase 5) | `initCallableSlots` + `buildCallableFunc` in reflection layer |
| WalkTree Starlark binding | Partial | Action created; gen integration needs Phase 7 (code generator) |
| `+devlore:callable` annotation | Exists on `Reducer` | Used by codegen for `swallow` — bridge generation in Phase 7 |
| RuntimePredicate | Designed, not implemented | Will become `PredicateAdapter` over `mem.Callable` |
| E2E tests | Pending (Phase 6) | Starlark test scripts for immediate and planned WalkTree |

## Design

### Unified Callable Model

Every execution-time callable — regardless of purpose — follows the same
lifecycle:

```
Plan time:
  *starlark.Function
      │
      ▼
  Extract: introspect params, free vars, source position
      │
      ▼
  Synthesize: generate self-contained .star source file
      │
      ▼
  Compile: SourceProgramOptions → Program
      │
      ▼
  Store: mem.Callable{Source, Compiled, Metadata}
      │
      ▼
  Slot: stored as immediate Resource value in graph node

Serialization:
  mem.Callable → YAML/JSON/disk (source text + bytecode + metadata)

Execution time (any machine):
  Load: CompiledProgram(bytecode) → Program
      │ (or recompile from source if CompilerVersion mismatches)
      ▼
  Init: prog.Init(thread, predeclared) → globals["_callable"]
      │
      ▼
  Adapt: initCallableSlots + buildCallableFunc → Go func type
      │
      ▼
  Invoke: action.Do() calls the adapted function
```

### Two Kinds of Callables

The system already has **plan-time callables** — Starlark functions that
execute during graph construction to build nodes. Choose's `then` callback
is the canonical example. These don't need slot storage or serialization;
they run immediately in the Starlark thread and are done.

This plan adds **execution-time callables** — Starlark functions extracted
into a `mem.Callable` resource and invoked from a Go action's `Do()`.

| | Plan-time callable | Execution-time callable |
|---|---|---|
| When it runs | During Starlark script execution | During `Do()` in the executor |
| What it does | Builds graph structure | Computes a value, filters, transforms |
| Storage | Not stored — ephemeral | `mem.Callable` resource in slot |
| Serializable | N/A | Yes — source text + compiled bytecode |
| Thread | Starlark's own thread | Thread from `op.Context` |
| Example | `plan.choose(when=p, then=lambda: ...)` | `file.walk_tree(root, lambda i, r, p: ...)` |

### mem.Resource — Foundation

`mem.Resource` is the first resource type backed by in-memory data rather
than an external system. It serves callables now and template payloads,
JSON content, and other in-memory artifacts later.

```go
// pkg/op/provider/mem/resource.go

type Resource struct {
    op.ResourceBase
    ContentType string // "callable", "json", "template", etc.
    Data        []byte // raw content (source text, JSON, template)
    Hash        string // SHA-256 of Data — metadata, NOT part of URI
}

func (r Resource) String() string { return r.Format(r) }
```

URI uses the opaque form (no `//`): `mem:callable/file.Reducer/myfn`,
`mem:json/config`. The content hash is stored as a field for change
detection and integrity verification, not in the URI. See
[4.1-resource-identity.md §mem:callable URI Structure](../architecture/4.1-resource-identity.md#memcallable-uri-structure).

The recovery mechanism can persist `mem.Resource` to disk — it's just
bytes with a content type. This is the first use case for `mem:` with
a materialization path to durable storage.

### mem.Callable — The Unified Callable

```go
// pkg/op/provider/mem/callable.go

type Callable struct {
    Resource // embeds mem.Resource (source text in Data)

    // Compiled bytecode — Program.Write output. Nil until Compile.
    // Persisted alongside source for fast reload.
    Compiled []byte

    // URI identity fields — these compose the opaque URI:
    // mem:callable/<FuncType>/<Name>
    FuncType string // named Go type: "file.Reducer", "Predicate"
    Name     string // function name or <action>.<param> for lambdas

    // Metadata captured at extraction time.
    FuncName        string   // function name in synthetic file ("_callable" or original)
    ParamNames      []string // parameter names (excluding swallowed)
    NumParams       int      // total params (for validation)
    CompilerVersion uint32   // starlark.CompilerVersion at compile time
    OriginalPos     string   // "recipe.star:42" (diagnostics only)

    // Live state — populated by Init(), not serialized.
    fn starlark.Callable
}
```

`Callable` IS a `mem.Resource` (embeds it). The `Data` field holds the
synthetic source text. The `Compiled` field holds bytecode. Both are
`[]byte` — serializable, transferable, persistable.

URI: `mem:callable/file.Reducer/count_python_files` (named def),
`mem:callable/file.Reducer/file.walk_tree.fn` (lambda). The `FuncType`
and `Name` fields compose the opaque URI segments. The content hash on
the embedded `Resource` detects when a callable with the same URI has
different content (triggering a catalog shadow).

### Extraction — Synthetic File Generation

At plan time, when a `*starlark.Function` is passed to an action:

```go
// pkg/op/provider/mem/extract.go

func Extract(fn *starlark.Function) (*Callable, error)
```

1. **Introspect parameters**: `fn.NumParams()`, `fn.Param(i)`,
   `fn.ParamDefault(i)`, `fn.HasVarargs()`, `fn.HasKwargs()`.

2. **Capture closure bindings**: `fn.NumFreeVars()`, `fn.FreeVar(i)` →
   `(Binding, Value)`. Each free variable is a frozen Starlark value.
   Serialize each value as a Starlark literal for the synthetic file
   preamble.

3. **Extract function source**: `fn.Position()` → filename, line, col.
   Read the source file and extract the function text. For lambdas,
   transform `lambda args: expr` → `def _callable(args): return expr`.
   For `def` functions, preserve the body as-is.

4. **Emit synthetic file**:
   ```starlark
   # Extracted callable — from recipe.star:42
   # Closure bindings:
   ext = ".py"
   threshold = 100
   config = {"verbose": True, "mode": "strict"}

   def _callable(initial, resource, path):
       if path.endswith(ext) and resource.Size > threshold:
           return initial + [resource]
       return initial
   ```

5. **Compile**: `SourceProgramOptions(opts, "<callable>", source, isPredeclared)`
   → `*Program`.

6. **Serialize bytecode**: `prog.Write(&buf)` → `Compiled` field.

7. **Store**: Source text in `Data`, bytecode in `Compiled`, metadata
   in the struct fields.

The synthetic file is **self-contained** — all closure bindings are
inlined. No external imports, no script re-execution needed.

### Value Serialization for Closure Bindings

Frozen Starlark values must be serialized as valid Starlark literals
in the synthetic file preamble:

| Starlark Type | Serialization | Example |
|---|---|---|
| `String` | Quoted string | `ext = ".py"` |
| `Int` | Integer literal | `threshold = 100` |
| `Float` | Float literal | `ratio = 3.14` |
| `Bool` | `True` / `False` | `verbose = True` |
| `NoneType` | `None` | `default = None` |
| `List` | List literal | `items = [1, 2, 3]` |
| `Dict` | Dict literal | `config = {"a": 1}` |
| `Tuple` | Tuple literal | `pair = (1, 2)` |

Complex types (Resources, custom structs) that don't have a natural
Starlark literal representation are serialized via `value.String()` and
wrapped in a comment noting the original type. The extraction step
reports a warning for non-primitive bindings. In practice, closure
bindings are almost always primitives and containers.

### Compilation and Initialization

```go
// pkg/op/provider/mem/callable.go

// Compile compiles the source text and stores the bytecode.
// Called once at extraction time. Idempotent.
func (c *Callable) Compile() error {
    _, prog, err := starlark.SourceProgramOptions(
        &syntax.FileOptions{}, "<callable>", c.Data, isPredeclared,
    )
    if err != nil {
        return err
    }
    var buf bytes.Buffer
    if err := prog.Write(&buf); err != nil {
        return err
    }
    c.Compiled = buf.Bytes()
    c.CompilerVersion = starlark.CompilerVersion
    return nil
}

// Init loads the compiled program (or recompiles from source on version
// mismatch) and extracts the callable function. Must be called before
// Invoke.
func (c *Callable) Init(thread *starlark.Thread) error {
    var prog *starlark.Program
    var err error

    if c.Compiled != nil && c.CompilerVersion == starlark.CompilerVersion {
        prog, err = starlark.CompiledProgram(bytes.NewReader(c.Compiled))
    } else {
        _, prog, err = starlark.SourceProgramOptions(
            &syntax.FileOptions{}, "<callable>", c.Data, isPredeclared,
        )
    }
    if err != nil {
        return fmt.Errorf("callable init: %w", err)
    }

    globals, err := prog.Init(thread, predeclared)
    if err != nil {
        return fmt.Errorf("callable init: %w", err)
    }

    fn, ok := globals[c.FuncName]
    if !ok {
        return fmt.Errorf("callable init: function %q not found", c.FuncName)
    }
    callable, ok := fn.(starlark.Callable)
    if !ok {
        return fmt.Errorf("callable init: %q is %s, not callable", c.FuncName, fn.Type())
    }
    c.fn = callable
    return nil
}

// Fn returns the live callable. Panics if Init has not been called.
func (c *Callable) Fn() starlark.Callable {
    if c.fn == nil {
        panic("callable: Init not called")
    }
    return c.fn
}
```

### Thread on Context

```go
// pkg/op/context.go — add field:
Thread *starlark.Thread
```

The executor creates a fresh `starlark.Thread` before running the graph
and sets it on `op.Context`. The thread's print handler writes to
`ctx.Writer`.

### Generic Callable Coercion

The reflection layer handles callable→func conversion generically. No
per-action adapter code is needed. `initCallableSlots` runs in every
reflected action's `Do()` before `coerceArgs`:

```go
// pkg/op/callable.go

func initCallableSlots(ctx *Context, slots map[string]any, methodType reflect.Type, paramNames []string) error {
    for i, name := range paramNames {
        callable, ok := slots[name].(CallableResource)
        if !ok {
            continue
        }
        paramIdx := i + 1 // skip receiver
        paramType := methodType.In(paramIdx)
        if paramType.Kind() != reflect.Func {
            continue
        }
        if err := callable.Init(ctx.Thread); err != nil {
            return fmt.Errorf("param %s: init callable: %w", name, err)
        }
        adapted, err := buildCallableFunc(callable.Fn(), ctx.Thread, paramType)
        if err != nil {
            return fmt.Errorf("param %s: adapt callable: %w", name, err)
        }
        slots[name] = adapted
    }
    return nil
}
```

`buildCallableFunc` uses `reflect.MakeFunc` to create a Go function
matching the target type. It inspects the Starlark function's
`NumParams()` and truncates Go args to the callable's arity — this
handles `+devlore:callable swallow=stack` generically without per-action
knowledge. The adapter marshals Go→Starlark, calls the function, and
unmarshals Starlark→Go returns.

RuntimePredicate, ReducerAdapter, and any future callable-typed params
all work through this single mechanism — no adapter code per action.

### Signature Validation

At extraction time, validate the callable against the target action's
expected arity. Uses `*starlark.Function` introspection:

```go
// pkg/op/provider/mem/extract.go

func ValidateArity(fn *starlark.Function, minParams, maxParams int) error {
    numRequired := 0
    for i := range fn.NumParams() {
        if fn.ParamDefault(i) == nil {
            numRequired++
        }
    }
    if numRequired > maxParams {
        return fmt.Errorf("%s requires %d args but target accepts at most %d",
            fn.Name(), numRequired, maxParams)
    }
    if fn.NumParams() < minParams {
        return fmt.Errorf("%s accepts %d args but target requires at least %d",
            fn.Name(), fn.NumParams(), minParams)
    }
    return nil
}
```

For `starlark.Builtin` callables (Go functions exposed to Starlark),
introspection is not possible — builtins don't carry parameter metadata.
Validation is skipped; arity errors surface at call time.

### Slot Flow

`mem.Callable` is a Resource. Resources already flow through the slot
system as immediate values. No new slot variant needed — callables use
the existing `SetSlotImmediate` path. `FillSlot` already handles
Resources via the constructor registry.

At execution time, `ResolvedSlots` returns the `mem.Callable` as an
immediate value. The action's `Do()` method calls `Init(ctx.Thread)`
to populate the live callable, then adapts it.

### Immediate Mode Flow

```
Starlark: file.walk_tree(root, lambda i, r, p: ..., True)
    │
    ▼
buildMethodBridge: recognizes callable param type
    │
    ├─ Extract: *starlark.Function → mem.Callable (source + bytecode)
    ├─ Validate arity
    ├─ Init(thread) → live callable
    ├─ buildCallableFunc(fn, thread, targetType) → Go func (arity-truncated)
    │
    ▼
Provider.WalkTree(root, adaptedFn, true) → (result, stack, error)
    │
    ▼
classifyReturn → Starlark value
```

In immediate mode, extraction and compilation happen inline. The
compiled bytecode is discarded after use (no slot storage needed).

### Planned Mode Flow

```
Starlark: plan.file.walk_tree(root, lambda i, r, p: ..., True)
    │
    ▼
buildPlannedBridge: recognizes callable param type
    │
    ├─ Extract: *starlark.Function → mem.Callable (source + bytecode)
    ├─ Validate arity
    ├─ FillSlot stores mem.Callable as immediate Resource
    │
    ▼
Graph node with slots: {root: ..., fn: mem.Callable, honor_gitignore: true}
    │
    ▼  (serialization / transfer / checkpoint)
    │
    ▼  (execution time — same or different machine)
    │
executor.executeNode:
    ├─ ResolvedSlots returns mem.Callable in "fn" slot
    ├─ ctx.Thread set by executor
    │
    ▼
Action.Do(ctx, slots):
    ├─ initCallableSlots: Init(ctx.Thread) + buildCallableFunc → Go func
    ├─ coerceArgs: adapted func assigned to method param
    ├─ Provider.WalkTree(root, adaptedFn, honorGitignore)
    │
    ▼
Result + RecoveryStack
```

### Serialization

`mem.Callable` serializes fully:

```yaml
# In a serialized graph node's slot:
fn:
  uri: "mem:callable/file.Reducer/count_python_files"
  content_type: callable
  hash: "a1b2c3..."
  func_type: file.Reducer
  name: count_python_files
  source: |
    # Extracted callable — from recipe.star:42
    ext = ".py"
    def _callable(initial, resource, path):
        if path.endswith(ext):
            return initial + [resource]
        return initial
  compiled: <base64-encoded bytecode>
  compiler_version: 14
  func_name: _callable
  param_names: [initial, resource, path]
  original_pos: "recipe.star:42"
```

On load:
- If `compiler_version` matches → `CompiledProgram(compiled)` (fast)
- If version mismatch → `SourceProgramOptions(source)` (recompile)
- Source is always present as the authoritative fallback

### Recovery, Persistence, and Compensation

**Is saga recovery real for a callable?**

The callable itself is immutable code — source text and bytecode. It has
no side effects to undo. You can't "un-compile" or "un-extract" a
function. In the saga sense, a `mem.Callable` is not compensable. There's
nothing to roll back.

What IS compensable is **what the callable does when invoked**. WalkTree's
Reducer can push operations onto the RecoveryStack — those get unwound by
`CompensateWalkTree`. But that's the Reducer's effects being compensated,
not the callable resource itself.

The recovery framing has two distinct meanings here:

| Concern | Real? | Mechanism |
|---|---|---|
| Undo the callable's invocation effects | Yes | Existing RecoveryStack (WalkTree already does this) |
| Undo "creating" the callable | No | Immutable data — nothing to undo |
| Persist callable for resumption after crash | Yes | Checkpoint to disk — but this is **persistence**, not compensation |
| Clean up checkpoint files after success | Trivially yes | Recovery stack can remove temp files |

The real benefits of `mem.Resource` for callables are **serialization**
(anywhere to anywhere) and **self-containment** (graph carries its own
code). The recovery mechanism is a convenient vehicle for persisting
state to disk, but the callable itself doesn't exercise the compensation
path.

Where recovery IS real for `mem.Resource` in general: template payloads,
generated JSON content, intermediate computation results — in-memory data
that was produced by a compensable action and needs to be undone if a
downstream node fails. But for callables specifically, recovery is about
persistence and portability, not about undoing effects.

**Compensation for WalkTree**: The `Reducer` callback can push operations
onto the `RecoveryStack` during traversal. On error, `CompensateWalkTree`
unwinds the stack in LIFO order. The adapter passes the Go-side `stack`
to the Reducer; the Starlark function never sees it
(`+devlore:callable swallow=stack`). This is unchanged — callables don't
alter the existing compensation mechanism.

## Implementation Phases

|     | Phase | Name | Description | PR |
|-----|-------|------|-------------|----|
| [x] | 0 | [Resource identity](mem-resource/phase-0.md) | Slim `Resource` interface to `URI() + Resolve()`, correct URI schemes, rename net→appnet | #192–#197 |
| [x] | 1 | [mem.Resource + Callable](mem-resource/phase-1.md) | `mem.Resource` type, `mem.Callable` with source/bytecode storage | #197 |
| [x] | 2 | [Extraction](mem-resource/phase-2.md) | `Extract(*starlark.Function)`, closure capture, synthetic file generation | #198 |
| [x] | 3 | [Compilation](mem-resource/phase-3.md) | `Compile()`, `Init(thread)`, `CompiledProgram` round-trip, version fallback | #199 |
| [x] | 4 | [Thread + bridge](mem-resource/phase-4.md) | Thread on Context, immediate + planned bridge callable detection | #200 |
| [x] | 5 | [WalkTree action](mem-resource/phase-5.md) | Generic callable→func coercion in reflection layer | pending |
| [ ] | 6 | [E2E tests](mem-resource/phase-6.md) | Starlark test scripts for immediate and planned WalkTree | |
| [ ] | 7 | [Codegen](mem-resource/phase-7.md) | `star` recognizes callable params, generates adapter + bridge code | |

### Phase 0: Resource Identity

Simplify the `Resource` interface from 6 methods to 3. Correct URI
schemes to match their proper forms (opaque vs hierarchical). Rename
`net` → `appnet`. See [4.1-resource-identity.md](../architecture/4.1-resource-identity.md).

**Interface change** — `pkg/op/resource.go`:

- Remove `Scheme()`, `Host()`, `Path()` from `Resource` interface
- Keep them on `ResourceBase` as parsing helpers (not interface methods)
- Add `Opaque()` and `Fragment()` parsing helpers to `ResourceBase`
- Remove `NewURI(r Resource) string` method
- Rename `SchemeNet` → `SchemeAppNet`, value `"net"` → `"appnet"`
- `ResourceBase.URI()` returns the cached `uri` field (already does)

**Per-provider URI changes:**

- `pkg/op/provider/file/resource.go`:
  - Remove `Scheme()`, `Host()`, `Path()` methods
  - Cached `file://` + `SourcePath` construction (no change to URI format)

- `pkg/op/provider/pkg/resource.go`:
  - Remove `Scheme()`, `Host()`, `Path()` methods
  - URI becomes purl-compliant: `pkg:<type>/<name>[@<version>]`
  - `URI()` and `Purl()` converge — `Purl()` may become unnecessary
  - Remove `NewURI` dispatch

- `pkg/op/provider/service/resource.go`:
  - Remove `Scheme()`, `Host()`, `Path()` methods
  - URI becomes opaque: `svc:<name>` (was `svc:///<name>`)

- `pkg/op/provider/net/` → `pkg/op/provider/appnet/`:
  - Rename package `net` → `appnet`
  - Remove `Scheme()`, `Host()`, `Path()` methods
  - URI becomes opaque wrapper: `appnet:<escaped-inner-uri>`
  - Targeted escaping: `#` and `?` in inner URI are percent-encoded
  - Update all imports and references

- `pkg/op/provider/git/resource.go`:
  - Remove `Scheme()`, `Host()`, `Path()` methods
  - URI becomes opaque: `git:<encoded-repo-url>[?path=<path>]#<commit>`
  - Repo URL percent-encoded, optional path query, commit as fragment
  - `Resolve()` pins mutable refs (branch/tag) to commit hash in fragment
  - Cached construction (remove `NewURI` dispatch)

- Update all test files that call `Scheme()`, `Host()`, `Path()` on
  the interface — change to call on concrete type or use `ResourceBase`
  parsing helpers

- Update `DESIGN-DISCUSSION.md` references to `NewURI`

**Tests:**

- Each resource type: URI is correct after construction
- Each resource type: URI updates after `Resolve()` if path changes
- `pkg`: URI matches purl spec (`pkg:brew/jq`, `pkg:brew/jq@1.7`)
- `svc`: opaque URI (`svc:nginx`, not `svc:///nginx`)
- `appnet`: inner URI escaping round-trips correctly (`#` and `?` escaped)
- `git`: opaque URI with encoded repo URL, optional path query, commit fragment
- `git`: `Resolve()` pins mutable ref to commit hash in fragment
- `ResourceBase` parsing helpers: `Scheme()`, `Opaque()`, `Host()`,
  `Path()`, `Fragment()` work on cached URI strings
- Catalog operations still work with cached URIs
- `MarshalStarvalue` / round-trip tests still pass

### Phase 1: mem.Resource + Callable — DONE (PR #197)

- `pkg/op/provider/mem/resource.go` — `mem.Resource` type: `ContentType`,
  `Qualifier`, `Data`, `Hash`, opaque URI construction (`mem:<content-type>/...`),
  `String()` via `ResourceBase.Format`, `NewResource`, `NewResourceWithData`,
  constructor registration in `init()`
- `pkg/op/provider/mem/callable.go` — `mem.Callable` type: embeds
  `mem.Resource`, adds `FuncType`, `Name` (URI identity), `Compiled`,
  `FuncName`, `ParamNames`, `CompilerVersion`, `OriginalPos`,
  unexported `fn`. URI: `mem:callable/<FuncType>/<Name>`
- `pkg/op/provider/mem/callable_test.go` — construction, opaque URI
  generation, hash as metadata

### Phase 2: Extraction — DONE (PR #198)

- `pkg/op/provider/mem/extract.go` — `Extract`, `ExtractWithName`,
  `synthesize`, `extractLambdaBody`, `extractDefSource`, `extractSpan`,
  `ValidateArity`. Uses `syntax.Walk` for recursive AST search.
- `pkg/op/provider/mem/extract_test.go` — 13 tests: simple lambda,
  closure capture, named def, nested def with closure, custom naming,
  3 round-trips, 5 arity checks. `execScript` helper.
- `pkg/op/provider/mem/literals.go` — `FormatLiteral` serializes frozen
  Starlark values (None, Bool, Int, Float, String, List, Dict, Tuple)
  as source literals. Depth limit 20. Rejects Set.
- `pkg/op/provider/mem/literals_test.go` — 14 tests covering all literal
  types, escaping, nesting, unsupported type rejection.

### Phase 3: Compilation — DONE (PR #199)

- `pkg/op/provider/mem/callable.go` — `Compile()` and `Init(thread)` methods.
  `Compile` uses `SourceProgramOptions` + `Program.Write`. `Init` loads via
  `CompiledProgram` (fast path) or recompiles from source (version mismatch).
- `pkg/op/provider/mem/callable_test.go` — 12 tests added: Compile (4),
  Init (6), BytecodeRoundTrip, ExtractCompileInitRoundTrip.

### Phase 4: Thread + Bridge — DONE (PR #200)

- `pkg/op/context.go` — added `Thread *starlark.Thread` field
- `internal/execution/executor.go` — `newThread()` creates thread with
  print→writer; set on context in `runFlat`, `RunPhased`, `RunNodes`
- `pkg/op/callable.go` — **new file**: `CallableResource` interface
  (`Resource` + `Init` + `Fn` + `FuncTypeName`), `RegisterCallableExtractor`/
  `ExtractCallable` callback registry, `isCallableResource`, `isFuncType`
- `pkg/op/callable_test.go` — 6 tests: interface compliance, extractor
  registry, isCallableResource, isFuncType, validateSlotType for callable→func
- `pkg/op/planned_reflect.go` — `buildPlannedBridge` intercepts
  `*starlark.Function` for func-typed params, extracts to `CallableResource`,
  stores via `SetSlotImmediate`
- `pkg/op/action_reflect.go` — `validateSlotType` accepts `CallableResource`
  for func-typed targets
- `pkg/op/provider/mem/resource.go` — registered callable extractor in
  `init()` (Extract + Compile → CallableResource)
- `pkg/op/provider/mem/callable.go` — added `FuncTypeName()` method

### Phase 5: WalkTree Action — DONE (pending PR)

Generic callable→func coercion in the reflection layer. No per-action
custom code needed — the reflected action infrastructure handles
`CallableResource` slots automatically.

- `pkg/op/callable.go` — added `initCallableSlots` (pre-processes slots
  in `Do()` before `coerceArgs`), `buildCallableFunc` (creates Go func
  adapter via `reflect.MakeFunc` with arity truncation for swallowed
  params), `makeErrorReturn`, `unmarshalReturn`. Generic — works for any
  action with callable-typed parameters.
- `pkg/op/action_reflect.go` — wired `initCallableSlots` into all three
  `Do()` methods: `reflectedPureAction`, `reflectedFallibleAction`,
  `reflectedCompensableAction`. Runs before `coerceArgs` so standard
  coercion sees a directly-assignable func value.
- `pkg/op/callable_test.go` — 5 new tests: `BuildCallableFunc_SimpleReturn`,
  `BuildCallableFunc_ArityTruncation` (4-param Go func, 3-param Starlark fn),
  `BuildCallableFunc_StarlarkError`, `InitCallableSlots_ReplacesCallable`,
  `InitCallableSlots_SkipsNonCallable`.
- `pkg/op/provider/file/callable_test.go` — 2 integration tests through
  reflected actions: `TestWalkTreeAction_Integration` (full walk of temp
  dir with Starlark callable, path collection, Undo(nil), RecoveryStack),
  `TestWalkTreeAction_DryRun`.
- `pkg/op/starvalue_marshal.go` — added exported `MarshalValue` and
  `UnmarshalAny` wrappers for provider package access.

**Gen integration note**: The generated `params.gen.go` needs `"fn"` added
to `WalkTree` params. This is deferred to Phase 7 (code generator update).

### Phase 6: E2E Tests

- `test_walk_tree.star` — immediate mode: walk temp dir, collect paths
- `test_walk_tree_planned.star` — planned mode: `plan.file.walk_tree`
- `test_walk_tree_gitignore.star` — `.gitignore` filtering
- `test_walk_tree_skip.star` — `SkipDir` / `SkipAll` from callable
- `test_walk_tree_closure.star` — lambda with closure bindings
- `runner_test.go` — test functions

### Phase 7: Codegen

- Teach `star` to recognize `+devlore:callable` on function types
- Generate `fn` param in `params.gen.go` for callable-typed parameters
- Generate bridge code that wraps `starlark.Callable` → `Extract` → `CallableResource`
- Generate param registration with callable marker
- This phase is in the `star` tool (noblefactor-ops)

## Files to Create/Modify

**Phase 0 — Resource Identity:**

| File | Action | Purpose |
|---|---|---|
| `pkg/op/resource.go` | Modify | Slim interface, remove `NewURI`, add `Opaque`/`Fragment` helpers, rename `SchemeNet` → `SchemeAppNet` |
| `pkg/op/provider/file/resource.go` | Modify | Remove `Scheme`/`Host`/`Path`, cached URI construction |
| `pkg/op/provider/pkg/resource.go` | Modify | Purl-compliant opaque URI, converge `URI()` and `Purl()` |
| `pkg/op/provider/service/resource.go` | Modify | Opaque `svc:<name>` URI |
| `pkg/op/provider/net/` → `pkg/op/provider/appnet/` | Rename + Modify | Package rename, opaque `appnet:` URI with targeted escaping |
| `pkg/op/provider/git/resource.go` | Modify | Opaque `git:` URI with encoded repo URL, query, fragment |
| Test files | Modify | Update for new URI formats and removed interface methods |

**Phases 1–7 — mem.Resource and Callables:**

| File | Action | Purpose | Phase | Status |
|---|---|---|---|---|
| `pkg/op/provider/mem/resource.go` | Rewrite | `mem.Resource` type with ContentType, Data, Hash | 1 | Done |
| `pkg/op/provider/mem/callable.go` | Create | `mem.Callable` — unified callable resource | 1,3,4 | Done |
| `pkg/op/provider/mem/extract.go` | Create | Extraction, closure capture, synthetic file generation | 2 | Done |
| `pkg/op/provider/mem/literals.go` | Create | Starlark value → source literal serializer | 2 | Done |
| `pkg/op/provider/mem/callable_test.go` | Create | Callable tests | 1,3,4 | Done |
| `pkg/op/provider/mem/extract_test.go` | Create | Extraction tests | 2 | Done |
| `pkg/op/provider/mem/literals_test.go` | Create | Literal serialization tests | 2 | Done |
| `pkg/op/callable.go` | Create | `CallableResource` interface, extractor registry, generic callable→func coercion | 4,5 | Done |
| `pkg/op/callable_test.go` | Create | Interface, registry, type-check, coercion tests | 4,5 | Done |
| `pkg/op/context.go` | Modify | Add Thread field | 4 | Done |
| `pkg/op/planned_reflect.go` | Modify | Callable detection in planned bridge | 4 | Done |
| `pkg/op/action_reflect.go` | Modify | validateSlotType accepts Callable; initCallableSlots in Do() | 4,5 | Done |
| `pkg/op/starvalue_marshal.go` | — | No changes needed (generic code uses unexported funcs directly) | — | — |
| `pkg/op/provider/file/callable_test.go` | Create | Integration tests for WalkTree through reflected actions | 5 | Done |
| `internal/execution/executor.go` | Modify | Create thread, set on context | 4 | Done |
| `internal/e2e/testrunner/data/test_walk_tree*.star` | Create | E2E test scripts | 6 | Pending |
| `internal/e2e/testrunner/runner_test.go` | Modify | Test functions | 6 | Pending |

## Relationship to Other Plans

**RuntimePredicate** (orchestration-primitives Step 3) becomes an adapter
over `mem.Callable` — `PredicateAdapter`. The orchestration-primitives
plan should build on the callable infrastructure from this plan. The
`RuntimePredicate` type is eliminated; its functionality is:

```go
predicate := PredicateAdapter(callable, ctx.Thread)
result, err := predicate(input)
```

**mem.Resource** is introduced here as the first `mem:` resource. The
architecture doc's other planned uses (template payloads, JSON content)
build on the same foundation — they're `mem.Resource` instances with
different `ContentType` values.

## Related Documents

- [Architecture: Resource Identity](../architecture/4.1-resource-identity.md) — URI schemes, opaque vs hierarchical, interface simplification
- [Architecture: Memory Resources](../architecture/4.2-mem-resource.md) — `mem:` scheme, callable design
- [Architecture: Resource Management](../architecture/4-resource-management.md) — Resource lifecycle, `mem:` scheme table
- [Architecture: Orchestration Primitives](../architecture/2.3-orchestration-primitives.md) — RuntimePredicate (superseded by callable)
- [Orchestration Primitives](orchestration-primitives.md) — RuntimePredicate, WaitUntil
- [Resource Management](resource-management.md) — Resource lifecycle, catalog
- [Terminal Flow Control](terminal-flow-control.md) — Flow actions pattern
- [Codegen Extraction](codegen-extraction.md) — Star tool, `+devlore:` annotations
- [Provider Registration](provider-registration.md) — Action registration model
- BUGS.md #170 — WalkTree binding gap

## Open Questions

- [ ] Should the adapter support arity adaptation (calling a 2-param
  function where the Go type expects 3)? The `Actor` convenience wrapper
  suggests users often don't need `initial`. Introspecting
  `*starlark.Function.NumParams()` enables this.

  Yes. The adapter inspects arity and omits trailing params when the
  callable accepts fewer. The `+devlore:callable` annotation specifies
  the minimum required arity. The Actor pattern validates this idiom.

- [ ] For closure bindings of non-primitive types (e.g., a Resource
  captured in a lambda), what serialization format should the synthetic
  file use? `value.String()` works for display but may not produce a
  valid Starlark literal for all types.

  For now, restrict to primitive types + containers. Report a clear error
  at extraction time if a free variable holds a non-serializable type.
  Extend later if a real use case demands it.

- [ ] Should `mem.Callable.Init()` cache the live callable across
  multiple invocations (e.g., WalkTree calls the reducer per-file)?

  Yes — `Init()` is called once per execution. The live `fn` field is
  reused for all invocations within that execution. The adapter closes
  over `c.Fn()` which returns the cached callable.

- [ ] Should the compiled bytecode be stored in the resource catalog,
  or only in the slot value?

  Both. The catalog tracks the `mem:callable/<FuncType>/<Name>` URI
  for deduplication. If two nodes reference the same callable (same
  function type and name), they share one catalog entry. The content
  hash detects when the callable's content has changed.
