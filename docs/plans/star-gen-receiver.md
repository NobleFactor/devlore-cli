---
title: "Star Extension for Generating Receivers and Graph Operations"
status: draft
created: 2026-02-12
updated: 2026-02-12
---

# Plan: Star Extension for Generating Receivers and Graph Operations

## Summary

Every planned receiver (`plan_file.go`, `plan_git.go`), graph operation (`ops_package.go`),
and immediate receiver (`receiver_git.go`) follows an identical mechanical pattern.
Writing these by hand is tedious and error-prone. This plan creates a star extension
(`com.noblefactor.star.GenReceiver`) that reads a Go implementation struct via the
existing `go.methods()` / `go.structs()` AST primitives, then generates the three kinds
of boilerplate. Input: path to a Go package + struct name. Output: generated `_gen.go`
files ready to compile.

## Goals

1. **Eliminate hand-written boilerplate**: Generate planned receivers, graph operations,
   and immediate receivers from Go struct definitions
2. **Enforce the receiver pattern**: All generated code uses `Receiver` base type with
   `Attr`/`AttrNames` — never dict-based `FromStringDict` namespaces
3. **Extend AST primitives**: Add parameter info to `go.methods()` and `go.funcs()` so
   the generator knows method signatures

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `go.methods()` / `go.funcs()` | Partial | Returns name, receiver, returns, doc, scope — no params |
| `typeToString()` | Partial | Missing Ellipsis, InterfaceType, FuncType, ChanType, IndexExpr |
| Planned receivers | Manual | `plan_file.go`, `plan_git.go` etc. written by hand |
| Graph operations | Manual | `ops_package.go` etc. written by hand |
| Immediate receivers | Manual | `receiver_git.go` etc. written by hand |
| Dict-based receiver ban | Enforced | `TestNoDictBasedReceivers` catches violations in CI |

## Graph Execution Model

### Single-Operation Nodes

Each node executes exactly one operation. `Node.Operations []string` is replaced by
`Node.Operation string`. Chained operations (e.g., decrypt → expand → copy) are
expressed as separate nodes connected by edges, with data flowing through
slots and promises — the same mechanism used for all inter-node data binding.

This eliminates the implicit `[]byte` accumulator in the executor pipeline loop.
The planned receiver is responsible for decomposing multi-step workflows into
individual nodes with edges between them.

### Typed Operation Results

Operations carry their own input/output contracts as typed objects rather than
relying on generic fields on `Node`. Currently, `Node` has `SourceChecksum` and
`TargetChecksum` fields — but checksums are only meaningful for specific operations
(e.g., `copy` produces a target checksum, `render` produces a source checksum,
`link` produces neither). Generic fields on every node conflate the node's
structural identity with operation-specific state.

The executor's `Result` type should return a typed result object per operation.
Each operation defines what data it produces:

| Operation | Result Data |
|---|---|
| `copy` | `TargetChecksum string` |
| `render` | `SourceChecksum string`, rendered content (flows via outputs map) |
| `link` | (none — symlink has no content checksum) |
| `package_install` | installed version, package manager used |

The node stores the operation's result object (serialized to the receipt), not
a fixed set of checksum fields. This generalizes: any operation-specific state
(checksums, versions, paths created, etc.) lives in the operation's typed result,
not in generic node fields.

This aligns with the code generator: the generator reads the operation struct's
return type and produces the correct serialization. No hand-maintained field lists.

### Implementation-to-Node Mapping

The mapping from Go implementation structs to graph nodes is fully mechanical
and discovered from the source:

| Implementation | Graph Concept | Example |
|---|---|---|
| Struct name | Node category | `FileOps` → `file` |
| Method name | Node operation | `FileOps.Copy` → `file.copy` |
| Node ID | `category.name-N` | `file.copy-1`, `file.copy-2` |
| Method params | Input slots | `(source, path string)` → slots `source`, `path` |
| Return value | Output edge | → edge to named slot on downstream node |

**Node ID**: The `-N` suffix is an auto-incrementing counter that tracks
how many nodes of a given category/name exist in the execution graph.

**Input slots** are filled by either:
- An edge (promise from an upstream node's output)
- A literal value provided at plan time

**Output** is the fulfillment of a promise. When a downstream node has an
input slot that references this node, the executor resolves the promise
via the edge.

### Name Normalization

All Go names are normalized to snake_case for Starlark and graph identifiers:

| Go Name | Starlark / Graph Name |
|---|---|
| `File` | `file` |
| `Copy` | `copy` |
| `WalkTree` | `walk_tree` |
| `ConfigGet` | `config_get` |
| `PackageInstall` | `package_install` |

This applies to: category names, operation names, slot names, and Starlark
method names.

### Error Handling

Error handling follows the PowerShell `ErrorActionPreference` model. Each node
declares how errors propagate via a per-node `ErrorAction` field:

```go
ErrorAction ErrorAction `json:"error_action" yaml:"error_action"`
```

| ErrorAction | Behavior |
|---|---|
| `ErrorActionStop` | Node fails → graph stops, no downstream nodes execute |
| `ErrorActionContinue` | Node fails → log error, skip dependent downstream nodes (unfulfilled promise), independent branches continue |
| `ErrorActionSilent` | Same as Continue but suppress error output |

This replaces and subsumes the current `ConflictPolicy` on `ExecutorOptions`.
A conflict is an error from the operation — the node's `ErrorAction` determines
what happens next. The executor loop checks the failing node's `ErrorAction`
instead of a global `ConflictPolicy` setting.

Operations return `(value, error)`. The value flows through the output edge
to downstream nodes. The error is processed according to the node's `ErrorAction`.
When a node with `ErrorActionContinue` fails, the executor skips all downstream
nodes whose input slots reference the failed node's output (promise unfulfilled),
but independent branches of the graph continue executing.

### Implementation Struct Contract

Implementation structs are the source of truth for code generation. The generator
enforces two hard constraints — violation is a generator error, not a TODO.

**Gate 1: All types must map to Starlark.** Every parameter type and return type
must resolve to a concrete Starlark type. If a method has a parameter or return
type outside the supported set, the generator errors and reports the method name
and the unmappable type.

Supported type mappings:

| Go Type | Starlark Type | Direction |
|---|---|---|
| `string` | `starlark.String` | param + return |
| `bool` | `starlark.Bool` | param + return |
| `int`, `int64` | `starlark.Int` | param + return |
| `[]string` | `*starlark.List` | param + return |
| `...string` | positional `*args` | param only |

Any type outside this set → generator error.

**Gate 2: Return signature must be `(T, error)`.** Every method returns
exactly one value and one error — standard Go convention, no exceptions:

| Return Signature | Meaning |
|---|---|
| `(string, error)` | Value flows through output edge, error checked by `ErrorAction` |
| `(bool, error)` | Same |
| `(int, error)` | Same |
| `([]string, error)` | Same |
| `(MyStruct, error)` | Struct value → generated Starlark struct type (see below) |

Methods with any other return shape — `error` alone, `(string, int, error)`,
`([]byte, error)`, no return — are rejected. Generator error.

The implementation struct is a contract. If a method doesn't fit the mold,
the generator rejects it and tells you exactly what failed. No partial output,
no placeholders.

### Starlark Struct Types

When an implementation method returns `(SomeStruct, error)`, the generator
must produce a matching Starlark struct type. These types are data structs
(read-only attribute bags), not namespace receivers.

**Canonical location**: `internal/starlark/types.go` for hand-written types,
`internal/starlark/types_gen.go` for generator output.

**Create-once semantics**: Starlark struct types are stable artifacts. They
are generated or hand-written once, then reused across all receivers that
return the same Go struct. The generator checks whether a converter already
exists before creating one:

1. If `types.go` or `types_gen.go` already has a converter for the return
   struct → the generator references it in the generated receiver code.
2. If no converter exists → the generator creates one in `types_gen.go`.
3. Once created, the type definition may be hand-tuned (renamed fields,
   convenience methods, etc.) without being overwritten by subsequent runs.

This prevents duplication — many methods across different receivers return
`host.Result`, but the Starlark type for it exists exactly once.

> **Banned pattern.** `FromStringDict` with `NewBuiltin` values for namespace
> receivers is prohibited. Data-only result structs now also use
> `Attr()`/`AttrNames()` receivers instead of `FromStringDict`.

Currently, result structs are scattered as inline `FromStringDict` calls:
- `resultToStarlark()` in `bindings.go` (converts `host.Result`)
- Inline in `receiver_git.go`, `receiver_docker.go`, `receiver_npm.go`

The generator consolidates this. Example for `host.Result`:

```go
// resultToStarlark converts host.Result to a Starlark struct.
// Generated from host.Result by gen.receiver.
func resultToStarlark(r host.Result) *starlarkstruct.Struct {
    return starlarkstruct.FromStringDict(starlark.String("result"), starlark.StringDict{
        "ok":     starlark.Bool(r.OK),
        "stdout": starlark.String(r.Stdout),
        "stderr": starlark.String(r.Stderr),
        "code":   starlark.MakeInt(r.Code),
    })
}
```

The generator inspects the Go struct's fields (via `go.structs()`) and
applies Gate 1 recursively — every field must map to a Starlark type.
Structs with unmappable fields → generator error.

| Go Struct Field Type | Starlark Struct Field |
|---|---|
| `string` | `starlark.String` |
| `bool` | `starlark.Bool` |
| `int`, `int64` | `starlark.MakeInt` |
| `[]string` | `starlark.NewList(...)` |
| _(unmapped)_ | **generator error** |

## Requirements

### Prerequisite: Extend `go.methods()` and `go.funcs()` with Parameter Info

**File**: `noblefactor-ops/internal/starlark/receiver_go.go`

Add `params` field to `goMethods()` and `goFuncs()`. Extract `fn.Type.Params.List` — each
`*ast.Field` has `Names []*ast.Ident` and `Type ast.Expr`.

Each param struct contains:
- `name`: parameter name (empty for unnamed params)
- `type`: type string from `typeToString()`
- `variadic`: bool, true for `...T` params

Extend `typeToString()` with missing AST types:

| AST Type | Output | Example |
|---|---|---|
| `*ast.Ellipsis` | `"...Elt"` | `...string` |
| `*ast.InterfaceType` (empty) | `"any"` | `interface{}` |
| `*ast.FuncType` | `"func(...)"` | callback params |
| `*ast.ChanType` | `"chan T"` | channel params |
| `*ast.IndexExpr` | `"X[Index]"` | generic types |

### Code Generation: `go.generate()` Method

Add a new method to GoReceiver: `go.generate(template, descriptor) -> string`

- `template`: one of `"planned_receiver"`, `"graph_ops"`, `"immediate_receiver"`
- `descriptor`: Starlark dict with analyzed method info (built by the .star command)
- Returns: generated Go source code as a string

Templates live as raw string constants in a new file:
`noblefactor-ops/internal/starlark/receiver_go_gen.go`

#### Type Mapping

| Go Type | Starlark (immediate) | Slot (plan) | Slot Reader (ops) |
|---|---|---|---|
| `string` | `starlark.String` | `FillSlot()` | `node.GetSlot("x")` |
| `bool` | `starlark.Bool` | `FillSlot()` | `node.GetSlot("x") == "true"` |
| `int` / `int64` | `starlark.Int` | `FillSlot()` | `strconv.Atoi(node.GetSlot("x"))` |
| `[]string` | `*starlark.List` | `FillSlot()` | `strings.Split(node.GetSlot("x"), ",")` |
| `...string` (variadic) | positional `*args` | join → single slot | `strings.Split()` |
| Go struct (return only) | Generated Starlark struct (`types_gen.go`) | — | — |
| _(unmapped type)_ | **generator error** | — | — |

#### Template 1: Planned Receiver

Each method: unpacks Starlark args, creates `*execution.Node` with a single
`Operation` derived from the struct/method name, fills input slots from args,
appends node to graph, returns promise. When the Starlark script chains
operations (e.g., passes a promise as an arg), the planned receiver creates
an edge between the upstream and downstream nodes.

Reference: `devlore-cli/internal/starlark/plan_file.go`

#### Template 2: Graph Operations

Each operation: implements the appropriate `Operation` interface (`Transform`,
`Writer`, or `Direct`) based on its category. Reads input slots, handles
`DryRun`, calls backing implementation, returns `(value, error)`. The value
flows through the output edge; the error is processed by the executor
according to the node's `ErrorAction`. Registration function returns all ops.

Reference: `devlore-cli/internal/execution/ops_package.go`

#### Template 3: Immediate Receiver

Each method: embeds `Receiver` base, `Attr()` switch dispatches to methods that unpack
args, call backing impl, convert result. Reference: `devlore-cli/internal/starlark/receiver_git.go`

### Naming Conventions

| Concept | Pattern | Example |
|---|---|---|
| Planned receiver struct | `XxxPlan` | `FilePlan`, `GitPlan` |
| Generated planned receiver file | `plan_xxx_gen.go` | `plan_file_gen.go` |
| Op struct | `XxxYyyOp` | `FileCopyOp` |
| Op name | `"category.method"` | `"file.copy"` |
| Generated ops file | `ops_xxx_gen.go` | `ops_file_gen.go` |
| Ops registration | `XxxOps() []Operation` | `FileOps()` |
| Immediate receiver struct | `XxxReceiver` | `GitReceiver` |
| Generated immediate receiver file | `receiver_xxx_gen.go` | `receiver_git_gen.go` |
| Starlark method | `snake_case(GoMethod)` | `ConfigGet` → `config_get` |
| Slot name | `snake_case(param)` | `SourceRoot` → `source_root` |
| Node ID | `category.name-N` | `file.copy-1` |

### Skip List

Methods excluded automatically: `String`, `Type`, `Freeze`, `Truth`, `Hash`, `Attr`,
`AttrNames` (starlark.Value / starlark.HasAttrs interfaces).

### Extension Structure

```
noblefactor-ops/star/extensions/com.noblefactor.star.GenReceiver/
  extension.yaml
  commands/
    gen-receiver.star
```

The `.star` command orchestrates:
1. `go.methods(path, receiver_type=struct_name)` → get methods with params
2. Filter: public methods only, optional `--methods` inclusion list
3. Normalize names: struct → category (snake_case), methods → operations (snake_case)
4. Build descriptor dict with method info, namespace, type mappings
5. `go.generate("planned_receiver", descriptor)` → write to `planned_xxx_gen.go`
6. `go.generate("graph_ops", descriptor)` → write to `ops_xxx_gen.go`
7. `go.generate("immediate_receiver", descriptor)` → write to `immediate_xxx_gen.go`

## Execution Model Changes

### `Node.Operation` (singular)

The `Node` struct changes from:

```go
Operations []string `json:"operations" yaml:"operations"`
```

to:

```go
Operation string `json:"operation" yaml:"operation"`
```

The executor no longer loops over operations. Each node runs one operation.
Error handling is per-node via `ErrorAction`:

```go
func (e *GraphExecutor) executeNode(ctx *Context, node Executable) *Result {
    op, ok := e.registry.Get(node.GetOperation())
    if !ok {
        return &Result{Status: ResultFailed, Error: ...}
    }

    var value any
    var err error
    switch typed := op.(type) {
    case Transform:
        value, err = typed.Transform(ctx, node, content)
    case Writer:
        value, err = typed.Write(ctx, node, content)
    case Direct:
        err = typed.Execute(ctx, node)
    }

    if err != nil {
        switch node.GetErrorAction() {
        case ErrorActionStop:
            return &Result{Status: ResultFailed, Error: err}
        case ErrorActionContinue:
            log(err)
            return &Result{Status: ResultFailed, Error: err}
        case ErrorActionSilent:
            return &Result{Status: ResultFailed, Error: err}
        }
    }
    // value flows through output edge to downstream nodes
}
```

### Chained operations become node chains

A planned receiver that previously created:

```go
node := &execution.Node{
    Operation: "render",  // old: Operations: []string{"render", "copy"}
}
```

Now creates two nodes with an edge:

```go
renderNode := &execution.Node{ID: "file.render-1", Operation: "render", ...}
copyNode   := &execution.Node{ID: "file.copy-1",   Operation: "copy", ...}
// copyNode's "content" slot is a promise referencing renderNode's output
// Edge: renderNode → copyNode
```

## Implementation Phases

### Phase 0: Single-Operation Node Migration (devlore-cli)

- [ ] Change `Node.Operations []string` to `Node.Operation string`
- [ ] Update `Executable` interface: `GetOperations()` → `GetOperation()`
- [ ] Update executor to remove pipeline loop
- [ ] Add `ErrorAction` field to `Node`
- [ ] Replace `ConflictPolicy` on `ExecutorOptions` with per-node `ErrorAction`
- [ ] Update executor error handling: check failing node's `ErrorAction`, skip dependent downstream nodes on `Continue`/`Silent`
- [ ] Update lore planned receivers to emit node chains for multi-step workflows
- [ ] Update writ graph builder to emit node chains instead of operation lists
- [ ] Update writ reconciler to emit node chains
- [ ] Update stateview and history to use singular `Operation`
- [ ] Consolidate scattered result converters into `types.go` using Attr/AttrNames receivers
- [ ] Update all existing tests

The only multi-op patterns in the codebase are `["render", "copy"]` and
`["decrypt", "render", "copy"]` — the writ template expansion pipeline.
These become node chains: `render → copy` or `decrypt → render → copy`,
with edges carrying content between them.

**Files**:

| File | Action |
|---|---|
| `internal/execution/graph.go` | Modify (`Operation` singular, `GetOperation()`, add `ErrorAction`) |
| `internal/execution/executor.go` | Simplify (no pipeline loop), replace `ConflictPolicy` with per-node `ErrorAction` |
| `internal/execution/operation.go` | No change (interfaces unchanged) |
| `internal/starlark/types.go` | Create (consolidate `resultToStarlark` and other struct converters) |
| `internal/execution/stateview.go` | Modify (singular `Operation`) |
| `internal/execution/plan.go` | Modify (singular `Operation`) |
| `internal/starlark/plan_file.go` | Modify (emit node chains for `configure`) |
| `internal/starlark/plan.go` | Modify (singular, chain for `configure`) |
| `internal/starlark/platform/darwin.go` | Modify (emit node chains) |
| `internal/starlark/platform/linux.go` | Modify (emit node chains) |
| `internal/starlark/platform/windows.go` | Modify (emit node chains) |
| `internal/starlark/platform/common.go` | Modify (singular `Operation`) |
| `internal/writ/graph_builder.go` | Modify (emit node chains) |
| `internal/writ/reconcile/reconcile.go` | Modify (emit node chains) |
| `internal/writ/tree/builder.go` | Modify (singular `Operation`) |
| `internal/writ/tree/operation.go` | Modify (singular `Operation`) |
| `internal/writ/commands.go` | Modify (singular `Operation`) |
| `internal/writ/migrate/format.go` | Modify (singular `Operation`) |
| `internal/execution/*_test.go` | Update |
| `internal/writ/*_test.go` | Update |
| `internal/lore/*_test.go` | Update |
| `docs/architecture/*.md` | Update |

### Phase 1: Extend GoReceiver AST Primitives (noblefactor-ops)

- [ ] Add `params` to `goMethods()` and `goFuncs()`
- [ ] Extend `typeToString()` with missing AST types
- [ ] Add param extraction tests

**Files**:

| File | Action |
|---|---|
| `internal/starlark/receiver_go.go` | Modify |
| `internal/starlark/receiver_go_ast_test.go` | Modify |

### Phase 2: Add `go.generate()` with Templates (noblefactor-ops)

- [ ] Create template constants for all three generated receiver types + struct type template
- [ ] Implement `go.generate()` method with type mapping and name normalization
- [ ] Implement Gate 1 (all types must map) and Gate 2 (`(T, error)` return) validation
- [ ] Generate Starlark struct types for Go struct return values into `types_gen.go`
- [ ] Add `"generate"` to `Attr()`/`AttrNames()`
- [ ] Test each template against known patterns
- [ ] Test gate enforcement: verify generator errors on unmapped types and invalid return shapes

**Files**:

| File | Action |
|---|---|
| `internal/starlark/receiver_go_gen.go` | Create (templates + generate method + gate validation) |
| `internal/starlark/receiver_go_gen_test.go` | Create (template tests + gate enforcement tests) |
| `internal/starlark/receiver_go.go` | Modify |

### Phase 3: Create the Star Extension (noblefactor-ops)

- [ ] Create `extension.yaml`
- [ ] Create `gen-receiver.star` command
- [ ] Test end-to-end generation

**Files**:

| File | Action |
|---|---|
| `star/extensions/com.noblefactor.star.GenReceiver/extension.yaml` | Create |
| `star/extensions/com.noblefactor.star.GenReceiver/commands/gen-receiver.star` | Create |

### Phase 4: Validate Against Existing Hand-Written Code

- [ ] Run `star gen.receiver` against devlore-cli's `FilePlan` → diff with `plan_file.go`
- [ ] Run against `PackageInstallOp` → diff with `ops_package.go`
- [ ] Run against `GitReceiver` → diff with `receiver_git.go`
- [ ] Adjust templates until generated output matches hand-written patterns

## Integration

Generated files need one manual line each to wire into the system:

- **Planned receivers**: Add field + case in `PlanRoot.Attr()` (`plan_root.go`)
- **Graph operations**: Add `XxxOps()` call in `AllOps()` (`ops.go`)
- **Immediate receivers**: Add `NewXxxReceiver()` in `Bindings.Globals()` (`bindings.go`)

This is intentional — the generator eliminates the method-by-method boilerplate,
while the one-line registration serves as an explicit manifest.

## Verification

1. `cd noblefactor-ops && go test ./internal/starlark/...` — all tests pass
2. `go build -o /tmp/star-test ./cmd/star` — compiles
3. `star gen.receiver --path <path> --struct FilePlan --namespace plan.file --output /tmp/gen` — generates valid Go
4. `gofmt -e /tmp/gen/plan_file_gen.go` — no syntax errors
5. Diff generated vs hand-written — structure matches

### Phase 5: Decompose starlarkcode into Per-Domain Packages (devlore-cli + noblefactor-ops)

The monolithic `starlarkcode` provider bundles capture, indexing, stats, complexity, and analysis into one package. Every other provider gets its own package under `pkg/op/provider/`. This phase decomposes `starlarkcode/` into 6 focused packages and renames `starlarkcode` to `starcode` for brevity.

The chained Starlark API (`starcode.capture(...).index(...)`) is preserved. Leaf packages contain Go domain types and logic. `starsources` has thin delegation methods that the generator wraps for Starlark consumption.

#### Target Layout

```
pkg/op/provider/
    starcode/
        provider.go              ← Provider{Root} + Capture → *starsources.Sources
        gen/immediate.gen.go     ← StarcodeReceiver + init()

    starsources/
        provider.go              ← Sources{Root, Files} + Paths, Count + delegation
        gen/sources.gen.go       ← SourcesValue wrapper (imports leaf gen/ converters)

    starindex/
        provider.go              ← types + IndexFiles()

    starstats/
        provider.go              ← types + ComputeStats()

    starcomplexity/
        provider.go              ← types + ComputeComplexity()

    staranalysis/
        provider.go              ← types + Analyze() (imports starindex, starstats, starcomplexity)
```

#### Dependency Graph

```
starcode → starsources
starsources → starindex, starstats, staranalysis
staranalysis → starindex, starstats, starcomplexity
```

No cycles. Leaf packages (starindex, starstats, starcomplexity) have zero intra-module dependencies.

#### New Files (devlore-cli)

| File | Content |
|---|---|
| `pkg/op/provider/starcode/provider.go` | `Provider{Root}` + `Capture()` + private helpers. Returns `*starsources.Sources`. Directives: `+devlore:access=immediate`, `+devlore:bind Root=WorkDir` |
| `pkg/op/provider/starsources/provider.go` | `Sources{Root, Files}` + `Paths()`, `Count()` + delegation: `Index()` → `starindex.IndexFiles()`, `Stats()` → `starstats.ComputeStats()`, `Analyze()` → `staranalysis.Analyze()`. Same `+devlore:defaults` and `+devlore:struct_param` directives as current |
| `pkg/op/provider/starindex/provider.go` | Types: Index, IndexedFile, IndexTotals, IndexedFunction, IndexedLoad, IndexedGlobal. Function: `IndexFiles(root, files, withDocstrings, withGlobals)`. Private helpers: indexFile, indexStmts, extractDocstring, extractAssignName. No external imports |
| `pkg/op/provider/starstats/provider.go` | Types: Stats, FileStats, StatsTotals. Function: `ComputeStats(root, files, withBytes, withLOC)`. Private helpers: countLines. No external imports |
| `pkg/op/provider/starcomplexity/provider.go` | Types: ComplexityReport, FileComplexity, FunctionComplexity, complexityWalker. Function: `ComputeComplexity(root, files)`. Private helpers: analyzeFileComplexity, countFunctionLOC, walker methods. No external imports |
| `pkg/op/provider/staranalysis/provider.go` | Types: AnalysisReport, AnalysisConfig, Hotspot. Function: `Analyze(root, files, cfg)`. Imports: starindex, starstats, starcomplexity. `AnalysisReport.Stats` is `*starstats.Stats`, `.Complexity` is `*starcomplexity.ComplexityReport`, `.Index` is `*starindex.Index` |

#### Delete (devlore-cli)

| Directory | Reason |
|---|---|
| `pkg/op/provider/starlarkcode/` | All code moved to 6 new packages above |

#### Generator Changes (noblefactor-ops)

Three new generator capabilities in `internal/starlark/codegen.go` and `star/.../commands/generate.star`:

1. **Converter-only mode** (`--converters-only=true`): For leaf packages with no Provider/receiver. Runs `go.structs()`, builds converter descriptors. No receiver, no init(), no methods. Struct conversion is handled by `op.Marshal` via reflection.

2. **Exported converter names**: Leaf converters must be PascalCase (`IndexToStarlark`) since `starsources/gen` calls them cross-package. Add `exported: true` flag to converter descriptors.

3. **Cross-package converter imports**: `starsources/gen/sources.gen.go` imports leaf gen packages:
   - `starindexgen "...starindex/gen"` for `starindexgen.IndexToStarlark()`
   - `starstatsgen "...starstats/gen"` for `starstatsgen.StatsToStarlark()`
   - `staranalysisgen "...staranalysis/gen"` for `staranalysisgen.AnalysisReportToStarlark()`

| File | Action |
|---|---|
| `internal/starlark/codegen.go` | Modify: converter-only template path, exported converter names, cross-package import aliases |
| `star/.../commands/generate.star` | Modify: `--converters-only` flag, exported converter naming, cross-package import collection |

#### Generation Commands

```bash
# Leaf packages: converter-only mode (order: leaves first)
star devlore actions generate --source=pkg/op/provider/starindex --gen=true --converters-only=true
star devlore actions generate --source=pkg/op/provider/starstats --gen=true --converters-only=true
star devlore actions generate --source=pkg/op/provider/starcomplexity --gen=true --converters-only=true
star devlore actions generate --source=pkg/op/provider/staranalysis --gen=true --converters-only=true

# Provider packages: full receiver + converter generation
star devlore actions generate --source=pkg/op/provider/starcode --gen=true
star devlore actions generate --source=pkg/op/provider/starsources --gen=true
```

#### Update Existing Files (devlore-cli)

| File | Change |
|---|---|
| `internal/starlark/integration_test.go` | Side-effect import: `_ "...starlarkcode/gen"` → `_ "...starcode/gen"` |
| `internal/starlark/testdata/load_test.star` | `load("@devlore//starlarkcode", ...)` → `load("@devlore//starcode", ...)` |
| `pkg/op/provider/register.go` | Side-effect import: `_ "...starlarkcode/gen"` → `_ "...starcode/gen"` |

#### Verification

- [ ] `make build` — both repos
- [ ] `make test` — both repos; integration test exercises full capture → index/stats/analyze chain
- [ ] `make vet` — both repos
- [ ] Grep for `starlarkcode` — no matches outside deletion candidates
- [ ] Verify old `starlarkcode/` directory deleted
- [ ] Verify chained Starlark API: `starcode.capture(...).index(...)` etc.

## Related Documents

- `noblefactor-ops/docs/architecture/star-file-tree-walking.md` — Prior receiver work
- `devlore-cli/internal/starlark/receiver_test.go` — Dict-based receiver ban enforcement
