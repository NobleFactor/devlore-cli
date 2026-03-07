# Phase 6: Typed Slots and Full Generation

## Context

Phases 0-4 built the generator pipeline in noblefactor-ops. Phase 5 planned the
devlore-cli restructuring (impl structs, nuke-safe generation). This phase
supersedes Phase 5 with architectural changes from the typed-slots design
(see [2.1-typed-slots.md](../../architecture/2.1-typed-slots.md)).

Key architectural decisions that change the approach:

| Phase 5 Assumption | Typed-Slots Architecture |
|---|---|
| `GetSlot` returns `string` | `GetSlot` returns `any` |
| `SlotValue.Immediate` is `string` | `SlotValue.Immediate` is `any` |
| Impl methods take `ctx *Context` first | Service methods are stateless — no ctx param |
| Ops access `ctx.Data` directly | Engine fills unfilled slots from Context.Data |
| `Category() OpCategory` on every op | Removed — no OpCategory |
| `Direct`/`Writer`/`Transform` interfaces | Removed — single `Operation` interface |
| `Executable` interface | Removed — ops receive `*Node` directly |
| `op_category` in descriptor | Return signature determines content model |
| `--category` flag required | Namespace derived from service name |
| Impl structs named `fileOps` | Services named `FileService` (hand-written) |
| Generated ops in same package | Generated code in same package (`_gen.go` suffix) |

**Repos**: noblefactor-ops (template changes), devlore-cli (infrastructure + restructure)

## Goals

1. **Typed slots.** `SlotValue.Immediate` is `any`. Slot types are determined
   by the service method's signature. No string-only slots.
2. **Slot resolution chain.** Caller-provided first, then engine fills unfilled
   slots from Context.Data. Operations never access ctx.Data directly.
3. **Single Operation interface.** `Direct`, `Writer`, `Transform`, `Executable`
   are deleted. One interface: `Operation` with `Name()` and `Execute()`. Each
   generated op is self-contained — the content model is baked in by the generator.
4. **Return signature is the spec.** The generator infers the content model
   (no content, consumer, transformer) from the service method's return type.
   No annotation needed.
5. **Services are the source of truth.** Hand-written `FileService`,
   `PackageService`, `ServiceManagerService`. The generator reads their method
   signatures to produce everything else.
6. **All infrastructure is generated.** Ops interface, graph operations, plan
   receivers, execute receivers, Starlark type mappings, registration — all
   generated from service signatures.
7. **Nuke-safe.** Delete any `*_gen.go` file, re-run the generator, get it back.

## Current State

| Component | Current | Target |
|---|---|---|
| `SlotValue.Immediate` | `string` | `any` |
| `GetSlot` return type | `string` | `any` |
| `Executable` interface | Present | Deleted — ops receive `*Node` |
| `FillSlot` | Converts all to string | Stores native Go types |
| `OpCategory` enum | Present, used by executor | Deleted |
| `Category()` on ops | Present on all 31 current ops | Deleted |
| `Direct`/`Writer`/`Transform` | Present, executor type-switches | Deleted — single `Operation` |
| Engine slot filling | Not implemented | Fills unfilled from Context.Data |
| Ops ctx.Data access | 8 ops read ctx.Data directly | Zero — all through slots |
| Services | Don't exist | `FileService`, `PackageService`, `ServiceManagerService` |
| Generated ops interface | Doesn't exist | `fileOps`, `packageOps`, `serviceManagerOps` |
| Generated graph ops | Don't exist | All ops generated as `_gen.go` files |
| Generated planned receivers | Don't exist | GitPlan, ArchivePlan generated |
| Generated execute receivers | Don't exist | Archive, ServiceManager generated |

## Step 1: Typed Slot Infrastructure (devlore-cli)

Changes to `internal/execution/graph.go`.

### 1a: SlotValue.Immediate → any

```go
type SlotValue struct {
    Immediate any    `json:"immediate,omitempty" yaml:"immediate,omitempty"`
    NodeRef   string `json:"node_ref,omitempty" yaml:"node_ref,omitempty"`
    Slot      string `json:"slot,omitempty" yaml:"slot,omitempty"`
}
```

`IsImmediate` changes: `s.Immediate != ""` → `s.Immediate != nil`.

### 1b: GetSlot → returns any

```go
func (n *Node) GetSlot(name string) any {
    if n.Slots != nil {
        if sv, ok := n.Slots[name]; ok {
            if sv.IsImmediate() {
                return sv.Immediate
            }
        }
    }
    return nil
}
```

### 1c: SetSlotImmediate → takes any

```go
func (n *Node) SetSlotImmediate(name string, value any) {
    if n.Slots == nil {
        n.Slots = make(map[string]SlotValue)
    }
    n.Slots[name] = SlotValue{Immediate: value}
}
```

### 1d: Delete Executable interface

Remove the `Executable` interface entirely. All code that references
`Executable` changes to `*Node`.

### 1e: Cascade fixes

All callers of `GetSlot` that expect `string` must type-assert:

- `executor.go`: `sortNodesByDepth`, `sortByDepth` use
  `node.GetSlot("path")` → `path, _ := node.GetSlot("path").(string)`
- `executor.go`: source file reading →
  `source, _ := node.GetSlot("source").(string)`
- `output.go`: `Output.Attr` returns slot value to Starlark →
  type-switch to convert `any` back to `starlark.Value`
- `output.go`: `Output.Path()` → `o.node.GetSlot("path").(string)`
- All 31 existing ops that call `node.GetSlot("x")` and use the result as
  string → add `.(string)` assertions. (These ops are replaced in Step 5,
  so the temporary assertions are short-lived.)

### Verification

```bash
go build ./internal/execution/...
go test ./internal/execution/ -count=1
go test ./internal/starlark/ -count=1
```

## Step 2: FillSlot Typed Conversions (devlore-cli)

Changes to `internal/starlark/output.go`.

Currently `FillSlot` converts everything to strings via `starlark.AsString`,
`fmt.Sprintf`. Change to store native Go types.

| Starlark Type | Go Storage Type | Current | New |
|---|---|---|---|
| `starlark.String` | `string` | `AsString` → string | Same |
| `starlark.Int` | `int` | `Sprintf("%d")` → string | `IntValue()` → int |
| `starlark.Bool` | `bool` | `Sprintf("%t")` → string | `bool(v)` → bool |
| `starlark.Float` | `float64` | `Sprintf("%f")` → string | `float64(v)` → float64 |
| `*starlark.Dict` | `map[string]any` | Not supported | Recursive conversion |
| `*starlark.List` (of strings) | `[]string` | Index slots + `.len` | `[]string` slice |
| `os.FileMode` via `starlark.Int` | `os.FileMode` | N/A (planned receiver handles) | Direct storage |

The `*Output` (promise) and `*Gather` paths are unchanged — they create edges,
not immediate values.

### Verification

```bash
go test ./internal/starlark/ -count=1
```

## Step 3: Delete Dispatch Interfaces (devlore-cli)

### 3a: Delete from operation.go

Remove `Direct`, `Writer`, `Transform` interfaces. Remove `OpCategory` type
and constants. The file becomes:

```go
type Operation interface {
    Name() string
    Execute(ctx *Context, node *Node) error
}
```

### 3b: Update executor

Remove the type-switch dispatch (`case Transform`, `case Writer`, `case Direct`).
The executor calls `op.Execute(ctx, node)` uniformly for every node. Content
sourcing moves into the ops themselves (temporarily for hand-written ops,
permanently for generated ops).

### 3c: Update all 31 ops

Every op implements the single `Execute(ctx *Context, node *Node) error` method.
Ops that previously implemented `Transform` or `Writer` now handle their own
content sourcing inside `Execute()`.

### 3d: Add content helpers to Context

```go
func (ctx *Context) ContentFor(node *Node) []byte  // read from upstream or source file
func (ctx *Context) StoreContent(node *Node, content []byte)  // store for downstream
```

### Verification

```bash
go build ./internal/execution/...
go test ./internal/execution/ -count=1
```

## Step 4: Engine Slot Filling (devlore-cli)

Add slot filling from Context.Data to the executor. Before executing each node,
fill any slot that the caller didn't provide.

### 4a: Add fillSlotsFromData to GraphExecutor

```go
func (e *GraphExecutor) fillSlotsFromData(node *Node) {
    for key, value := range e.options.Data {
        if _, exists := node.Slots[key]; !exists {
            node.SetSlotImmediate(key, value)
        }
    }
}
```

### 4b: Call before dispatch

In the executor's node loop, call `e.fillSlotsFromData(node)` before
`op.Execute(ctx, node)`.

### 4c: Verify existing ops still work

The 8 ops that currently read `ctx.Data` directly (RenderOp reads all of
ctx.Data, DecryptOp reads `decryptor`, BackupOp reads `backup_suffix`,
ValidateOp reads `validators`, MoveOp reads `git_mv`, UnlinkOp/RemoveOp
read `prune_empty_dirs`/`prune_boundary`) will continue to work because:

1. Engine fills slots from Context.Data
2. Ops still read ctx.Data (temporarily)
3. Both paths produce the same values

This is a transitional step. Step 5 removes the ctx.Data reads from ops.

### Verification

```bash
go test ./internal/execution/ -count=1
```

## Step 5: Service Extraction (devlore-cli)

Extract business logic into hand-written services. Each method receives all
inputs as parameters — no ctx, no ctx.Data access. Services are named
`*Service` by convention.

### 5a: file_service.go

```go
type FileService struct{}

func (f *FileService) Link(source, path string) error
func (f *FileService) Copy(path string, mode os.FileMode, content []byte) (string, error)
func (f *FileService) Render(templateData map[string]any, source, path, project string, content []byte) ([]byte, error)
func (f *FileService) Decrypt(decryptor func(string, []byte) ([]byte, error), source string, content []byte) ([]byte, error)
func (f *FileService) Backup(path, backupSuffix string) error
func (f *FileService) Unlink(path string, prune bool, pruneBoundary string) error
func (f *FileService) Remove(path string, prune bool, pruneBoundary string) error
func (f *FileService) Write(content, path string, mode os.FileMode) error
func (f *FileService) Validate(validators map[string]func() error, check, message string) error
func (f *FileService) Move(gitMv func(src, dst string) error, source, path string) error
```

Every parameter except the framework `content []byte` (last param on
consumer/transformer methods) maps to a slot. Parameters like `decryptor`,
`validators`, `templateData`, `backupSuffix`, `prune`, `pruneBoundary`,
`gitMv` come from Context.Data via engine slot filling.

Helper functions (`pruneEmptyParents`, `isSubpath`) move to this file.

### 5b: package_service.go

```go
type PackageService struct{}

func (p *PackageService) Install(packages []string, manager string, cask bool) error
func (p *PackageService) Upgrade(packages []string, manager string, cask bool) error
func (p *PackageService) Remove(packages []string, manager string, cask bool) error
func (p *PackageService) Update(manager string) error
func (p *PackageService) Shell(command string) error
func (p *PackageService) PowerShell(command string) error
```

Helper functions (`resolvePMForInstall`, `resolvePMForUpgrade`,
`resolvePMForRemove`, brew cask helpers) stay in this file.

`packages` is `[]string` — stored natively in the typed slot. The
`parsePackages` helper (comma-separated string splitting) is removed.
`FillSlot` handles `*starlark.List` → `[]string` conversion at plan time.

### 5c: service_manager_service.go

Platform-agnostic service operations. Same pattern as package: the caller
says `service_manager.start("foo")` and the service handles platform dispatch
internally. 15 platform-specific ops collapse to 5.

```go
type ServiceManagerService struct{}

func (s *ServiceManagerService) Start(name string) error
func (s *ServiceManagerService) Stop(name string) error
func (s *ServiceManagerService) Restart(name string) error
func (s *ServiceManagerService) Enable(name string) error
func (s *ServiceManagerService) Disable(name string) error
```

Each method detects the platform at runtime (`runtime.GOOS`) and dispatches
to the appropriate service manager (launchd on darwin, systemd on linux,
sc on windows). The 15 current ops (`LaunchdStartOp`, `SystemdStartOp`,
`WinServiceStartOp`, etc.) are replaced by 5 ops.

### Verification

Services compile standalone. No tests yet — they're exercised through
the generated ops in Step 7.

```bash
go build ./internal/execution/...
```

## Step 6: Generator Template Updates (noblefactor-ops)

### 6a: Single Operation interface in templates

All generated ops implement `Operation` with `Name()` and `Execute()`.
No `Direct`, `Writer`, `Transform` in generated code. The content model
is baked into each op's `Execute()` method based on the service method's
return signature.

### 6b: Generate ops interface

The generator produces an unexported interface from the service's method
signatures:

```go
// ops_file_gen.go
type fileOps interface {
    Link(source, path string) error
    Copy(path string, mode os.FileMode, content []byte) (string, error)
    // ...
}
```

### 6c: Content model inference from return signature

```go
type methodInfo struct {
    GoName       string
    SnakeName    string
    Params       []paramInfo
    ReturnType   string
    ContentModel string // "none", "consumer", "transformer"
    Doc          string
}
```

| Return Signature | Content Model | Generated Execute() |
|---|---|---|
| `error` | `none` | Read slots, delegate, return error |
| `(string, error)` | `consumer` | Read content + slots, delegate, store checksum |
| `([]byte, error)` | `transformer` | Read content + slots, delegate, store output content |

### 6d: Namespace from service name

Strip `Service` suffix, snake_case:

| Service | Namespace |
|---|---|
| `FileService` | `file` |
| `PackageService` | `package` |
| `ServiceManagerService` | `service_manager` |

### 6e: Expand type mappings

Add to `typeMappings`:

| Go Type | Slot Reader Pattern | Starlark Error Name |
|---|---|---|
| `map[string]any` | `node.GetSlot("x").(map[string]any)` | `dict` |
| `os.FileMode` | `os.FileMode(node.GetSlot("x").(int))` | `int` |
| `func(string, []byte) ([]byte, error)` | `node.GetSlot("x").(func(...))` | `func` |
| `func(src, dst string) error` | `node.GetSlot("x").(func(...))` | `func` |
| `map[string]func() error` | `node.GetSlot("x").(map[string]func() error)` | `dict` |
| `[]string` | `node.GetSlot("x").([]string)` | `list` |

Function-typed slots are never filled from Starlark. They come exclusively
from Context.Data at runtime. The generator skips them in planned receiver templates
(no UnpackArgs entry) but includes them in graph ops templates (slot assertion).

### 6f: Skip framework params in slot generation

The generator must distinguish slot params from framework params. Convention:

- **Framework content**: The last `[]byte` param on consumer/transformer methods.
  Identified by: method returns `(string, error)` or `([]byte, error)`, AND
  the last param's type is `[]byte`. This param is NOT a slot — it's the
  content pipeline.
- **All other params**: Slots.

No annotation needed. The method signature IS the specification.

### 6g: Generate as `_gen.go` files in the same package

Following Go convention, generated code lives in the same package as the
source it serves, with a `_gen.go` suffix. Each file carries the standard
`// Code generated from gen-receiver templates; DO NOT EDIT.` header.

```
internal/execution/
├── ops_file_gen.go              # fileOps interface, FileLinkOp, ..., FileOps()
├── ops_encryption_gen.go        # EncryptionDecryptOp, EncryptionOps()
├── ops_package_gen.go           # packageOps interface, PackageInstallOp, ..., PackageOps()
├── ops_shell_gen.go             # ShellOp, PowerShellOp, ShellOps()
└── ops_service_manager_gen.go   # ServiceManagerStartOp, ..., ServiceManagerOps()
```

Registration function accepts the unexported interface:

```go
func FileOps(impl fileOps) []Operation { ... }
```

`AllOps()` in `ops_registry.go` wires it: `FileOps(&FileService{})`.

### 6h: Tests

Update existing tests, add new tests:

- `TestGenerateOpsInterface`: Verify generated interface matches service methods
- `TestGenerateGraphOpsTypedSlots`: Verify typed assertion code for string,
  bool, func params
- `TestGenerateConsumerInferred`: Method with `(string, error)` return generates
  content consumer Execute()
- `TestGenerateTransformerInferred`: Method with `([]byte, error)` return generates
  content transformer Execute()
- `TestGenerateNoContentInferred`: Method with `error` return generates
  no-content Execute()
- `TestGenerateNoDispatchInterfaces`: Verify no `Direct`, `Writer`, `Transform`
  in output
- `TestNamespaceFromService`: Verify suffix stripping and snake_case

### Verification

```bash
cd /path/to/noblefactor-ops
go test ./internal/starlark/ -run TestGenerate -count=1
go test ./internal/starlark/ -count=1
go build ./... && go vet ./internal/starlark/
```

## Step 7: Generate and Replace Graph Ops (devlore-cli)

### 7a: Generate ops_file_gen.go

Run the generator against `FileService`. The generated file contains:

- `fileOps` interface (unexported)
- `FileLinkOp`, `FileCopyOp`, `FileRenderOp`, `FileBackupOp`,
  `FileUnlinkOp`, `FileRemoveOp`, `FileWriteOp`, `FileMoveOp`
- Each op: struct with `impl fileOps`, `Name()`, `Execute()` with typed slot
  assertions + dry-run + content handling + delegation
- `FileOps(impl fileOps) []Operation` registration function

### 7b: Generate ops_encryption_gen.go

From `EncryptionService`. 1 op (Decrypt).

### 7c: Generate ops_package_gen.go

From `PackageService`. 4 ops (Install, Upgrade, Remove, Update).

### 7d: Generate ops_shell_gen.go

From `ShellService`. 2 ops (Shell, PowerShell).

### 7e: Generate ops_service_manager_gen.go

From `ServiceManagerService`. 5 platform-agnostic ops (Start, Stop, Restart,
Enable, Disable).

### 7f: Delete hand-written ops and wire AllOps

Remove `ops.go` (file ops + AllOps), `ops_package.go`, `ops_service.go`.

Create `ops_registry.go` with `ValidateOp` (hand-written) and `AllOps()`:

```go
// ops_registry.go
func AllOps() []Operation {
    var ops []Operation
    ops = append(ops, FileOps(&FileService{})...)
    ops = append(ops, EncryptionOps(&EncryptionService{})...)
    ops = append(ops, PackageOps(&PackageService{})...)
    ops = append(ops, ShellOps(&ShellService{})...)
    ops = append(ops, ServiceManagerOps(&ServiceManagerService{})...)
    ops = append(ops, &ValidateOp{})
    return ops
}
```

### Verification

```bash
go build ./internal/execution/...
go test ./internal/execution/ -count=1

# Nuke and regenerate
rm internal/execution/ops_*_gen.go
star devlore ops generate
go build ./internal/execution/...
go test ./internal/execution/ -count=1
```

## Step 8: Generate Planned Receivers (devlore-cli)

### 8a: Generate plan_git_gen.go

From GitPlan's 3 methods (clone, checkout, pull). Delete `plan_git.go`.

The generated planned receiver:
- Embeds `Receiver` via `NewReceiver("plan.git")`
- `Attr` dispatch uses `MakeAttr`
- `AttrNames` sorted alphabetically
- Each method: `UnpackArgs` with `starlark.Value` params, `FillSlot` for each,
  create node, return `NewOutput`
- Operation names: `git.clone`, `git.checkout`, `git.pull`
- Node IDs: `generateNodeID("git.clone")`

### 8b: Generate plan_archive_gen.go

From ArchivePlan's 1 method (extract). Delete `plan_archive.go`.

### 8c: Hand-written planned receivers stay

- `plan_file.go`: `configure()` creates two nodes (render → copy). Multi-node
  pattern cannot be generated mechanically.
- `plan_package.go`: Variadic positional args with `argsToStrings()`. Different
  from standard `UnpackArgs`.
- `plan_root.go`: Top-level orchestration methods.

### Verification

```bash
go build ./internal/starlark/...
go test ./internal/starlark/ -count=1
```

## Step 9: Generate Execute Receivers (devlore-cli)

### 9a: Typed receivers → generated (`_gen.go`)

- Archive execute receiver → `receiver_archive_gen.go`. Delete `receiver_archive.go`.
- ServiceManager execute receiver → `receiver_service_gen.go`. Delete `receiver_service.go`.

### 9b: Hand-written execute receivers stay

| File | Reason |
|---|---|
| `receiver_git.go` | kwargs passthrough (CLI wrapper) |
| `receiver_npm.go` | kwargs passthrough |
| `receiver_docker.go` | kwargs passthrough |
| `receiver_shell.go` | `host.RunCommand` backing |
| `receiver_env.go` | `os.Getenv` backing |
| `receiver_http.go` | `net/http` backing |
| `receiver_log.go` | `io.Writer` backing |
| `receiver_package.go` | Feature flags, settings |

### 9c: Update bindings.go

Update constructor calls if receiver constructors changed.

### Verification

```bash
go build ./internal/starlark/...
go test ./internal/starlark/ -count=1
```

## Step 10: Context.Data Key Alignment (devlore-cli)

Ensure operational Context.Data keys align with slot names. Template data
keys (Username, Home, OS, etc.) stay CamelCase — they are Go template
conventions and user-authored templates reference them.

### 10a: Audit operational keys

| Current Key | Slot Name | Action |
|---|---|---|
| `"decryptor"` | `decryptor` | Aligned — no change |
| `"backup_suffix"` | `backup_suffix` | Aligned — no change |
| `"prune_empty_dirs"` | `prune` | Rename key to `prune` |
| `"prune_boundary"` | `prune_boundary` | Aligned — no change |
| `"validators"` | `validators` | Aligned — no change |
| `"git_mv"` | `git_mv` | Aligned — no change |

### 10b: Template data keys stay CamelCase

| Key | Type | Status |
|---|---|---|
| `"Username"` | Template data | Keep CamelCase |
| `"Home"` | Template data | Keep CamelCase |
| `"OS"` | Template data | Keep CamelCase |
| `"ARCH"` | Template data | Keep CamelCase |
| `"Hostname"` | Template data | Keep CamelCase |
| `"Segments"` | Template data | Keep CamelCase |

These are consumed by Go text/templates (`{{.Username}}`), not by slot
resolution. User-authored templates reference them. No rename.

### 10c: Fix prune_empty_dirs → prune

In `commands.go`, the undeploy command sets `cfg.TemplateData["prune_empty_dirs"]`.
Rename to `"prune"` to match the slot name on
`FileService.Unlink(path string, prune bool, ...)`.

### Verification

```bash
go test ./internal/writ/ -count=1
go test ./... -count=1
```

## Step 11: Template Separation (noblefactor-ops + devlore-cli)

Move devlore-specific code generation templates out of noblefactor-ops,
making `go.generate()` a generic Go code generation engine.

### 11a: Generic go.generate() in noblefactor-ops

Change `go.generate()` to accept a template string instead of a template name:

```python
# Current: hardcoded template names
go.generate("graph_ops", descriptor)

# Target: template content as input
tmpl = file.read("templates/graph_ops.go.tmpl")
code = go.generate(tmpl, descriptor)
```

Keep in noblefactor-ops (generic):
- `go.generate(template_content, descriptor)` — renders any Go template with a descriptor
- `go.mapping(descriptor)` — produces mapping YAML
- Descriptor types (`generateDescriptor`, `methodInfo`, `paramInfo`)
- `camelToSnake`, `validateReturnSignature`, `validateParamTypes`
- The `immediate_receiver` template — it only references noblefactor-ops types
  (`Receiver`, `MakeAttr`, `NoSuchAttrError`, `starlark.UnpackArgs`)

### 11b: Move devlore-specific templates to devlore-cli

Move to devlore-cli's Starlark extension resources:
- `planned_receiver` template — references `execution.Graph`, `execution.Node`,
  `host.Host`, `FillSlot`, `NewOutput`, `generateNodeID`
- `graph_ops` template — references `*Context`, `*Node`, `Operation`,
  `node.GetSlot()`, `ctx.DryRun`, `ctx.Logger`
- The `framework` bool and `slotReader` patterns — content pipeline,
  `io.Writer` logger injection, `ctx.Data` function injection
- The specific type mapping entries (`os.FileMode`, `[]byte` framework, etc.)

### 11c: Extension template loading

The devlore ops extension loads templates from its own resources:

```python
# star/extensions/com.noblefactor.devlore.Actions/commands/generate.star
graph_ops_tmpl = file.read(extension.resource("templates/graph_ops.go.tmpl"))
code = go.generate(graph_ops_tmpl, descriptor)
```

### Verification

```bash
# noblefactor-ops: no devlore-specific imports or types
cd /path/to/noblefactor-ops
go test ./internal/starlark/ -count=1

# devlore-cli: templates load from extension resources
cd /path/to/devlore-cli
star devlore ops generate
go build ./... && go test ./...
```

## Ordering and Dependencies

```
Step 1 (typed slots)
  → Step 2 (FillSlot typed conversions)
  → Step 3 (delete dispatch interfaces)
  → Step 4 (engine slot filling)
  → Step 5 (service extraction)
  → Step 7 (generate graph ops)

Step 6 (template updates, noblefactor-ops) — parallel with Steps 1-5

Step 7 (generate graph ops) requires Steps 5 + 6
Step 8 (generate planned receivers) requires Step 6
Step 9 (generate execute receivers) requires Step 6
Step 10 (key normalization) — can run anytime after Step 4
Step 11 (template separation) — after Steps 7-9 complete
```

Steps 1-4 are infrastructure. Step 5 extracts services. Step 6 updates
templates. Steps 7-9 generate and replace. Step 10 normalizes keys.
Step 11 separates concerns between repos.

## Scope Boundaries

| Pattern | Generated | Hand-Written |
|---|---|---|
| Ops interface (unexported) | All | - |
| Graph ops (typed slot assertions + delegation) | All 21 | - |
| Planned receivers (standard 1:1 method → node) | GitPlan, ArchivePlan | FilePlan, PackagePlan |
| Execute receivers (typed params) | Archive, ServiceManager | Git/Npm/Docker/Shell/Env/HTTP/Log/Package |
| Receiver embedding + MakeAttr + AttrNames | All generated | All hand-written use same pattern |
| Slot readers + dry-run logging | All generated | - |
| Registration functions | All generated | - |
| Service implementations | - | Services (source of truth) |
| Multi-node methods (configure) | - | Required |
| Variadic args pattern (PackagePlan) | - | Required |
| kwargs passthrough (CLI wrappers) | - | Required |
| PlanRoot top-level methods | - | Required |
| System bindings | - | Required |

## Files Created/Modified

### devlore-cli

| File | Action | Purpose |
|---|---|---|
| `internal/execution/graph.go` | Modify | Typed slots (any), delete Executable |
| `internal/execution/operation.go` | Modify | Delete Direct/Writer/Transform, single Operation |
| `internal/execution/executor.go` | Modify | Uniform op.Execute(), engine slot filling |
| `internal/starlark/output.go` | Modify | FillSlot stores native Go types |
| `internal/execution/file_service.go` | Create | FileService (hand-written) |
| `internal/execution/package_service.go` | Create | PackageService (hand-written) |
| `internal/execution/service_manager_service.go` | Create | ServiceManagerService (hand-written) |
| `internal/execution/ops_registry.go` | Create | AllOps() wiring + ValidateOp |
| `internal/execution/ops_file_gen.go` | Create | Generated: fileOps interface + 8 file ops |
| `internal/execution/ops_encryption_gen.go` | Create | Generated: 1 encryption op |
| `internal/execution/ops_package_gen.go` | Create | Generated: packageOps interface + 4 package ops |
| `internal/execution/ops_shell_gen.go` | Create | Generated: 2 shell ops |
| `internal/execution/ops_service_manager_gen.go` | Create | Generated: 5 service manager ops |
| `internal/execution/ops.go` | Delete | Replaced by service + `_gen.go` files |
| `internal/execution/ops_package.go` | Delete | Replaced by service + `_gen.go` files |
| `internal/execution/ops_service.go` | Delete | Replaced by service + `_gen.go` files |
| `internal/starlark/plan_git_gen.go` | Create | Generated: GitPlan receiver |
| `internal/starlark/plan_git.go` | Delete | Replaced by plan_git_gen.go |
| `internal/starlark/plan_archive_gen.go` | Create | Generated: ArchivePlan receiver |
| `internal/starlark/plan_archive.go` | Delete | Replaced by plan_archive_gen.go |
| `internal/starlark/receiver_archive_gen.go` | Create | Generated: Archive execute receiver |
| `internal/starlark/receiver_archive.go` | Delete | Replaced by receiver_archive_gen.go |
| `internal/starlark/receiver_service_gen.go` | Create | Generated: ServiceManager execute receiver |
| `internal/starlark/receiver_service.go` | Delete | Replaced by receiver_service_gen.go |
| `internal/writ/commands.go` | Modify | Rename `prune_empty_dirs` key to `prune` |

### noblefactor-ops

| File | Action | Purpose |
|---|---|---|
| `internal/starlark/receiver_go_gen.go` | Modify | Single Operation, ops interface, content model inference, namespace from service |

## Verification

After all steps, validate the nuke-and-regenerate workflow:

```bash
# Delete all generated files
rm internal/execution/ops_*_gen.go
rm internal/starlark/planned_*_gen.go
rm internal/starlark/immediate_*_gen.go

# Regenerate all
star devlore ops generate

# Everything compiles and tests pass
go build ./...
go test ./...
```

## Related Documents

- [2.1-typed-slots.md](../../architecture/2.1-typed-slots.md) — Typed slots architecture
- [2-execution-graph.md](../../architecture/2-execution-graph.md) — Graph structure and lifecycle
- [devlore-command-tree.md](../devlore-command-tree.md) — Command tree restructuring
- [phase-5.md](phase-5.md) — Previous plan (superseded by this document)
- Phases 0-4 — Generator pipeline (merged)

## Resolved Questions

- **Template data keys**: Stay CamelCase. User-authored Go templates reference
  `{{.Username}}`, `{{.Home}}`, etc. No rename.
- **Service ops**: Platform-agnostic. 5 ops (start, stop, restart, enable,
  disable) with internal platform dispatch. Same pattern as package.
  15 platform-specific ops collapse to 5.
- **`packages` slot type**: `[]string` natively. Remove comma-separated string.
  `FillSlot` converts `*starlark.List` → `[]string`.
- **Phase 5**: Superseded by this document. Note added to phase-5.md.
- **Dispatch interfaces**: `Direct`, `Writer`, `Transform` deleted. Single
  `Operation` interface with `Name()` and `Execute()`. Content model baked
  into each generated op by the generator.
- **Service naming**: Hand-written structs named `*Service` (`FileService`,
  `PackageService`, `ServiceManagerService`). Generated ops interface named
  `*Ops` (unexported: `fileOps`, `packageOps`, `serviceManagerOps`).
- **Namespace derivation**: Strip `Service` suffix, snake_case the remainder.
  `FileService` → `file`. `ServiceManagerService` → `service_manager`.
- **Generated code location**: Same package with `_gen.go` suffix, following Go
  convention. Generated code has access to unexported symbols and stays within
  the same logical unit. Header: `// Code generated from gen-receiver templates; DO NOT EDIT.`
