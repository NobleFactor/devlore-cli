# Phase 4: Validate Against Existing Hand-Written Code

## Context

Phase 0-3 are merged. The generator pipeline is complete: `go.methods()` discovers
signatures, `go.generate()` produces code from templates, and `star gen.receiver`
orchestrates the full workflow. Phase 4 validates the generated output against
devlore-cli's hand-written plan receivers, graph operations, and real-time receivers.

The goal is to identify every structural difference between generated and hand-written
code, then decide for each: adjust the template, or accept the difference as an
intentional deviation that hand-written code has but generated code doesn't need.

**Repos**: noblefactor-ops (template fixes), devlore-cli (validation targets)

## Known Mismatches

Comparing the Phase 2 templates against the hand-written code reveals several
structural differences. These are documented here so each can be addressed
systematically.

### Mismatch 1: Operation Name Format

| Source | Format | Example |
|---|---|---|
| Generated (Phase 2) | `category.method` | `git.clone` |
| Hand-written | `category-method` | `git-clone` |

The hand-written code uses hyphens (`git-clone`, `archive-extract`, `package-install`).
The templates use dots (`git.clone`, `archive.extract`, `package.install`).

**Decision**: This is a naming convention choice. The dot format is the intended
convention going forward — it matches the Starlark namespace pattern (`plan.git.clone`)
and the operation registry lookup. The hand-written code predates this convention.
**No template change needed.** Phase 4 documents this as an intentional divergence.

### Mismatch 2: Node ID Prefix

| Source | Format | Example |
|---|---|---|
| Generated | `generateNodeID("method")` | `clone-1` |
| Hand-written | `generateNodeID("category-method")` | `git-clone-1` |

The hand-written code includes the category in the node ID prefix for uniqueness
across categories (e.g., `git-clone-1` vs `file-copy-1`).

**Decision**: The generator should include the category. **Template change needed.**
Update the plan receiver template to use `generateNodeID("{{$.Category}}.{{.SnakeName}}")`.

### Mismatch 3: Graph Ops — OpCategory

| Source | Category | Example |
|---|---|---|
| Generated | All `OpDirect` | Every op gets `Execute(ctx, node) error` |
| Hand-written | Mixed | `CopyOp` = `OpWriter`, `RenderOp`/`DecryptOp` = `OpTransform` |

The generator produces `OpDirect` with `Execute()` for all operations. Hand-written
code uses three interfaces:
- `Direct.Execute(ctx, node) error` — link, remove, write, backup, validate, move
- `Writer.Write(ctx, node, content) (checksum, error)` — copy
- `Transform.Transform(ctx, node, content) ([]byte, error)` — render, decrypt

The correct category depends on the operation's data flow, which cannot be inferred
from the method signature alone.

**Decision**: Add an optional `op_category` field to the method descriptor. Defaults
to `OpDirect`. The `.star` command can set this per-method via a mapping or flag.
**Template change needed** — the graph_ops template should read `op_category` from
each method and generate the correct interface implementation.

### Mismatch 4: Real-Time Receiver — kwargs Passthrough

| Source | Pattern | Example |
|---|---|---|
| Generated | Typed `UnpackArgs` | `var source string; starlark.UnpackArgs(...)` |
| Hand-written (GitReceiver) | `passThrough()` | kwargs → CLI flags, exec git |

The GitReceiver doesn't use typed parameter unpacking at all. It converts arbitrary
kwargs to CLI flags (`--branch main` → `-b main`). This is a fundamentally different
pattern — the receiver is a thin CLI wrapper, not a typed API.

**Decision**: The generated real-time receiver template is correct for typed APIs
(e.g., a future `DockerReceiver` with explicit params). The kwargs-passthrough pattern
is specific to CLI wrappers and won't be generated — it's hand-written by design.
**No template change needed.** Document that CLI-wrapper receivers are out of scope.

### Mismatch 5: Plan Receiver — starlark.Value Args

| Source | Unpack Type | Why |
|---|---|---|
| Generated | `starlark.Value` for all params | Supports both promises (Output) and immediates |
| Hand-written | `starlark.Value` for all params | Same reason |

**No mismatch.** The plan receiver template correctly uses `starlark.Value` because
plan-time arguments can be either literals or promises from upstream nodes.

### Mismatch 6: FillSlot Variable Name

| Source | Variable | Example |
|---|---|---|
| Generated | `{{.GoName}}` | `FillSlot(node, p.graph, "url", url)` |
| Hand-written | Same | `FillSlot(node, g.graph, "url", url)` |

The variable names match (param GoName is lowercased in Starlark). But the generated
code uses `p` as the receiver name and hand-written code varies (`g` for GitPlan,
`a` for ArchivePlan, `fp` for FilePlan).

**Decision**: The single-letter receiver name is a style preference. `p` is fine for
generated code. **No template change needed.**

### Mismatch 7: Multi-Node Methods (configure)

`FilePlan.configure()` creates TWO nodes (render → copy) with an internal edge.
The generator produces one node per method.

**Decision**: Multi-node methods require custom logic that the generator cannot
produce mechanically. These are hand-written by design. The generator handles the
common 1:1 method→node pattern. **No template change needed.** Document that
multi-node methods are out of scope.

## Step 1: Fix Node ID Prefix (noblefactor-ops)

Update the plan receiver template in `receiver_go_gen.go`:

```go
// Before:
ID:        generateNodeID("{{.SnakeName}}"),

// After:
ID:        generateNodeID("{{$.Category}}.{{.SnakeName}}"),
```

Update `TestGeneratePlanReceiver` to check for the new format.

## Step 2: Add OpCategory Support to Graph Ops Template (noblefactor-ops)

### 2a: Extend methodInfo with OpCategory

Add `OpCategory string` field to `methodInfo` in `receiver_go_gen.go`. Default to
`"OpDirect"` when not set in the descriptor.

### 2b: Update methodInfoFromValue

Read optional `op_category` from the descriptor dict. Valid values:
`"direct"`, `"writer"`, `"transform"`. Map to Go constants:

| Descriptor Value | Go Constant | Interface |
|---|---|---|
| `"direct"` (default) | `OpDirect` | `Execute(ctx *Context, node Executable) error` |
| `"writer"` | `OpWriter` | `Write(ctx *Context, node Executable, content []byte) (string, error)` |
| `"transform"` | `OpTransform` | `Transform(ctx *Context, node Executable, content []byte) ([]byte, error)` |

### 2c: Update graph_ops template

The template needs three method signatures based on `OpCategory`:

```go
{{if eq .OpCategory "OpWriter"}}
func (o *{{$.StructName}}{{.GoName}}Op) Category() OpCategory { return OpWriter }

func (o *{{$.StructName}}{{.GoName}}Op) Write(ctx *Context, node Executable, content []byte) (string, error) {
    // ...slots + dry-run + TODO
}
{{else if eq .OpCategory "OpTransform"}}
func (o *{{$.StructName}}{{.GoName}}Op) Category() OpCategory { return OpTransform }

func (o *{{$.StructName}}{{.GoName}}Op) Transform(ctx *Context, node Executable, content []byte) ([]byte, error) {
    // ...slots + dry-run + TODO
}
{{else}}
func (o *{{$.StructName}}{{.GoName}}Op) Category() OpCategory { return OpDirect }

func (o *{{$.StructName}}{{.GoName}}Op) Execute(ctx *Context, node Executable) error {
    // ...slots + dry-run + TODO
}
{{end}}
```

### 2d: Update gen-receiver.star (optional)

Add `--op-categories` flag or a mapping file that specifies per-method categories.
For Phase 4, the default (`direct`) is sufficient. Per-method overrides can be
added in the `.star` command later.

## Step 3: Update Tests (noblefactor-ops)

### `TestGeneratePlanReceiver`

Update assertion for node ID to include category:
```go
// Before:
if !strings.Contains(code, `generateNodeID("copy")`) {

// After:
if !strings.Contains(code, `generateNodeID("file.copy")`) {
```

### `TestGenerateGraphOpsWriter`

New test: descriptor with a method that has `op_category: "writer"`. Validate:
- Contains `Category() OpCategory { return OpWriter }`
- Contains `Write(ctx *Context, node Executable, content []byte)`
- Valid Go syntax

### `TestGenerateGraphOpsTransform`

New test: descriptor with a method that has `op_category: "transform"`. Validate:
- Contains `Category() OpCategory { return OpTransform }`
- Contains `Transform(ctx *Context, node Executable, content []byte)`
- Valid Go syntax

### `TestGenerateGraphOpsDefaultDirect`

Existing `TestGenerateGraphOps` already covers this — verify it still passes
with no `op_category` set (defaults to `OpDirect`).

## Step 4: Structural Validation Against Hand-Written Code

Run the generator against devlore-cli's known patterns and compare.

### 4a: GitPlan (simplest — 3 methods, no deviations)

```bash
star gen.receiver \
  --path /path/to/devlore-cli/internal/starlark \
  --struct GitPlan \
  --category git \
  --templates plan_receiver
```

Compare generated `plan_git_gen.go` against `plan_git.go`:
- [x] Struct: `GitPlan` with `graph`, `host`, `project` fields
- [x] Constructor: `NewGitPlan(graph, host, project)`
- [x] starlark.Value methods: String, Type, Freeze, Truth, Hash
- [x] Attr switch: `clone`, `checkout`, `pull`
- [x] AttrNames: sorted alphabetically
- [x] Methods: UnpackArgs with starlark.Value, FillSlot, NewOutput
- [ ] Operation name: generated uses `git.clone`, hand-written uses `git-clone`
- [ ] Node ID: generated uses `generateNodeID("git.clone")`, hand-written uses `generateNodeID("git-clone")`

Documented as intentional (Mismatch 1). The dot format is the new convention.

### 4b: ArchivePlan (1 method — minimal)

```bash
star gen.receiver \
  --path /path/to/devlore-cli/internal/starlark \
  --struct ArchivePlan \
  --category archive \
  --templates plan_receiver
```

Compare against `plan_archive.go`. Expected: near-identical structure.

### 4c: PackageOps — Graph Ops

```bash
star gen.receiver \
  --path /path/to/devlore-cli/internal/execution \
  --struct PackageInstallOp \
  --category package \
  --templates graph_ops
```

Compare against `ops_package.go`. Key differences:
- Hand-written has complex package manager resolution (resolvePMForInstall)
- Hand-written reads `packages`, `manager`, `cask` slots
- Generator produces the structural scaffold; implementation logic is TODO

## Step 5: Document Scope Boundaries

Create a section in the master plan or a README noting what the generator handles
vs what requires hand-writing:

| Pattern | Generated | Hand-Written |
|---|---|---|
| Single method → single node | Yes | - |
| Typed parameter unpacking | Yes | - |
| FillSlot for promises/immediates | Yes | - |
| starlark.Value interface boilerplate | Yes | - |
| Attr/AttrNames dispatch | Yes | - |
| Op struct + Name() + Category() | Yes | - |
| Slot readers in Execute/Write/Transform | Yes | - |
| Multi-node methods (configure) | No | Required |
| kwargs passthrough (CLI wrappers) | No | Required |
| Custom validation logic (service actions) | No | Required |
| Package manager resolution | No | Required |
| Operation implementation bodies | Scaffold only | Required |

## Verification

```bash
cd /Users/david-noble/Workspace/NobleFactor/noblefactor-ops

# New and updated tests pass
go test ./internal/starlark/ -run "TestGenerate" -count=1

# Existing tests still pass
go test ./internal/starlark/ -count=1

# Full build + vet
go build ./... && go vet ./internal/starlark/
```
