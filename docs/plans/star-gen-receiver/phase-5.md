# Phase 5: Implementation Structs and Nuke-Safe Generation

> **Superseded by [phase-6.md](phase-6.md).** Phase 6 incorporates the typed
> slots architecture, removes OpCategory, and unifies service ops. This
> document is preserved for historical reference.

## Context

Phases 0-4 built the generator pipeline: `go.methods()` discovers signatures,
`go.generate()` produces code from templates, `star gen.receiver` orchestrates,
and the templates support all three OpCategory variants (Direct/Writer/Transform).

Phase 5 restructures devlore-cli so that generated code can be nuked and
regenerated from scratch. The architecture separates implementation structs
(source of truth) from generated boilerplate (disposable).

**Repos**: noblefactor-ops (template updates), devlore-cli (restructure)

## Goals

1. **Implementation structs are the single source of truth.** Business logic
   lives in one place — implementation structs in `internal/execution/`.
2. **Generated code is nuke-safe.** Delete any `_gen.go` file, re-run the
   generator, get it back. No business logic lost.
3. **Prune is implicit.** Regeneration overwrites the entire file — removed
   methods disappear automatically. The compiler enforces consistency.
4. **Clear segregation.** Generated files use `_gen.go` suffix. Hand-written
   files use descriptive names. No ambiguity about what's generated.
5. **Generated code follows the Receiver pattern.** All generated receivers
   use the same base type, helpers, and conventions as hand-written receivers.

## Current State

| Component | Source of Truth | Generated | Hand-Written |
|---|---|---|---|
| Plan receivers (GitPlan, ArchivePlan) | None | No | Yes (plan_git.go, plan_archive.go) |
| Plan receivers (FilePlan, PackagePlan) | None | No | Yes (multi-node / variadic patterns) |
| Graph ops (file) | Inline in ops.go | No | Yes (10 ops) |
| Graph ops (package) | Inline in ops_package.go | No | Yes (6 ops) |
| Graph ops (service) | Inline in ops_service.go | No | Yes (15 ops) |
| Real-time receivers | Inline in receiver_*.go | No | Yes (10 receivers) |
| System bindings | Inline in system_*.go | No | Yes (always manual) |

## Target Architecture

```
internal/execution/
  # Implementation structs — SOURCE OF TRUTH
  impl_file.go           fileOps: Link, Copy, Render, Decrypt, Backup, ...
  impl_package.go        packageOps: Install, Upgrade, Remove, Update
  impl_service.go        serviceOps: Start, Stop, Restart, Enable, Disable

  # Generated graph ops — NUKE-SAFE
  ops_file_gen.go        FileLinkOp, FileCopyOp, ... → delegate to fileOps
  ops_package_gen.go     PackageInstallOp, ... → delegate to packageOps
  ops_service_gen.go     ServiceStartOp, ... → delegate to serviceOps

  # Framework (not generated)
  operation.go           Interfaces: Operation, Transform, Writer, Direct
  executor.go            GraphExecutor
  registry.go            OperationRegistry

internal/starlark/
  # Base receiver infrastructure — SHARED BY ALL
  receiver.go            Receiver, NewReceiver, MakeAttr, BuiltinFunc, NoSuchAttrError

  # Generated plan receivers — NUKE-SAFE
  plan_git_gen.go        GitPlan (clone, checkout, pull)
  plan_archive_gen.go    ArchivePlan (extract)

  # Hand-written plan receivers — EDGE CASES
  plan_file.go           FilePlan (configure = multi-node)
  plan_package.go        PackagePlan (variadic args pattern)
  plan_root.go           PlanRoot (source, literal, download, service, shell, gather)

  # Generated real-time receivers — NUKE-SAFE
  receiver_archive_gen.go
  receiver_service_gen.go

  # Hand-written receivers — ALWAYS MANUAL
  receiver_git.go        kwargs passthrough
  receiver_npm.go        kwargs passthrough
  receiver_docker.go     kwargs passthrough
  receiver_shell.go      host.RunCommand
  receiver_env.go        os.Getenv
  receiver_http.go       net/http
  receiver_log.go        io.Writer
  receiver_package.go    feature flags, settings

  # System bindings — ALWAYS MANUAL
  system_*.go            Read-only queries (not graph-building)
```

## Design Decisions

### Use the Receiver base pattern from noblefactor-ops

Both noblefactor-ops and devlore-cli already share an identical receiver pattern:

```go
// receiver.go — the shared base type
type Receiver struct{ name string }
func NewReceiver(name string) Receiver
func (r Receiver) String() string        // returns r.name
func (r Receiver) Type() string          // returns r.name
func (r Receiver) Freeze()               // no-op
func (r Receiver) Truth() starlark.Bool  // always true
func (r Receiver) Hash() (uint32, error) // unhashable

// Helpers
type BuiltinFunc func(thread, fn, args, kwargs) (starlark.Value, error)
func MakeAttr(name string, fn BuiltinFunc) starlark.Value
func NoSuchAttrError(receiver, attr string) error
```

In noblefactor-ops, every hand-written receiver follows this pattern
(using `BaseReceiver` — to be renamed to `Receiver` when sharing is resolved):

```go
type JSONReceiver struct{ BaseReceiver }                    // embed base
func NewJSONReceiver() *JSONReceiver { ... }               // constructor
func (r *JSONReceiver) Attr(name string) { ... }           // switch + MakeAttr
func (r *JSONReceiver) AttrNames() []string { ... }        // sorted list
func (r *JSONReceiver) encode(...) { ... }                 // BuiltinFunc sig
```

In devlore-cli, the real-time receivers already follow this same pattern:

```go
type ArchiveReceiver struct{ Receiver; host host.Host; ... }  // embed base
func NewArchiveReceiver(...) *ArchiveReceiver { ... }
func (r *ArchiveReceiver) Attr(name string) { MakeAttr(...) }
```

But the **plan receivers do NOT**. GitPlan and ArchivePlan have hand-written
inline starlark.Value methods instead of embedding `Receiver`. The plan
receiver template similarly generates inline boilerplate.

**Decision**: Update all three templates (plan_receiver, realtime_receiver,
graph_ops) so that generated code uses the established pattern:
- Plan receivers embed `Receiver` via `NewReceiver(namespace)`
- Attr dispatch uses `MakeAttr(name, fn)` instead of `starlark.NewBuiltin()`
- Error handling uses `NoSuchAttrError(receiver, attr)` instead of
  `starlark.NoSuchAttrError(fmt.Sprintf(...))`

This ensures generated code is structurally identical to hand-written code.
When hand-written plan receivers (FilePlan, PackagePlan) are eventually
refactored, they too should embed `Receiver`.

**Naming**: The base type is `Receiver` with factory `NewReceiver()` (as in
devlore-cli). noblefactor-ops currently uses `BaseReceiver`/`NewBaseReceiver()`
— this inconsistency will be resolved when the common types sharing mechanism
between star and devlore-cli is established. **TODO**: work out the sharing
mechanism for common types.

### Delegation pattern for graph ops

Generated ops delegate to an implementation struct rather than containing
inline business logic. The op struct holds a pointer to the impl:

```go
// impl_file.go — hand-written, survives regeneration
type fileOps struct{}

func (f *fileOps) Link(ctx *Context, source, path string) error {
    return os.Symlink(source, path)
}

// ops_file_gen.go — generated, nuke-safe
type FileLinkOp struct{ impl *fileOps }

func (o *FileLinkOp) Name() string         { return "file.link" }
func (o *FileLinkOp) Category() OpCategory { return OpDirect }

func (o *FileLinkOp) Execute(ctx *Context, node Executable) error {
    source := node.GetSlot("source")
    path := node.GetSlot("path")
    if ctx.DryRun {
        _, _ = fmt.Fprintf(ctx.Logger, "[dry-run] file.link %s %s\n", source, path)
        return nil
    }
    return o.impl.Link(ctx, source, path)
}

func FileOps() []Operation {
    impl := &fileOps{}
    return []Operation{
        &FileLinkOp{impl: impl},
        // ...
    }
}
```

The generator reads `fileOps` methods, produces the op struct with slot
readers + dry-run logging + delegation call. The impl struct is hand-written
and survives regeneration.

### Writer and Transform ops pass content through

For Writer ops, the generated `Write()` method passes `content` to the impl:

```go
func (o *FileCopyOp) Write(ctx *Context, node Executable, content []byte) (string, error) {
    path := node.GetSlot("path")
    if ctx.DryRun {
        return ChecksumBytes(content), nil
    }
    return o.impl.Copy(ctx, path, content)
}
```

For Transform ops, same pattern:

```go
func (o *FileRenderOp) Transform(ctx *Context, node Executable, content []byte) ([]byte, error) {
    source := node.GetSlot("source")
    return o.impl.Render(ctx, source, content)
}
```

The `content` parameter is framework-provided (from upstream content flow),
not a slot. The template handles this implicitly based on OpCategory — Writer
and Transform ops always receive `content` and pass it to the impl method.

### Implementation struct method signatures

The impl struct methods define the slot→parameter mapping:

```go
// Direct ops: (ctx, slot_params...) error
func (f *fileOps) Link(ctx *Context, source, path string) error

// Writer ops: (ctx, slot_params..., content) (checksum, error)
func (f *fileOps) Copy(ctx *Context, path string, content []byte) (string, error)

// Transform ops: (ctx, slot_params..., content) ([]byte, error)
func (f *fileOps) Render(ctx *Context, source string, content []byte) ([]byte, error)
```

Convention: `ctx *Context` is always first. For Writer/Transform, `content
[]byte` is always last. Everything in between maps to slots. The generator
skips `ctx` and `content` when producing slot readers (they're framework
params, not slots).

### Prune is implicit

When you regenerate `ops_file_gen.go`, the generator reads the current methods
on `fileOps` and produces ops for each one. If a method was removed from
`fileOps`, it simply doesn't appear in the regenerated file. The compiler
enforces consistency — any code that references the removed op will fail to
compile.

No special `--prune` flag is needed. The workflow is:
1. Add/remove methods on the impl struct
2. Re-run `star gen.receiver`
3. The compiler tells you what wiring needs updating

### FilePlan and PackagePlan stay hand-written

**FilePlan**: `configure()` creates two nodes (render→copy) with an internal
edge. This multi-node pattern can't be generated mechanically. Since FilePlan
has only 5 methods and one is multi-node, keeping it entirely hand-written is
simpler than splitting generated/manual methods with merged Attr().

**PackagePlan**: `install()`, `upgrade()`, `remove()` use variadic positional
args with `argsToStrings()` and join into comma-separated slot values. This
differs from the standard `UnpackArgs` pattern the generator produces.

Both are stable (methods rarely change) and small enough that hand-writing
is low cost.

### Service ops use dynamic dispatch

`plan.service(name, action)` creates a node with `Operation: "service-{action}"`,
and the service ops are platform-specific (launchd/systemd/Windows). The impl
struct has methods like `Start(ctx, name)` but the op names vary by platform.

The impl struct pattern still works: `serviceOps.Start(ctx, name)` contains
the platform dispatch logic (or uses the host.ServiceManager interface).

### Real-time receivers: typed vs kwargs passthrough

The generator handles **typed** receivers (named parameters with Starlark
types). These CAN be generated:
- ArchiveReceiver, ServiceReceiver

**kwargs passthrough** receivers convert arbitrary kwargs to CLI flags. The
generator doesn't handle this — it's a fundamentally different pattern.
These stay hand-written:
- GitReceiver, NpmReceiver, DockerReceiver

Other receivers with unique backing logic also stay hand-written:
- ShellReceiver (host.RunCommand), EnvReceiver (os.Getenv), HTTPReceiver
  (net/http), LogReceiver (io.Writer), PackageReceiver (feature flags)

## Implementation Steps

### Step 1: Update plan_receiver template to use Receiver pattern (noblefactor-ops)

The current `planReceiverTemplate` generates inline starlark.Value methods.
Update it to embed `Receiver` and use `MakeAttr`/`NoSuchAttrError`.

**1a: Replace inline starlark.Value boilerplate with Receiver embedding**

```go
// Before:
type {{.StructName}}Plan struct {
    graph   *execution.Graph
    host    host.Host
    project string
}

func (p *{{.StructName}}Plan) String() string        { return "{{.Namespace}}" }
func (p *{{.StructName}}Plan) Type() string          { return "{{.Namespace}}" }
func (p *{{.StructName}}Plan) Freeze()               {}
func (p *{{.StructName}}Plan) Truth() starlark.Bool  { return true }
func (p *{{.StructName}}Plan) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: {{.Namespace}}") }

// After:
type {{.StructName}}Plan struct {
    Receiver
    graph   *execution.Graph
    host    host.Host
    project string
}

func New{{.StructName}}Plan(graph *execution.Graph, h host.Host, project string) *{{.StructName}}Plan {
    return &{{.StructName}}Plan{
        Receiver: NewReceiver("{{.Namespace}}"),
        graph:    graph,
        host:     h,
        project:  project,
    }
}
```

**1b: Replace starlark.NewBuiltin with MakeAttr in Attr dispatch**

```go
// Before:
case "{{.SnakeName}}":
    return starlark.NewBuiltin("{{$.Namespace}}.{{.SnakeName}}", p.{{.SnakeName}}), nil

// After:
case "{{.SnakeName}}":
    return MakeAttr("{{$.Namespace}}.{{.SnakeName}}", p.{{.SnakeName}}), nil
```

**1c: Replace starlark.NoSuchAttrError with NoSuchAttrError in default case**

```go
// Before:
return nil, starlark.NoSuchAttrError(fmt.Sprintf("{{.Namespace}} has no attribute %q", name))

// After:
return nil, NoSuchAttrError("{{.Category}}", name)
```

**1d: Remove `"fmt"` from plan_receiver import if only used for NoSuchAttrError**

The `fmt` import is still needed for FillSlot error wrapping, so it stays. But
`starlark.NoSuchAttrError` is no longer called, so verify the import list.

**1e: Update tests**

Update `TestGeneratePlanReceiver` assertions:
- Check for `Receiver` embedding (contains `Receiver` in struct)
- Check for `NewReceiver("plan.file")` in constructor
- Check for `MakeAttr(` instead of `starlark.NewBuiltin(`
- Check for `NoSuchAttrError("file"` instead of `starlark.NoSuchAttrError`

### Step 2: Update realtime_receiver template to use Receiver pattern (noblefactor-ops)

Same changes as Step 1 but for the real-time receiver template:
- Embed `Receiver` instead of inline starlark.Value methods
- Use `MakeAttr` in Attr dispatch
- Use `NoSuchAttrError` in default case
- Update constructor to call `NewReceiver(name)`

### Step 3: Update graph_ops template — delegation pattern (noblefactor-ops)

**3a: Op struct with impl field**

```go
// Before:
type {{$.StructName}}{{.GoName}}Op struct{}

// After:
type {{$.StructName}}{{.GoName}}Op struct{ impl *{{$.ImplType}} }
```

Add `ImplType` field to `generateDescriptor`. Derived from the implementation
struct name (e.g., `fileOps`).

**3b: Delegation call in Execute/Write/Transform**

For Direct ops:
```go
func (o *{{$.StructName}}{{.GoName}}Op) Execute(ctx *Context, node Executable) error {
{{slotReaders .Params}}
    if ctx.DryRun {
        _, _ = fmt.Fprintf(ctx.Logger, "[dry-run] ...")
        return nil
    }
    return o.impl.{{.GoName}}(ctx{{implArgs .Params}})
}
```

For Writer ops:
```go
func (o *{{$.StructName}}{{.GoName}}Op) Write(ctx *Context, node Executable, content []byte) (string, error) {
{{slotReaders .Params}}
    if ctx.DryRun {
        return ChecksumBytes(content), nil
    }
    return o.impl.{{.GoName}}(ctx{{implArgs .Params}}, content)
}
```

For Transform ops:
```go
func (o *{{$.StructName}}{{.GoName}}Op) Transform(ctx *Context, node Executable, content []byte) ([]byte, error) {
{{slotReaders .Params}}
    if ctx.DryRun {
        return content, nil
    }
    return o.impl.{{.GoName}}(ctx{{implArgs .Params}}, content)
}
```

**3c: Registration function with impl**

```go
func {{.StructName}}Ops() []Operation {
    impl := &{{.ImplType}}{}
    return []Operation{
        &{{$.StructName}}{{.GoName}}Op{impl: impl},
    }
}
```

**3d: New template function `implArgs`**

Produces the comma-separated arg list for the delegation call. Reads param
GoName values and joins them: `, source, path`.

**3e: Update `gen-receiver.star` descriptor**

Add `impl_type` field to the descriptor, derived from the struct name. The
`.star` command passes this from the `--struct` flag (lowercased or as-is).

**3f: Tests**

Update `TestGenerateGraphOps` to verify:
- Op struct has `impl` field
- Execute/Write/Transform delegates to impl
- Registration function creates impl

### Step 4: Skip-param convention for ctx and content (noblefactor-ops)

The generator needs to distinguish slot parameters from framework parameters
(`ctx *Context`, `content []byte`).

**Convention-based approach**: The generator skips the first param if its type
is `*Context` and the last param if its type is `[]byte` AND the op_category
is Writer/Transform. The convention is clear and requires no extra config.
The generator already knows OpCategory, so it can infer which params are
framework-provided.

### Step 5: Extract file operation impls (devlore-cli)

**5a: Create `internal/execution/impl_file.go`**

Extract business logic from existing ops into `fileOps` struct:

```go
type fileOps struct{}

func (f *fileOps) Link(ctx *Context, source, path string) error { ... }
func (f *fileOps) Copy(ctx *Context, path string, content []byte) (string, error) { ... }
func (f *fileOps) Render(ctx *Context, source string, content []byte) ([]byte, error) { ... }
func (f *fileOps) Decrypt(ctx *Context, source string, content []byte) ([]byte, error) { ... }
func (f *fileOps) Backup(ctx *Context, path string) error { ... }
func (f *fileOps) Unlink(ctx *Context, path string) error { ... }
func (f *fileOps) Remove(ctx *Context, path string) error { ... }
func (f *fileOps) Write(ctx *Context, content, path string) error { ... }
func (f *fileOps) Validate(ctx *Context, check string) error { ... }
func (f *fileOps) Move(ctx *Context, source, path string) error { ... }
```

Each method contains the logic currently in the corresponding op's
Execute/Write/Transform body.

**5b: Generate `ops_file_gen.go`**

Run `star gen.receiver --struct fileOps --path ./internal/execution
--category file --templates graph_ops --write`

**5c: Delete `ops.go` inline ops**

Remove the 10 hand-written op structs. Keep `FileOps()` and `AllOps()`
registration functions (these are now in the generated file).

**5d: Verify**

All existing tests must pass. The executor still calls `op.Execute()`,
which now delegates to `fileOps`. Behavior is identical.

### Step 6: Extract package and service impls (devlore-cli)

**6a: `impl_package.go`**

```go
type packageOps struct{}

func (p *packageOps) Install(ctx *Context, packages, manager string, cask bool) error { ... }
func (p *packageOps) Upgrade(ctx *Context, packages, manager string, cask bool) error { ... }
func (p *packageOps) Remove(ctx *Context, packages, manager string, cask bool) error { ... }
func (p *packageOps) Update(ctx *Context, manager string) error { ... }
func (p *packageOps) Shell(ctx *Context, command string) error { ... }
func (p *packageOps) PowerShell(ctx *Context, command string) error { ... }
```

**6b: `impl_service.go`**

Since service ops are platform-specific but all follow the same pattern
(exec a command), the impl can use the `host.ServiceManager` interface:

```go
type serviceOps struct{}

func (s *serviceOps) Start(ctx *Context, name string) error { ... }
func (s *serviceOps) Stop(ctx *Context, name string) error { ... }
func (s *serviceOps) Restart(ctx *Context, name string) error { ... }
func (s *serviceOps) Enable(ctx *Context, name string) error { ... }
func (s *serviceOps) Disable(ctx *Context, name string) error { ... }
```

The platform dispatch (launchd vs systemd vs sc) is the impl's responsibility.
The generated ops just delegate.

**6c: Generate ops_package_gen.go and ops_service_gen.go**

**6d: Delete `ops_package.go` and `ops_service.go` inline ops**

Keep helper functions (`parsePackages`, `resolvePMForInstall`, etc.) in
`impl_package.go`.

### Step 7: Generate plan receivers (devlore-cli)

**7a: Generate `plan_git_gen.go`**

Run the generator against a descriptor with GitPlan's methods (clone,
checkout, pull). Delete `plan_git.go`.

**7b: Generate `plan_archive_gen.go`**

Run the generator against a descriptor with ArchivePlan's methods (extract).
Delete `plan_archive.go`.

**7c: FilePlan and PackagePlan stay hand-written**

No changes. These use patterns the generator doesn't support.

**7d: Update `plan_root.go`**

Update field types and constructor to use generated plan types if their
names changed (e.g., `GitPlan` stays the same).

### Step 8: Generate real-time receivers (devlore-cli)

**8a: Identify typed receivers**

Review each receiver. Generate those that use typed parameters:
- ArchiveReceiver → `receiver_archive_gen.go`
- ServiceReceiver → `receiver_service_gen.go`

**8b: kwargs passthrough receivers stay hand-written**

GitReceiver, NpmReceiver, DockerReceiver, ShellReceiver, EnvReceiver,
HTTPReceiver, LogReceiver, PackageReceiver — all stay manual.

**8c: Update `bindings.go`**

Update constructor calls if receiver constructors changed.

## Scope Boundaries

| Pattern | Generated | Hand-Written |
|---|---|---|
| Receiver embedding + NewReceiver() | Yes | - |
| MakeAttr dispatch + NoSuchAttrError | Yes | - |
| Single method → single op (delegation) | Yes | - |
| Slot readers + dry-run logging | Yes | - |
| Op struct + Name() + Category() | Yes | - |
| Registration function with impl | Yes | - |
| Plan receiver (standard UnpackArgs) | Yes | - |
| Attr/AttrNames dispatch | Yes | - |
| Implementation bodies | - | Required (impl structs) |
| Multi-node methods (configure) | No | Required |
| Variadic args pattern (PackagePlan) | No | Required |
| kwargs passthrough (CLI wrappers) | No | Required |
| System bindings | No | Required |
| PlanRoot top-level methods | No | Required |

## Verification

After each step:

```bash
# All tests pass
go test ./internal/execution/ -count=1
go test ./internal/starlark/ -count=1

# Build + vet
go build ./... && go vet ./...

# No regressions in executor behavior
go test ./internal/execution/ -run TestExecute -count=1
```

After all steps, validate the nuke-and-regenerate workflow:

```bash
# Delete all generated files
rm internal/execution/ops_*_gen.go
rm internal/starlark/plan_*_gen.go
rm internal/starlark/receiver_*_gen.go

# Regenerate
star gen.receiver --struct fileOps --path ./internal/execution --category file --templates graph_ops --output ./internal/execution --write
star gen.receiver --struct packageOps --path ./internal/execution --category package --templates graph_ops --output ./internal/execution --write
# ... (one per impl struct)

# Everything compiles
go build ./...
go test ./...
```
