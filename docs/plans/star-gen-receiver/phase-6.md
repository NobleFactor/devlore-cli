# Phase 6: Typed Slots and Full Generation

## Context

Phases 0-4 built the generator pipeline in noblefactor-ops. Phase 5 planned the
devlore-cli restructuring (impl structs, nuke-safe generation). This phase
supersedes Phase 5 with architectural changes from the typed-slots design
(see [devlore-typed-slots.md](../../architecture/devlore-typed-slots.md)).

Key architectural decisions that change the approach:

| Phase 5 Assumption | Typed-Slots Architecture |
|---|---|
| `GetSlot` returns `string` | `GetSlot` returns `any` |
| `SlotValue.Immediate` is `string` | `SlotValue.Immediate` is `any` |
| Impl methods take `ctx *Context` first | Impl methods are stateless — no ctx param |
| Ops access `ctx.Data` directly | Engine fills unfilled slots from Context.Data |
| `Category() OpCategory` on every op | Removed; dispatch by interface type switch |
| `op_category` in descriptor | Return signature determines interface |
| `--category` flag required | Namespace derived from struct name |

**Repos**: noblefactor-ops (template changes), devlore-cli (infrastructure + restructure)

## Goals

1. **Typed slots.** `SlotValue.Immediate` is `any`. Slot types are determined
   by the implementation method's signature. No string-only slots.
2. **Slot resolution chain.** Caller-provided first, then engine fills unfilled
   slots from Context.Data. Operations never access ctx.Data directly.
3. **No OpCategory.** The executor dispatches via interface type switch
   (Direct/Writer/Transform). The `Category()` method and `OpCategory` enum
   are removed.
4. **Return signature is the spec.** The generator infers Direct/Writer/Transform
   from the impl method's return type. No annotation needed.
5. **Stateless impl structs.** Implementation methods receive all inputs as
   parameters. No ctx, no state, no side-channel data.
6. **All Starlark infrastructure is generated.** Plan receivers, graph ops,
   and real-time receivers are generated from implementation struct signatures.
7. **Nuke-safe.** Delete any `_gen.go`, re-run the generator, get it back.

## Current State

| Component | Current | Target |
|---|---|---|
| `SlotValue.Immediate` | `string` | `any` |
| `GetSlot` return type | `string` | `any` |
| `Executable.GetSlot` | `string` | `any` |
| `FillSlot` | Converts all to string | Stores native Go types |
| `OpCategory` enum | Present, used by executor | Removed |
| `Category()` on ops | Present on all 31 current ops | Removed |
| Engine slot filling | Not implemented | Fills unfilled from Context.Data |
| Ops ctx.Data access | 8 ops read ctx.Data directly | Zero — all through slots |
| Impl structs | Don't exist | Source of truth for generation |
| Generated graph ops | Don't exist | All 21 ops generated |
| Generated plan receivers | Don't exist | GitPlan, ArchivePlan generated |
| Generated real-time receivers | Don't exist | ArchiveReceiver, ServiceReceiver generated |

## Step 1: Typed Slot Infrastructure (devlore-cli)

Changes to `internal/execution/graph.go`.

### 1a: SlotValue.Immediate → any

```go
// graph.go line 230
type SlotValue struct {
    Immediate any    `json:"immediate,omitempty" yaml:"immediate,omitempty"`
    NodeRef   string `json:"node_ref,omitempty" yaml:"node_ref,omitempty"`
    Slot      string `json:"slot,omitempty" yaml:"slot,omitempty"`
}
```

`IsImmediate` changes: `s.Immediate != ""` → `s.Immediate != nil`.

### 1b: GetSlot → returns any

```go
// graph.go line 180
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
// graph.go line 192
func (n *Node) SetSlotImmediate(name string, value any) {
    if n.Slots == nil {
        n.Slots = make(map[string]SlotValue)
    }
    n.Slots[name] = SlotValue{Immediate: value}
}
```

### 1d: Executable interface

```go
// graph.go line 302
type Executable interface {
    GetID() string
    GetOperation() string
    GetSlot(name string) any
    GetProject() string
    GetMode() os.FileMode
}
```

### 1e: Cascade fixes

All callers of `GetSlot` that expect `string` must type-assert:

- `executor.go` lines 624, 636: `sortNodesByDepth` and `sortByDepth` use
  `node.GetSlot("path")` for depth sorting → `path, _ := node.GetSlot("path").(string)`
- `executor.go` line 426: source file reading →
  `source, _ := node.GetSlot("source").(string)`
- `output.go` line 55: `Output.Attr` returns slot value to Starlark →
  type-switch to convert `any` back to `starlark.Value`
- `output.go` line 163: `Output.Path()` → `o.node.GetSlot("path").(string)`
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
| `os.FileMode` via `starlark.Int` | `os.FileMode` | N/A (plan receiver handles) | Direct storage |

The `*Output` (promise) and `*Gather` paths are unchanged — they create edges,
not immediate values.

### Verification

```bash
go test ./internal/starlark/ -count=1
```

## Step 3: Remove OpCategory (devlore-cli)

### 3a: Delete OpCategory from operation.go

Remove `OpCategory` type, constants (`OpTransform`, `OpWriter`, `OpDirect`),
and `Category() OpCategory` from the `Operation` interface. The interface becomes:

```go
type Operation interface {
    Name() string
}
```

### 3b: Update executor content sourcing

`executor.go` line 425 uses `op.Category() != OpDirect` to decide whether to
read source content. Replace with interface type check:

```go
// Before
} else if op.Category() != OpDirect {

// After
} else if isContentOp(op) {
```

Where `isContentOp` checks `Writer` or `Transform`:

```go
func isContentOp(op Operation) bool {
    switch op.(type) {
    case Writer, Transform:
        return true
    }
    return false
}
```

### 3c: Remove Category() from all ops

Delete the `Category() OpCategory` method from every op struct in `ops.go`,
`ops_package.go`, `ops_service.go` (31 current ops; reduced to 21 after
service unification in Step 5).

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
// executor.go
func (e *GraphExecutor) fillSlotsFromData(node Executable) {
    n, ok := node.(*Node)
    if !ok {
        return
    }
    for key, value := range e.options.Data {
        if _, exists := n.Slots[key]; !exists {
            n.SetSlotImmediate(key, value)
        }
    }
}
```

### 4b: Call before dispatch

In `executeExecutableWithOutputs`, call `e.fillSlotsFromData(node)` before
the content sourcing and dispatch logic.

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

## Step 5: Implementation Struct Extraction (devlore-cli)

Extract business logic into stateless implementation structs. Each method
receives all inputs as parameters — no ctx, no ctx.Data access.

### 5a: impl_file.go

```go
type fileOps struct{}

func (f *fileOps) Link(source, path string) error
func (f *fileOps) Copy(path string, mode os.FileMode, content []byte) (string, error)
func (f *fileOps) Render(templateData map[string]any, source, path, project string, content []byte) ([]byte, error)
func (f *fileOps) Decrypt(decryptor func(string, []byte) ([]byte, error), source string, content []byte) ([]byte, error)
func (f *fileOps) Backup(path, backupSuffix string) error
func (f *fileOps) Unlink(path string, prune bool, pruneBoundary string) error
func (f *fileOps) Remove(path string, prune bool, pruneBoundary string) error
func (f *fileOps) Write(content, path string, mode os.FileMode) error
func (f *fileOps) Validate(validators map[string]func() error, check, message string) error
func (f *fileOps) Move(gitMv func(src, dst string) error, source, path string) error
```

Every parameter except the framework `content []byte` (last param on
Writer/Transform methods) maps to a slot. Parameters like `decryptor`,
`validators`, `templateData`, `backupSuffix`, `prune`, `pruneBoundary`,
`gitMv` come from Context.Data via engine slot filling.

Helper functions (`pruneEmptyParents`, `isSubpath`) move to this file.

### 5b: impl_package.go

```go
type packageOps struct{}

func (p *packageOps) Install(packages []string, manager string, cask bool) error
func (p *packageOps) Upgrade(packages []string, manager string, cask bool) error
func (p *packageOps) Remove(packages []string, manager string, cask bool) error
func (p *packageOps) Update(manager string) error
func (p *packageOps) Shell(command string) error
func (p *packageOps) PowerShell(command string) error
```

Helper functions (`resolvePMForInstall`, `resolvePMForUpgrade`,
`resolvePMForRemove`, brew cask helpers) stay in this file.

`packages` is `[]string` — stored natively in the typed slot. The
`parsePackages` helper (comma-separated string splitting) is removed.
`FillSlot` handles `*starlark.List` → `[]string` conversion at plan time.

### 5c: impl_service.go

Platform-agnostic service operations. Same pattern as package: the caller
says `service.start("foo")` and the impl handles platform dispatch internally.
15 platform-specific ops collapse to 5.

```go
type serviceOps struct{}

func (s *serviceOps) Start(name string) error
func (s *serviceOps) Stop(name string) error
func (s *serviceOps) Restart(name string) error
func (s *serviceOps) Enable(name string) error
func (s *serviceOps) Disable(name string) error
```

Each method detects the platform at runtime (`runtime.GOOS`) and dispatches
to the appropriate service manager (launchd on darwin, systemd on linux,
sc on windows). The 15 current ops (`LaunchdStartOp`, `SystemdStartOp`,
`WinServiceStartOp`, etc.) are replaced by 5 ops (`ServiceStartOp`, etc.).

### Verification

Impl structs compile standalone. No tests yet — they're exercised through
the generated ops in Step 7.

```bash
go build ./internal/execution/...
```

## Step 6: Generator Template Updates (noblefactor-ops)

### 6a: Remove op_category from descriptor

Delete `OpCategory` field from `methodInfo`. Delete `op_category` handling
from `methodInfoFromValue`. Add `Interface` field (inferred from return type):

```go
type methodInfo struct {
    GoName    string
    SnakeName string
    Params    []paramInfo
    ReturnType string
    Interface  string // "Direct", "Writer", "Transform"
    Doc        string
}
```

In `descriptorFromValue` or `goGenerate`, after `validateReturnSignature`
extracts the value type:

```go
switch valueType {
case "string":
    m.Interface = "Writer"
case "[]byte":
    m.Interface = "Transform"
default:
    m.Interface = "Direct"
}
```

For methods that return `error` only (not `(T, error)`), the gate validation
needs a new path. Currently `validateReturnSignature` rejects bare `error`.
Add support:

```go
if returns == "error" {
    return "", nil  // empty valueType signals Direct
}
```

### 6b: Graph ops template — remove Category(), typed assertions

Replace `{{if eq .OpCategory "OpWriter"}}` with `{{if eq .Interface "Writer"}}`.

Remove all `Category() OpCategory` lines.

Replace `tplSlotReaders` with typed assertion code:

```go
// For string params:
source, ok := node.GetSlot("source").(string)
if !ok {
    return fmt.Errorf("{{$.Category}}.{{.SnakeName}}: slot \"source\" requires string")
}

// For bool params:
prune, _ := node.GetSlot("prune").(bool)

// For function params:
decryptor, ok := node.GetSlot("decryptor").(func(string, []byte) ([]byte, error))
if !ok {
    return nil, fmt.Errorf("{{$.Category}}.{{.SnakeName}}: slot \"decryptor\" requires func")
}
```

The assertion pattern depends on the Go type. Required params (string, func)
use `ok` check with error. Optional/defaultable params (bool, int) use
zero-value fallback.

### 6c: Graph ops template — delegation without ctx

Impl methods don't take ctx. The delegation call passes only slot values:

```go
return o.impl.{{.GoName}}({{implArgs .Params}})           // Direct
return o.impl.{{.GoName}}({{implArgs .Params}}, content)  // Writer/Transform
```

### 6d: Namespace from struct name

Add a `namespaceFromStruct` function that strips known suffixes and converts
to snake_case:

| Struct | Strip Suffix | Namespace |
|---|---|---|
| `fileOps` | `Ops` | `file` |
| `packageOps` | `Ops` | `package` |
| `serviceOps` | `Ops` | `service` |
| `GitPlan` | `Plan` | `git` |
| `ArchiveReceiver` | `Receiver` | `archive` |

The `category` field in the descriptor is derived from this — no `--category`
CLI flag needed.

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
from Context.Data at runtime. The generator skips them in plan receiver templates
(no UnpackArgs entry) but includes them in graph ops templates (slot assertion).

### 6f: Skip framework params in slot generation

The generator must distinguish slot params from framework params. Convention:

- **Framework content**: The last `[]byte` param on Writer/Transform methods.
  Identified by: method returns `(string, error)` or `([]byte, error)`, AND
  the last param's type is `[]byte`. This param is NOT a slot — it's the
  content pipeline.
- **All other params**: Slots.

No annotation needed. The method signature IS the specification.

### 6g: Tests

Update existing tests, add new tests:

- `TestGenerateGraphOpsTypedSlots`: Verify typed assertion code for string,
  bool, func params.
- `TestGenerateGraphOpsWriterInferred`: Method with `(string, error)` return
  generates `Write()` without explicit op_category.
- `TestGenerateGraphOpsTransformInferred`: Method with `([]byte, error)` return
  generates `Transform()`.
- `TestGenerateGraphOpsDirectInferred`: Method with `error` return generates
  `Execute()`.
- `TestGenerateGraphOpsNoCategory`: Verify no `Category()` method in output.
- `TestNamespaceFromStruct`: Verify suffix stripping and snake_case.

### Verification

```bash
cd /Users/david-noble/Workspace/NobleFactor/noblefactor-ops
go test ./internal/starlark/ -run TestGenerate -count=1
go test ./internal/starlark/ -count=1
go build ./... && go vet ./internal/starlark/
```

## Step 7: Generate and Replace Graph Ops (devlore-cli)

### 7a: Generate ops_file_gen.go

Run the generator against `fileOps`. The generated file contains:

- `FileLinkOp` (Direct), `FileCopyOp` (Writer), `FileRenderOp` (Transform),
  `FileDecryptOp` (Transform), `FileBackupOp` (Direct), `FileUnlinkOp` (Direct),
  `FileRemoveOp` (Direct), `FileWriteOp` (Direct), `FileValidateOp` (Direct),
  `FileMoveOp` (Direct)
- Each op: struct with `impl *fileOps`, `Name()`, Execute/Write/Transform
  with typed slot assertions + dry-run + delegation
- `FileOps() []Operation` registration function

### 7b: Generate ops_package_gen.go

From `packageOps`. 6 ops (Install, Upgrade, Remove, Update, Shell, PowerShell).

### 7c: Generate ops_service_gen.go

From `serviceOps`. 5 platform-agnostic ops (Start, Stop, Restart, Enable,
Disable). Operation names: `service.start`, `service.stop`, etc.

### 7d: Delete hand-written ops

Remove `ops.go` (file ops + AllOps), `ops_package.go`, `ops_service.go`.

Move `AllOps()` to a new small wiring file or into one of the generated files.
`AllOps` calls `FileOps()`, `PackageOps()`, `ServiceOps()`.

### Verification

```bash
go build ./internal/execution/...
go test ./internal/execution/ -count=1

# Nuke and regenerate
rm internal/execution/ops_*_gen.go
# Run generator for each impl struct
go build ./internal/execution/...
go test ./internal/execution/ -count=1
```

## Step 8: Generate Plan Receivers (devlore-cli)

### 8a: Generate plan_git_gen.go

From GitPlan's 3 methods (clone, checkout, pull). Delete `plan_git.go`.

The generated plan receiver:
- Embeds `Receiver` via `NewReceiver("plan.git")`
- `Attr` dispatch uses `MakeAttr`
- `AttrNames` sorted alphabetically
- Each method: `UnpackArgs` with `starlark.Value` params, `FillSlot` for each,
  create node, return `NewOutput`
- Operation names: `git.clone`, `git.checkout`, `git.pull`
- Node IDs: `generateNodeID("git.clone")`

### 8b: Generate plan_archive_gen.go

From ArchivePlan's 1 method (extract). Delete `plan_archive.go`.

### 8c: Hand-written plan receivers stay

- `plan_file.go`: `configure()` creates two nodes (render → copy). Multi-node
  pattern cannot be generated mechanically.
- `plan_package.go`: Variadic positional args with `argsToStrings()`. Different
  from standard `UnpackArgs`.
- `plan_root.go`: Top-level orchestration methods.

### 8d: Update plan_root.go constructors

If generated plan receiver constructors changed signature (they shouldn't —
`NewGitPlan(graph, host, project)` matches the template output), verify
`plan_root.go` still compiles.

### Verification

```bash
go build ./internal/starlark/...
go test ./internal/starlark/ -count=1
```

## Step 9: Generate Real-Time Receivers (devlore-cli)

### 9a: Typed receivers → generated

- `receiver_archive_gen.go` from ArchiveReceiver's typed params (extract).
  Delete `receiver_archive.go`.
- `receiver_service_gen.go` from ServiceReceiver's typed params.
  Delete `receiver_service.go`.

### 9b: Hand-written receivers stay

| Receiver | Reason |
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
`fileOps.Unlink(path string, prune bool, ...)`.

### Verification

```bash
go test ./internal/writ/ -count=1
go test ./... -count=1
```

## Ordering and Dependencies

```
Step 1 (typed slots)
  → Step 2 (FillSlot typed conversions)
  → Step 3 (remove OpCategory)
  → Step 4 (engine slot filling)
  → Step 5 (impl struct extraction)
  → Step 7 (generate graph ops)

Step 6 (template updates, noblefactor-ops) — parallel with Steps 1-5

Step 7 (generate graph ops) requires Steps 5 + 6
Step 8 (generate plan receivers) requires Step 6
Step 9 (generate real-time receivers) requires Step 6
Step 10 (key normalization) — can run anytime after Step 4
```

Steps 1-4 are infrastructure. Step 5 extracts impl structs. Step 6 updates
templates. Steps 7-9 generate and replace. Step 10 normalizes keys.

## Scope Boundaries

| Pattern | Generated | Hand-Written |
|---|---|---|
| Graph ops (typed slot assertions + delegation) | All 21 | - |
| Plan receivers (standard 1:1 method → node) | GitPlan, ArchivePlan | FilePlan, PackagePlan |
| Real-time receivers (typed params) | Archive, Service | Git/Npm/Docker/Shell/Env/HTTP/Log/Package |
| Receiver embedding + MakeAttr + AttrNames | All generated | All hand-written use same pattern |
| Slot readers + dry-run logging | All generated | - |
| Registration functions with impl | All generated | - |
| Implementation bodies | - | impl structs (source of truth) |
| Multi-node methods (configure) | - | Required |
| Variadic args pattern (PackagePlan) | - | Required |
| kwargs passthrough (CLI wrappers) | - | Required |
| PlanRoot top-level methods | - | Required |
| System bindings | - | Required |

## Files Created/Modified

### devlore-cli

| File | Action | Purpose |
|---|---|---|
| `internal/execution/graph.go` | Modify | Typed slots (any), GetSlot returns any |
| `internal/execution/operation.go` | Modify | Remove OpCategory |
| `internal/execution/executor.go` | Modify | Type-switch content sourcing, engine slot filling |
| `internal/starlark/output.go` | Modify | FillSlot stores native Go types |
| `internal/execution/impl_file.go` | Create | fileOps implementation struct |
| `internal/execution/impl_package.go` | Create | packageOps implementation struct |
| `internal/execution/impl_service.go` | Create | serviceOps implementation struct |
| `internal/execution/ops_file_gen.go` | Create | Generated file ops |
| `internal/execution/ops_package_gen.go` | Create | Generated package ops |
| `internal/execution/ops_service_gen.go` | Create | Generated service ops |
| `internal/execution/ops.go` | Delete | Replaced by impl_file.go + ops_file_gen.go |
| `internal/execution/ops_package.go` | Delete | Replaced by impl_package.go + ops_package_gen.go |
| `internal/execution/ops_service.go` | Delete | Replaced by impl_service.go + ops_service_gen.go |
| `internal/starlark/plan_git_gen.go` | Create | Generated GitPlan |
| `internal/starlark/plan_archive_gen.go` | Create | Generated ArchivePlan |
| `internal/starlark/plan_git.go` | Delete | Replaced by plan_git_gen.go |
| `internal/starlark/plan_archive.go` | Delete | Replaced by plan_archive_gen.go |
| `internal/starlark/receiver_archive_gen.go` | Create | Generated ArchiveReceiver |
| `internal/starlark/receiver_service_gen.go` | Create | Generated ServiceReceiver |
| `internal/starlark/receiver_archive.go` | Delete | Replaced by receiver_archive_gen.go |
| `internal/starlark/receiver_service.go` | Delete | Replaced by receiver_service_gen.go |
| `internal/writ/commands.go` | Modify | Rename `prune_empty_dirs` key to `prune` |

### noblefactor-ops

| File | Action | Purpose |
|---|---|---|
| `internal/starlark/receiver_go_gen.go` | Modify | Remove op_category, typed assertions, return-type inference, namespace from struct |

## Verification

After all steps, validate the nuke-and-regenerate workflow:

```bash
# Delete all generated files
rm internal/execution/ops_*_gen.go
rm internal/starlark/plan_*_gen.go
rm internal/starlark/receiver_*_gen.go

# Regenerate all
star gen.receiver --struct fileOps --path ./internal/execution --templates graph_ops --write
star gen.receiver --struct packageOps --path ./internal/execution --templates graph_ops --write
star gen.receiver --struct serviceOps --path ./internal/execution --templates graph_ops --write
star gen.receiver --struct GitPlan --path ./internal/starlark --templates plan_receiver --write
star gen.receiver --struct ArchivePlan --path ./internal/starlark --templates plan_receiver --write
# ... real-time receivers

# Everything compiles and tests pass
go build ./...
go test ./...
```

## Related Documents

- [devlore-typed-slots.md](../../architecture/devlore-typed-slots.md) — Typed slots architecture
- [devlore-execution-graph.md](../../architecture/devlore-execution-graph.md) — Graph structure and lifecycle
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
