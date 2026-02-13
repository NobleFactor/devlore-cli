# Phase 0: Single-Operation Node Migration

## Context

The plan document at `docs/plans/star-gen-receiver.md` defines a Graph Execution Model
where each node performs exactly one operation. This is a prerequisite for code generation
-- the generator needs a 1:1 mapping between implementation methods and graph nodes.

Currently, nodes support multi-op pipelines (e.g., `Operations: ["render", "copy"]`).
This prevents 1:1 mapping and bakes implicit ordering into a single node. The migration
changes `Node.Operations []string` to `Node.Operation string`. Multi-op pipelines become
node chains connected by edges, with content flowing between them.

**Scope**: Structural migration only. ErrorAction (replacing ConflictResolution) is a
separate follow-up PR -- it's an orthogonal concern and keeping it separate makes each
PR reviewable.

## Step 1: Core Struct Changes (`internal/execution/graph.go`)

| Change | Before | After |
|---|---|---|
| Node field | `Operations []string` | `Operation string` |
| JSON/YAML tag | `operations` | `operation` |
| Executable interface | `GetOperations() []string` | `GetOperation() string` |
| Node method | `GetOperations() []string` | `GetOperation() string` |
| ComputeSummary | `n.Operations[0]` | `n.Operation` |
| Graph version constant | (lives in graph_builder.go) | bump `"5"` → `"6"` |

Also update `CanonicalContent()` if it references Operations.

## Step 2: Executor — Remove Pipeline Loop, Add Content Flow (`internal/execution/executor.go`)

**Current** (`executeExecutable`, line 188): iterates `node.GetOperations()` in a pipeline
loop, accumulating `content []byte` across transforms.

**New**: Single operation dispatch per node. Content flows between chained nodes via an
`outputs map[string][]byte` maintained by the executor during `Run()`.

```
Run():
  outputs := map[string][]byte{}
  for _, node := range ordered:
    result := executeNode(ctx, node, g.Edges, outputs)
    // ...

executeNode(ctx, node, edges, outputs):
  op := registry.Get(node.Operation)

  // Resolve input content
  var content []byte
  if upstream := findContentUpstream(node.ID, edges, outputs); upstream != nil:
    content = upstream
  else if needsContent(op):
    content = readFile(node.GetSlot("source"))

  // Single dispatch
  switch op.(type):
    Transform: outputs[node.ID] = op.Transform(ctx, node, content)
    Writer:    op.Write(ctx, node, content)
    Direct:    op.Execute(ctx, node)
```

**`findContentUpstream`**: For a given node, walk incoming edges. If any upstream node
has stored content in the outputs map, return it. This is how render→copy chains work.

**`needsContent`**: Simplified — just check `op.Category() != OpDirect`.

**`RunNodes`**: Same pattern — accept edges, maintain outputs map.

## Step 3: Plan API (`internal/execution/plan.go`)

All single-op methods change from `Operations: []string{"foo"}` to `Operation: "foo"`:
- `Mkdir`, `Link`, `Remove`, `Unlink`, `Backup`, `Validate`, `Rename`

**`Copy(source, path, transforms ...string)`** — creates a node chain when
transforms are provided:

```go
func (p *Plan) Copy(source, path string, transforms ...string) *Node {
    if len(transforms) == 0 {
        // Simple copy — single node
        node := &Node{ID: p.nextID("copy"), Operation: "copy", ...}
        node.SetSlotImmediate("source", source)
        node.SetSlotImmediate("path", path)
        p.graph.Nodes = append(p.graph.Nodes, node)
        return node
    }

    // Chain: transform1 → transform2 → ... → copy
    allOps := append(transforms, "copy")
    var prevNode *Node
    var lastNode *Node
    for i, op := range allOps {
        isLast := (i == len(allOps)-1)
        node := &Node{ID: p.nextID(op), Operation: op, ...}
        if i == 0 {
            node.SetSlotImmediate("source", source)
        }
        if isLast {
            node.SetSlotImmediate("path", path)
        }
        p.graph.Nodes = append(p.graph.Nodes, node)
        if prevNode != nil {
            p.graph.Edges = append(p.graph.Edges, Edge{From: prevNode.ID, To: node.ID})
        }
        prevNode = node
        lastNode = node
    }
    return lastNode  // Return final node for DependsOn
}
```

Same pattern for `CopyWithMode`.

## Step 4: StateView (`internal/execution/stateview.go`)

| Change | Before | After |
|---|---|---|
| `HistoryRecord.Operations` | `[]string` | `Operation string` |
| `IsCopied()` | iterates ops for render/decrypt/copy | `op == "copy"` |
| `IsLinked()` | `len(ops) == 1 && ops[0] == "link"` | `op == "link"` |
| `FileEntry.Operations()` | returns `[]string` | returns `string` (rename to `LastOp()`) |
| `processGraph` | `record.Operations = node.Operations` | `record.Operation = node.Operation` |
| `isPackageNode` | iterates operations | check `node.Operation` |

**Skip transform nodes in processGraph**: Nodes with operations like "render" or "decrypt"
are intermediate transform nodes — they don't represent file entries. Add a guard:

```go
func isTransformOnlyNode(node *Node) bool {
    switch node.Operation {
    case "render", "decrypt":
        return true
    }
    return false
}

// In processGraph:
if node.Status == StatusSkipped || isTransformOnlyNode(node) {
    continue
}
```

**Decision**: Keep it simple. `IsCopied` returns true for `copy`, `IsLinked` for `link`.
The `ComputeSummary` in graph.go counts "render" and "decrypt" nodes as templates/secrets.

## Step 5: Preflight (`internal/execution/preflight.go`)

`nodeWritesToTarget`: change from iterating `node.Operations` to checking `node.Operation`:

```go
func nodeWritesToTarget(node *Node) bool {
    if node.GetSlot("path") == "" {
        return false
    }
    return node.Operation == "link" || node.Operation == "copy"
}
```

## Step 6: Starlark Plan Bindings

### `internal/starlark/plan_*.go`

All single-op methods change `Operations: []string{"foo"}` to `Operation: "foo"`:
- `plan_root.go`: source, literal, download, service, shell
- `plan_package.go`: install, upgrade, remove, update
- `plan_file.go`: link, copy, write, remove
- `plan_git.go`: clone, checkout, pull
- `plan_archive.go`: extract

`configure` methods create render→copy chains (render node + copy node + edge).

### `internal/starlark/platform/common.go`, `darwin.go`, `linux.go`, `windows.go`

All single-op methods updated. `Configure` methods create render+copy chains.

## Step 7: Writ Package

### `internal/writ/graph_builder.go`

**Version bump**: `CurrentVersion = "6"`

**`BuildTree`**: Decompose multi-op FileEntry pipelines into node chains. Single-op
entries produce one node. Multi-op entries (e.g., `["render", "copy"]`) produce a chain
of nodes connected by edges, with intermediate nodes having IDs like `fileID:opName`.

### `internal/writ/commands.go`

All node construction and references updated to single-op pattern.

### `internal/execution/builder.go`

`isDelegateNode` simplified to `node.Operation == "delegate"`.

## Step 8: Writ Subpackages

### `internal/writ/migrate/format.go`, `execute.go`, `session.go`

`nodeView.Operations []string` → `Operation string`. All iteration replaced with
direct `node.Operation` checks.

### `internal/writ/reconcile/reconcile.go`

`checkEntry` signature changed from `operations []string` to `operation string`.
`FromBuildResult` extracts final op from tree's `f.Operations[]` pipeline.

### `internal/lore/builder.go`

`Operation: opName` (was `Operations: []string{opName}`).

### `internal/manifest/builder.go`

Converted `buildPackageNode` (single node with 4 ops) to `buildPackageNodes` (chain of
4 single-op nodes: prepare → install → provision → verify).

### `internal/writ/tree/` — NO CHANGES

The tree package describes processing pipelines as metadata. `FileEntry.Operations []string`
stays as-is (input to graph building, not execution model).

## Step 9: Tests

All test files updated to use `Operation: "op"` instead of `Operations: []string{"op"}`.
Multi-op pipeline tests converted to chain tests (multiple nodes + edges).

Key test conversions:
- `TestEngineRunRenderCopyPipeline`: 2-node chain (render → copy)
- `TestEngineRunDecryptRenderCopyPipeline`: 3-node chain (decrypt → render → copy)
- `TestIsPackageNode`: single operation string tests
- `TestFileEntryIsCopied`/`IsLinked`: single operation tests
- `TestBuilder_BuildGraphFromManifest`: 3 packages × 4 phases = 12 nodes, 9 edges

## Content Flow Verification

| Pipeline | Chain | Content Flow |
|---|---|---|
| `["link"]` | single node | no content |
| `["copy"]` | single node | source → copy |
| `["render", "copy"]` | render → copy | source → render → outputs → copy → target |
| `["decrypt", "copy"]` | decrypt → copy | source → decrypt → outputs → copy → target |
| `["decrypt", "render", "copy"]` | decrypt → render → copy | source → decrypt → render → copy → target |

## Verification

1. `go build ./...` — compiles
2. `go test ./...` — all tests pass
3. `go vet ./...` — no issues
