# Memory Resources

This document describes the `mem:` resource scheme — in-memory data
with a serialization lifecycle. Memory resources exist in the process
heap during execution, serialize to portable formats for transfer and
checkpointing, and compile on demand at the execution site.

See also: [mem-resource plan](../plans/mem-resource.md) — implementation
plan with phases, files, and tests.

## 1. The Portability Problem

A graph planned on machine A should execute on machine B. For external
resources this works: `file://` paths, `git://` URLs, `pkg://` names,
and `svc://` service identifiers are portable strings that resolve
against the target machine's state.

But some graph inputs are not external — they're computed at plan time
and exist only in the process heap. A Starlark lambda passed to
`file.walk_tree` is a live function object. A rendered template payload
is a byte buffer. A JSON document assembled from config fragments is a
`map[string]any`. These values have no URI, no on-disk representation,
and no way to cross a process boundary.

| Resource Type | Identity | Portable? | Problem |
|---|---|---|---|
| `file://` | Filesystem path | Yes | Resolves via `os.Stat` |
| `git://` | Clone path + ref | Yes | Resolves via `git` |
| `pkg://` | Package name + type | Yes | Resolves via package manager |
| `svc://` | Service name | Yes | Resolves via service manager |
| In-memory data | None | No | Dies with the process |

The `mem:` scheme gives in-memory data a URI, a serialization format,
and a resolution lifecycle — making it portable.

## 2. Design

### 2.1 mem.Resource

A `mem.Resource` is a typed byte buffer with an opaque URI.

```go
type Resource struct {
    op.ResourceBase
    ContentType string // "callable", "json", "template", etc.
    Data        []byte // raw content
    Hash        string // SHA-256 of Data — change detection, not identity
}
```

- **Scheme**: `mem`
- **Opaque**: `ContentType` + type-specific segments (no `//`, no Host/Path)
- **URI**: `mem:callable/file.Reducer/myfn`, `mem:json/config`
- **Hash**: metadata field for change detection and integrity — NOT in URI

The URI is stable across content changes. When a resource with the same
URI appears with a different hash, the catalog creates a shadow. Two
nodes referencing the same callable by name share one catalog entry.
See [devlore-resource-identity.md](devlore-resource-identity.md) for
the full URI design.

### 2.2 Lifecycle

```
                    ┌──────────────┐
                    │  Plan Time   │
                    │              │
                    │  Starlark VM │
                    │  produces    │
                    │  in-memory   │
                    │  value       │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │   Extract    │
                    │              │
                    │  Capture     │
                    │  metadata,   │
                    │  serialize   │
                    │  to bytes    │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │ mem.Resource │
                    │              │
                    │  Data []byte │
                    │  URI  string │
                    └──────┬───────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
       ┌──────▼──────┐ ┌──▼──────┐ ┌───▼─────────┐
       │  Slot       │ │ Persist │ │  Transfer   │
       │  (graph     │ │ (disk   │ │  (network   │
       │  node)      │ │ chkpt)  │ │  to remote) │
       └──────┬──────┘ └──┬──────┘ └───┬─────────┘
              │            │            │
              └────────────┼────────────┘
                           │
                    ┌──────▼───────┐
                    │  Exec Time   │
                    │              │
                    │  Resolve /   │
                    │  compile     │
                    │  on demand   │
                    └──────────────┘
```

Unlike `file://` or `git://` resources that reference external state,
`mem:` resources carry their content inline. The resource IS the data,
not a handle to data stored elsewhere. This means:

- **Serialization is lossless** — the full content travels with the graph.
- **No external dependencies** — no filesystem path, no URL, no package
  name to look up on the target machine.
- **Resolution is local** — `Resolve()` operates on the embedded `Data`,
  not on external I/O.

### 2.3 Persistence

A `mem.Resource` can be persisted to disk through any mechanism:

- **Graph serialization**: When a graph is written to YAML/JSON, the
  `Data` field is included (base64-encoded for binary content).
- **Recovery checkpoint**: The recovery stack can write `mem.Resource`
  data to disk as part of a saga checkpoint.
- **Receipt**: A completed execution receipt includes `mem.Resource`
  data for audit and replay.

Persistence is a property of the byte buffer, not of the resource type.
Any `[]byte` can be written to disk. The `mem.Resource` adds identity
(URI), typing (ContentType), and catalog integration.

### 2.4 Relationship to Recovery

The saga recovery mechanism (RecoveryStack, CompensateX methods) is
designed for undoing side effects. A `mem.Resource` itself is pure data
— creating one has no side effects to undo.

What IS compensable is the work performed USING a memory resource:

| Layer | Compensable? | Example |
|---|---|---|
| Creating a `mem.Resource` | No | Extracting a callable — pure computation |
| Storing it in a slot | No | Immutable graph definition |
| Persisting it to disk | Trivially | Delete the checkpoint file |
| **Invoking its content** | **Yes** | WalkTree's Reducer pushes operations onto the RecoveryStack |

The recovery mechanism is relevant to `mem.Resource` in two ways:

1. **Checkpoint persistence**: The recovery stack can persist `mem.Resource`
   data to disk as part of its checkpoint state, ensuring the graph is
   self-contained for resumption after failure.
2. **Cleanup**: If temporary files were written for the checkpoint, the
   recovery stack cleans them up on successful completion.

The substantive compensation happens at the action level, not the
resource level. WalkTree's `CompensateWalkTree` unwinds the Reducer's
effects — the callable resource itself is just the code that was
executed, not the effects it produced.

## 3. Callable — First Application

The first concrete `mem.Resource` is a Starlark callable: a lambda or
`def` function extracted from the Starlark VM into a self-contained,
compilable, serializable resource.

### 3.1 The Callable Problem

Provider methods like `file.WalkTree` accept Go function parameters
(`Reducer`). In Starlark, the user passes a lambda or named function.
The current system cannot:

- Store a `starlark.Callable` in a slot (not serializable)
- Invoke a Starlark function from `Do()` (no thread available)
- Transfer a callable to another machine (it's a VM pointer)

### 3.2 Solution: Extract, Compile, Serialize

At plan time, extract the callable into a self-contained synthetic
source file. Compile it to bytecode. Store both representations.

#### Extraction

Given a `*starlark.Function`, the extraction step:

1. **Introspects parameters**: `NumParams()`, `Param(i)`,
   `ParamDefault(i)`, `HasVarargs()`, `HasKwargs()`.

2. **Captures closure bindings**: `NumFreeVars()`, `FreeVar(i)` returns
   `(Binding, Value)` — the name and frozen value of each captured
   variable. Frozen values are serialized as Starlark literals.

3. **Extracts function source**: `Position()` provides the filename,
   line, and column. The source file is read and the function text
   extracted. Lambdas are transformed to `def` form.

4. **Emits a synthetic file**:

   ```starlark
   # Extracted callable — from recipe.star:42
   # Closure bindings:
   ext = ".py"
   threshold = 100

   def _callable(initial, resource, path):
       if path.endswith(ext) and resource.Size > threshold:
           return initial + [resource]
       return initial
   ```

The synthetic file is **self-contained**. All closure bindings are
inlined as module-level constants. No imports, no external script
references.

#### Three-Tier Storage

| Tier | Content | Purpose |
|---|---|---|
| **Source** | Synthetic `.star` file text | Human-readable, recompilable, version-independent |
| **Compiled** | `Program.Write` bytecode | Fast load, no parse/compile cost, version-pinned |
| **Live** | `starlark.Callable` in process | Zero-cost invocation, populated by `Init()` |

Source is always present as the authoritative representation. Compiled
bytecode is an optimization — fast to load, but pinned to a specific
`starlark.CompilerVersion`. If the version mismatches at load time,
the source is recompiled transparently.

```go
type Callable struct {
    Resource // embeds mem.Resource — source text in Data

    Compiled        []byte   // Program.Write bytecode
    FuncName        string   // function name in synthetic file
    ParamNames      []string // for validation
    CompilerVersion uint32   // starlark.CompilerVersion at compile time
    OriginalPos     string   // "recipe.star:42" for diagnostics

    fn starlark.Callable     // live — populated by Init(), not serialized
}
```

#### Compilation

```go
// Compile: source text → bytecode (called at extraction time)
func (c *Callable) Compile() error

// Init: bytecode → live callable (called at execution time)
func (c *Callable) Init(thread *starlark.Thread) error

// Fn: returns the live callable (panics if Init not called)
func (c *Callable) Fn() starlark.Callable
```

`Init()` loads the compiled bytecode via `starlark.CompiledProgram`,
runs `prog.Init(thread, predeclared)` to execute the synthetic module,
and extracts the named function from the resulting globals. If the
compiler version mismatches, it falls back to recompiling from source.

#### Version Pinning

`starlark.CompilerVersion` is stored alongside the bytecode. At load
time:

- **Version matches**: `CompiledProgram(bytecode)` — instant load.
- **Version mismatches**: `SourceProgramOptions(source)` — recompile.
  The source is always present, so version drift is transparent.

Applications should incorporate `CompilerVersion` into cache keys when
storing compiled callables externally (disk cache, remote store).

### 3.3 Closure Binding Serialization

Free variables captured by a lambda are frozen after `ExecFile` returns.
The extraction step serializes each as a Starlark source literal:

| Starlark Type | Literal Form | Example |
|---|---|---|
| `String` | Quoted string | `ext = ".py"` |
| `Int` | Integer | `threshold = 100` |
| `Float` | Float | `ratio = 3.14` |
| `Bool` | `True` / `False` | `verbose = True` |
| `NoneType` | `None` | `default = None` |
| `List` | List literal | `items = [1, 2, 3]` |
| `Dict` | Dict literal | `config = {"a": 1}` |
| `Tuple` | Tuple literal | `pair = (1, 2)` |

Non-primitive types (Resources, custom structs) that lack a natural
Starlark literal form produce an extraction error. In practice, closure
bindings are almost always primitives and containers.

### 3.4 Signature Validation

At extraction time, the callable's parameters are validated against the
target action's expected arity using `*starlark.Function` introspection:

- `NumParams()` — total parameters
- `Param(i)` — parameter name and position
- `ParamDefault(i)` — default value (nil if required)
- `NumKwonlyParams()` — keyword-only count
- `HasVarargs()` / `HasKwargs()` — variadic acceptance

`starlark.Builtin` callables (Go functions exposed to Starlark) cannot
be introspected — they lack parameter metadata. Validation is deferred
to invocation time.

### 3.5 Adapter Pattern

Adapters convert a `mem.Callable` into a concrete Go function type.
Each callable-accepting action defines its own adapter:

```
mem.Callable
    │
    ├─ ReducerAdapter    → file.Reducer (WalkTree)
    ├─ PredicateAdapter  → func(any) (bool, error) (Choose, WaitUntil)
    ├─ FilterAdapter     → func(any) (bool, error) (Gather where clause)
    └─ TransformAdapter  → func(any) (any, error) (custom transforms)
```

The adapter handles:
- **Argument marshaling**: Go values → Starlark values for the call
- **Return unmarshaling**: Starlark result → Go value
- **Swallowed params**: Internal params (e.g., `stack` in Reducer) are
  provided by the Go caller, not the Starlark function

### 3.6 Unification

`mem.Callable` is the single type for all execution-time callables.
There is no separate `RuntimePredicate` type. The orchestration
primitives (Choose, WaitUntil, Gather) use `PredicateAdapter` over
the same `mem.Callable`:

| Use Case | Before | After |
|---|---|---|
| WalkTree Reducer | Not exposed to Starlark | `mem.Callable` + `ReducerAdapter` |
| Choose predicate | `RuntimePredicate` (separate type, not serializable) | `mem.Callable` + `PredicateAdapter` |
| WaitUntil condition | `RuntimePredicate` | `mem.Callable` + `PredicateAdapter` |
| Gather filter | Not designed | `mem.Callable` + `FilterAdapter` |

One type, one serialization format, one compilation path, N adapters.

## 4. Thread Management

Actions invoke callables at execution time. This requires a Starlark
thread — the execution context for `starlark.Call`.

```go
// pkg/op/context.go
type Context struct {
    // ... existing fields ...
    Thread *starlark.Thread
}
```

The executor creates a fresh `starlark.Thread` before running the graph
and sets it on `op.Context`. The thread's print handler writes to
`ctx.Writer`. Actions that invoke callables use `ctx.Thread`. Actions
that don't need it ignore the field.

Starlark's freeze semantics guarantee thread safety: after `ExecFile`,
all module-level values are frozen. The callable's closure bindings are
frozen. The executor's thread is fresh, but the callable's frozen data
is safe to read from any goroutine.

## 5. Serialization

### What Serializes

| Field | Format | Notes |
|---|---|---|
| `Data` (source text) | UTF-8 string | Always present, authoritative |
| `Compiled` (bytecode) | Base64-encoded bytes | Optional optimization |
| `ContentType` | String | `"callable"`, `"json"`, etc. |
| `FuncName` | String | Function name in synthetic file |
| `ParamNames` | String list | For validation without compilation |
| `CompilerVersion` | Integer | For version-match check on load |
| `OriginalPos` | String | Diagnostics only |

### What Doesn't Serialize

| Field | Reason |
|---|---|
| `fn` (live callable) | VM pointer — reconstructed by `Init()` |

The live callable is transient. It's created by `Init()` at execution
time and discarded when the execution completes. The source and bytecode
are the durable representations.

## 6. Supersession Table

| Existing Document | Section | Status | Notes |
|---|---|---|---|
| [devlore-resource-management.md](devlore-resource-management.md) | §3.3 Resource Types | **Extended** | `mem:` scheme now implemented; row updated from "Planned" to "Implemented" |
| [devlore-orchestration-primitives.md](devlore-orchestration-primitives.md) | Runtime Predicates | **Superseded** | `RuntimePredicate` type replaced by `mem.Callable` + `PredicateAdapter` |
| [devlore-orchestration-primitives.md](devlore-orchestration-primitives.md) | Serialization §"What Doesn't Serialize" | **Superseded** | Callables now serialize via source text + compiled bytecode |
| [devlore-typed-slots.md](devlore-typed-slots.md) | Slot Types | **Extended** | `mem.Callable` flows through existing immediate slot path as a Resource |

## 7. Related Documents

- [devlore-resource-management.md](devlore-resource-management.md) — Resource lifecycle, catalog, URI scheme table
- [devlore-orchestration-primitives.md](devlore-orchestration-primitives.md) — RuntimePredicate (superseded by callable)
- [devlore-typed-slots.md](devlore-typed-slots.md) — Slot model and resolution
- [devlore-phase-execution.md](devlore-phase-execution.md) — Saga pattern, recovery stack
- [mem-resource plan](../plans/mem-resource.md) — Implementation plan
