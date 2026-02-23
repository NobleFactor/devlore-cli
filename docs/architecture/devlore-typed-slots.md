# Typed Slots and Context Data

Slots are the mechanism by which actions receive their inputs. This document
describes the slot model, the Context.Data property bag, and the resolution chain.

See also: [devlore-execution-graph.md](devlore-execution-graph.md) — Graph
structure and lifecycle.

## Terminology

| Term | Origin | Meaning |
|---|---|---|
| **Receiver** | Starlark | An object with methods. `file`, `plan.file`, `git` are receivers. Methods are bound to the receiver — `file.copy()` calls the `copy` method with `file` as the receiver. |
| **Method** | Starlark/Python | A callable bound to a receiver. `file.copy()`, `plan.file.link()`, `git.clone()` are method calls. |
| **Namespace** | Ours | The organizational grouping that ties together a provider, its receivers, its methods, and its actions. Derived from the provider package name: `file`, `pkg`, `service`. Appears in action names (`file.link`), method paths (`plan.file.link()`), and error messages. |
| **Planned receiver** | Ours | A receiver offering planned access — its methods create graph nodes for later execution. `plan.file`, `plan.git`. Generated from the provider's method signatures. |
| **Execute receiver** | Ours | A receiver whose methods execute immediately. `file`, `archive`, `service`. Generated from the provider's method signatures. |
| **Provider** | Ours | The hand-written Go struct whose methods contain business logic (activities). Source of truth for the generator. `file.Provider`, `pkg.Provider`, `service.Provider`. Lives in `provider/<namespace>/provider.go`. |
| **Activity** | Saga | A paired unit of work on a Provider: a forward method and an optional backward (compensating) method. `file.Provider.Copy` + `file.Provider.CompensateCopy` is one Activity. Not a Go type — a design concept enforced by naming convention and the generator. |
| **Forward** | Saga | The forward method of an Activity. The method itself — `Copy`, `Move`, `Install`. No prefix. Returns `(...result, map[string]any, error)` where the `map[string]any` is compensation state. Non-compensable methods omit the state return. |
| **Backward** | Saga | The compensating method of an Activity. Named `Compensate<Forward>` — e.g., `CompensateCopy`, `CompensateMove`. Accepts `(state map[string]any)`, returns `error`. Undoes what Forward did, guided by the state Forward saved. |
| **State (compensation)** | Saga | The `map[string]any` returned by Forward and passed to Backward during unwind. The S in the (A, C, S) tuple. Opaque to the executor — only the Activity knows what it means. Serializes to JSON/YAML for receipts. |
| **Phase** | Ours | A scoped transaction boundary in a lifecycle pipeline. Groups nodes, owns retry policy, and references a compensating phase. The executor treats phases as checkpoints for the saga pattern. |
| **Recovery stack** | Ours | Runtime bookkeeping that tracks completed phases and their compensating actions. Entries are pushed as phases complete and popped in LIFO order during unwind. |
| **Slot** | Ours | A named, typed input on a graph node. Holds a value or a promise. |
| **Promise** | Ours | A slot value that references another node's output, resolved at execution time. |

## Data Flow

```
CLI flags → runtime environment → user config files → Context.Data
```

Context.Data is a `map[string]any` — the single global property bag for a graph
execution. Values flow in with CLI flags at highest priority, runtime environment
next, and user config files last.

## Slot Resolution

Slots hold typed values or promises. When an action reads a slot:

1. **Caller-provided** — planned receiver method filled it explicitly (value or promise)
2. **Context.Data fallback** — engine fills unfilled slots from Context.Data by key name

A planned receiver method can override any global default per-node. If it
doesn't, the engine provides the global value from Context.Data.

```
Planned receiver             Engine                   Action
     │                         │                       │
     │── FillSlot("path",v) ──▶│                       │
     │                         │── resolve promises ──▶│
     │                         │── fill unfilled      ▶│
     │                         │   from Context.Data       │
     │                         │                       │── slots["path"] → value
     │                         │                       │── slots["username"] → from Context.Data
```

## Slot Types

`SlotValue.Immediate` is `any`. The type of each slot is determined by the
provider's method signature:

```go
func (p *Provider) Link(source, path string) error
//                      string        string
//                      slot:"source" slot:"path"

func (p *Provider) Render(templateData map[string]any, source string, content []byte) ([]byte, error)
//                        map[string]any               string        []byte (framework)
//                        slot:"template_data"         slot:"source"

func (p *Provider) Decrypt(decryptor func(string, []byte) ([]byte, error), source string, content []byte) ([]byte, error)
//                          func(...)                                        string        []byte (framework)
//                          slot:"decryptor"                                 slot:"source"
```

Strings, bools, maps, functions — all just values in slots.

### Framework Content

Consumer and transformer actions receive content from the upstream pipeline
(e.g., read source → decrypt → render → copy). This content flows via
Result and promise slots — upstream actions return content as Result, and
downstream actions receive it through a `"content"` promise slot.

The generator infers the content model from the provider method's return
signature:

| Return Signature | Content Model | Content? |
|---|---|---|
| `error` | No content | No content parameter |
| `(string, error)` | Content consumer | Last `[]byte` param is content (string return is checksum) |
| `([]byte, error)` | Content transformer | Last `[]byte` param is content ([]byte return is transformed content) |

No annotation is needed. The method signature IS the specification. The
generator validates at discovery time:

- No-content methods return `error` only
- Consumer methods return `(string, error)` and have at least one `[]byte` param
- Transformer methods return `([]byte, error)` and have at least one `[]byte` param
- Any other return signature is rejected

Everything except the framework `[]byte` maps to a slot. The slot name is
the snake_case form of the Go parameter name.

## Action Interface

One interface. Every generated action implements it:

```go
type Result = any
type UndoState = any

type Action interface {
    Name() string
    Do(ctx *Context, slots map[string]any) (Result, UndoState, error)
    Undo(ctx *Context, slots map[string]any, state UndoState) error
}
```

The executor resolves all promise slots, then calls `action.Do(ctx, slots)`
for every node. No type-switch. Each generated action is self-contained —
it knows its own content model because the generator baked it in from the
provider method's return signature. The generated `Do()` method handles slot
type assertions, dry-run logging, and delegation to the provider internally.
Content flows via Result and promise slots — no `ContentFor`/`StoreContent`.

## Namespace

The namespace is the organizational grouping for a provider, its receivers,
its methods, and the actions they produce. It matches the provider's package
name:

| Provider | Package | Namespace |
|---|---|---|
| `file.Provider` | `provider/file` | `file` |
| `pkg.Provider` | `provider/pkg` | `pkg` |
| `service.Provider` | `provider/service` | `service` |

The namespace appears in:
- Action names: `file.link`, `pkg.install`, `service.start`
- Planned receiver methods: `plan.file.link()`, `plan.package.install()`
- Execute receiver methods: `file.copy()`, `archive.extract()`
- Error messages: `file.link: slot "source" requires string`
- Provider package paths: `provider/file/`, `provider/pkg/`, `provider/service/`

One provider per namespace. The namespace IS the provider's identity.

## Two Worlds

**Plan time (Starlark):** Planned receiver methods create nodes and fill slots.
Starlark values are converted to Go types via Starlark type mappings.
Promises (Output references to upstream nodes) are also slots — resolved
later by the engine.

**Graph runtime (pure Go):** The engine resolves promises, fills unfilled
slots from Context.Data, then executes actions. Everything is Go-typed. No
Starlark involvement.

## Starlark Type Mappings

At plan time, Starlark values must be converted to Go types for slot
filling. The type mappings are determined from the provider's method
signatures:

| Go Type | Starlark Type | Notes |
|---|---|---|
| `string` | `starlark.String` | |
| `bool` | `starlark.Bool` | |
| `int` | `starlark.Int` | |
| `[]string` | `*starlark.List` | Elements must be String |
| `map[string]any` | `*starlark.Dict` | |
| `os.FileMode` | `starlark.Int` | e.g., `0o644` |

Function-typed slots (e.g., `decryptor`) are not filled from Starlark.
They come exclusively from Context.Data at runtime.

## Serialization

Graphs serialize to YAML/JSON for receipts and dry-run output. Only
caller-provided slots (in `node.Slots`) are serialized. Function-valued
slots from Context.Data are runtime-only — the engine fills them at execution
time, so they never appear in the serialized graph.

This means the serialized graph is a complete record of the plan (what the
caller requested) but not of the runtime configuration (which Context.Data
values were used). Runtime configuration is implicit from the CLI flags,
environment, and config files that produced Context.Data.

## Context.Data Contents

Context.Data contains everything an action might need that isn't
node-specific:

```go
// Built from CLI flags → runtime env → user config
data := map[string]any{
    // Template variables (runtime environment)
    "username":    "david",
    "home":        "/Users/david",
    "os":          "darwin",
    "arch":        "arm64",
    "hostname":    "macbook",
    "config_home": "/Users/david/.config",
    "data_home":   "/Users/david/.local/share",
    "segments":    map[string]string{"team": "noblefactor"},

    // Functions (from user config)
    "env": func(key string) string { return os.Getenv(key) },

    // Capabilities (from tool setup)
    "decryptor":   secretsMgr.Decryptor(),
    "validators":  validatorRegistry,
    "git_mv":      gitMvFunc,

    // Settings (from CLI flags)
    "prune":          true,
    "prune_boundary": "/Users/david",
    "backup_suffix":  ".writ-backup",
}
```

Key naming convention: snake_case. This aligns with Starlark conventions
and the generator's parameter name → slot name mapping.

## Providers

Providers are the hand-written source of truth. The generator reads their
method signatures to produce everything else: action structs, planned receivers,
execute receivers, and Starlark type mappings. Each provider lives in its own
package under `internal/execution/provider/`.

```go
// provider/file/provider.go — hand-written, survives regeneration
type Provider struct{}

func (p *Provider) Link(source, path string) error {
    // idempotent symlink
}

func (p *Provider) Copy(path string, mode os.FileMode, content []byte) (string, error) {
    // write file, return checksum
}

func (p *Provider) Backup(path, backupSuffix string) (string, error) {
    // timestamped backup using backupSuffix
}

func (p *Provider) Unlink(path string, prune bool, pruneBoundary string) error {
    // remove symlink, optionally prune empty parents
}

func (p *Provider) Remove(path string, prune bool, pruneBoundary string) error {
    // delete file, optionally prune empty parents
}

func (p *Provider) Write(content, path string, mode os.FileMode) error {
    // write inline content
}

func (p *Provider) Move(gitMv func(src, dst string) error, source, path string) error {
    // git mv with os.Rename fallback
}
```

Every parameter except the framework `content []byte` maps to a slot. The
slot value comes from the resolution chain: caller-provided first, then
Context.Data fallback.

## Generated Code

The generator reads the provider's methods and produces action structs
in the same package (`provider/file/actions_gen.go`). Everything in
`actions_gen.go` files is generated — nuke-safe, never hand-edited.

### Action Structs

Each generated action implements the `Action` interface. The content
model (no content, consumer, transformer) is baked in by the generator:

```go
// provider/file/actions_gen.go — generated, nuke-safe
type Link struct{ Impl *Provider }

func (o *Link) Name() string { return "file.link" }

func (o *Link) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
    source := slots["source"].(string)
    path := slots["path"].(string)

    if ctx.DryRun {
        _, _ = fmt.Fprintf(ctx.Logger, "[dry-run] link %v %v\n", source, path)
        return nil, nil, nil
    }
    return nil, nil, o.Impl.Link(source, path)
}

func (o *Link) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
    return nil
}
```

A content-transformer action receives content via a promise slot from
an upstream node's Result:

```go
// provider/encryption/actions_gen.go — generated, nuke-safe
type Decrypt struct{ Impl *Provider }

func (o *Decrypt) Name() string { return "encryption.decrypt" }

func (o *Decrypt) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
    decryptor, _ := slots["decryptor"].(func(string, []byte) ([]byte, error))
    source, _ := slots["source"].(string)
    content, _ := slots["content"].([]byte)

    if ctx.DryRun {
        _, _ = fmt.Fprintf(ctx.Logger, "[dry-run] decrypt %v\n", source)
        return nil, nil, nil
    }
    result, err := o.Impl.Decrypt(decryptor, source, content)
    if err != nil {
        return nil, nil, err
    }
    return result, nil, nil
}

func (o *Decrypt) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
    return nil
}
```

No type-switch in the executor. Every action is self-contained. Content
flows via Result and promise slots — no `ContentFor`/`StoreContent`.

### Registration

```go
// provider/file/actions_gen.go
func Register(reg *execution.ActionRegistry) {
    p := &Provider{}
    reg.Register(&Link{Impl: p})
    reg.Register(&Copy{Impl: p})
    reg.Register(&Backup{Impl: p})
    reg.Register(&Unlink{Impl: p})
    reg.Register(&Remove{Impl: p})
    reg.Register(&Write{Impl: p})
    reg.Register(&Move{Impl: p})
    reg.Register(&Mkdir{Impl: p})
    reg.Register(&Source{Impl: p})
}
```

Each provider package has a `Register(reg)` function. The top-level
`provider.RegisterAll(reg)` calls all of them. The provider struct has
no fields — it is a method namespace. All inputs come through typed
slots. Registration is stateless.

## Engine Slot Filling

Before executing each node, the engine fills unfilled slots from Context.Data:

```go
func (e *GraphExecutor) fillSlots(node *Node) {
    for key, value := range e.data {
        if _, exists := node.Slots[key]; !exists {
            node.SetSlot(key, value)
        }
    }
}
```

This ensures every slot has a value before the action runs.
Caller-provided slots take precedence. Context.Data provides defaults.

## Architectural Concerns

### Promises vs Values

Slots hold either values (known at plan time) or promises (resolved at
runtime). This is a fundamental distinction:

- **Values**: Filled by a planned receiver method from Starlark arguments.
  The slot contains a concrete Go value. Available for dry-run
  serialization, graph inspection, and preflight validation.

- **Promises**: References to another node's output (e.g., "the checksum
  produced by the copy node"). Resolved by the engine at execution time
  when the upstream node completes. Not available for inspection until
  runtime.

- **Context.Data defaults**: Values that exist in the global property bag but
  aren't on the node until the engine fills them. They behave like values
  once filled, but their absence from the node's Slots map means they're
  invisible to graph serialization and preflight.

The engine must handle all three cases in the resolution chain. The
executor resolves promises and fills Context.Data defaults into a flat
`map[string]any` before calling `action.Do(ctx, slots)`. Planned receiver
methods and Starlark users work with values and promises. Context.Data
defaults are invisible to them — they're engine-level concerns.

### Discoverability

A slot's name, type, and Starlark type mapping must be discoverable by
extension authors, plan writers, and error messages.

- **Names**: Derived from Go parameter names (snake_case). The canonical
  list of slot names for an action is its provider method signature.
  Available via `go.methods()` in the generator, and listed by each
  receiver's methods at plan time.

- **Types**: Determined by the provider method signature. The generator
  knows the Go type of each slot and the corresponding Starlark type for
  planned receiver method arguments. The type mapping table (string↔String,
  bool↔Bool, etc.) must be documented and consistent.

- **Introspection**: Actions should be able to report their expected
  slots — names and types — for tooling, help text, and error messages.
  The generator can produce a `Slots() []SlotInfo` method on each
  action.

### Error Reporting

Errors use Starlark names, not Go names. Users write Starlark; they see
Starlark names. When a slot type is wrong or missing, the error message
uses the slot name (`template_data`), the Starlark type name (`dict`),
and the receiver's namespace (`plan.file.render`):

```
plan.file.render: slot "template_data" requires dict, got string
```

Not:

```
FileRenderOp: slot "templateData" must be map[string]any
```

This is a general concern with dynamic language bindings — Go types must
be translated to the user-facing Starlark vocabulary in all error paths.
The generator produces error messages using Starlark names. The type
mapping table drives both the conversion logic and the error vocabulary.
