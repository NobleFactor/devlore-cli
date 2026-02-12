---
title: "Star Extension for Generating Receivers and Graph Operations"
status: draft
created: 2026-02-12
updated: 2026-02-12
---

# Plan: Star Extension for Generating Receivers and Graph Operations

## Summary

Every plan receiver (`plan_file.go`, `plan_git.go`), graph operation (`ops_package.go`),
and real-time receiver (`receiver_git.go`) follows an identical mechanical pattern.
Writing these by hand is tedious and error-prone. This plan creates a star extension
(`com.noblefactor.star.GenReceiver`) that reads a Go implementation struct via the
existing `go.methods()` / `go.structs()` AST primitives, then generates the three kinds
of boilerplate. Input: path to a Go package + struct name. Output: generated `_gen.go`
files ready to compile.

## Goals

1. **Eliminate hand-written boilerplate**: Generate plan receivers, graph operations,
   and real-time receivers from Go struct definitions
2. **Enforce the receiver pattern**: All generated code uses `Receiver` base type with
   `Attr`/`AttrNames` — never dict-based `FromStringDict` namespaces
3. **Extend AST primitives**: Add parameter info to `go.methods()` and `go.funcs()` so
   the generator knows method signatures

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `go.methods()` / `go.funcs()` | Partial | Returns name, receiver, returns, doc, scope — no params |
| `typeToString()` | Partial | Missing Ellipsis, InterfaceType, FuncType, ChanType, IndexExpr |
| Plan receivers | Manual | `plan_file.go`, `plan_git.go` etc. written by hand |
| Graph operations | Manual | `ops_package.go` etc. written by hand |
| Real-time receivers | Manual | `receiver_git.go` etc. written by hand |
| Dict-based receiver ban | Enforced | `TestNoDictBasedReceivers` catches violations in CI |

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

- `template`: one of `"plan_receiver"`, `"graph_ops"`, `"realtime_receiver"`
- `descriptor`: Starlark dict with analyzed method info (built by the .star command)
- Returns: generated Go source code as a string

Templates live as raw string constants in a new file:
`noblefactor-ops/internal/starlark/receiver_go_gen.go`

#### Type Mapping

| Go Type | Starlark (real-time) | Slot (plan) | Slot Reader (ops) |
|---|---|---|---|
| `string` | `starlark.String` | `FillSlot()` | `node.GetSlot("x")` |
| `bool` | `starlark.Bool` | `FillSlot()` | `node.GetSlot("x") == "true"` |
| `int` / `int64` | `starlark.Int` | `FillSlot()` | `strconv.Atoi(node.GetSlot("x"))` |
| `[]string` | `*starlark.List` | `FillSlot()` | `strings.Split(node.GetSlot("x"), ",")` |
| `...string` (variadic) | positional `*args` | join -> single slot | `strings.Split()` |
| complex/interface | **skip** — emit TODO | — | — |

#### Template 1: Plan Receiver

Each method: unpacks Starlark args, creates `*execution.Node`, fills slots, appends
to graph, returns `NewOutput()`. Reference: `devlore-cli/internal/starlark/plan_file.go`

#### Template 2: Graph Operations

Each operation: implements `Operation` interface, reads slots, handles `DryRun`,
emits TODO placeholder for backing implementation call. Registration function returns
all ops. Reference: `devlore-cli/internal/execution/ops_package.go`

#### Template 3: Real-time Receiver

Each method: embeds `Receiver` base, `Attr()` switch dispatches to methods that unpack
args, call backing impl, convert result. Reference: `devlore-cli/internal/starlark/receiver_git.go`

### Naming Conventions

| Concept | Pattern | Example |
|---|---|---|
| Plan receiver struct | `XxxPlan` | `FilePlan`, `GitPlan` |
| Generated plan file | `plan_xxx_gen.go` | `plan_file_gen.go` |
| Op struct | `XxxYyyOp` | `FileConfigureOp` |
| Op name | `"namespace-method"` | `"file-copy"` |
| Generated ops file | `ops_xxx_gen.go` | `ops_file_gen.go` |
| Real-time struct | `XxxReceiver` | `GitReceiver` |
| Generated receiver file | `receiver_xxx_gen.go` | `receiver_git_gen.go` |
| Starlark method | `snake_case(GoMethod)` | `ConfigGet` -> `config_get` |

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
1. `go.methods(path, receiver_type=struct_name)` -> get methods with params
2. Filter: public methods only, optional `--methods` inclusion list
3. Build descriptor dict with method info, namespace, type mappings
4. `go.generate("plan_receiver", descriptor)` -> write to `plan_xxx_gen.go`
5. `go.generate("graph_ops", descriptor)` -> write to `ops_xxx_gen.go`
6. `go.generate("realtime_receiver", descriptor)` -> write to `receiver_xxx_gen.go`

## Implementation Phases

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

- [ ] Create template constants for all three receiver types
- [ ] Implement `go.generate()` method with type mapping
- [ ] Add `"generate"` to `Attr()`/`AttrNames()`
- [ ] Test each template against known patterns

**Files**:

| File | Action |
|---|---|
| `internal/starlark/receiver_go_gen.go` | Create |
| `internal/starlark/receiver_go_gen_test.go` | Create |
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

- [ ] Run `star gen.receiver` against devlore-cli's `FilePlan` -> diff with `plan_file.go`
- [ ] Run against `PackageInstallOp` -> diff with `ops_package.go`
- [ ] Run against `GitReceiver` -> diff with `receiver_git.go`
- [ ] Adjust templates until generated output matches hand-written patterns

## Integration

Generated files need one manual line each to wire into the system:

- **Plan receivers**: Add field + case in `PlanRoot.Attr()` (`plan_root.go`)
- **Graph operations**: Add `XxxOps()` call in `AllOps()` (`ops.go`)
- **Real-time receivers**: Add `NewXxxReceiver()` in `Bindings.Globals()` (`bindings.go`)

This is intentional — the generator eliminates the method-by-method boilerplate,
while the one-line registration serves as an explicit manifest.

## Verification

1. `cd noblefactor-ops && go test ./internal/starlark/...` — all tests pass
2. `go build -o /tmp/star-test ./cmd/star` — compiles
3. `star gen.receiver --path <path> --struct FilePlan --namespace plan.file --output /tmp/gen` — generates valid Go
4. `gofmt -e /tmp/gen/plan_file_gen.go` — no syntax errors
5. Diff generated vs hand-written — structure matches

## Related Documents

- `noblefactor-ops/docs/architecture/star-file-tree-walking.md` — Prior receiver work
- `devlore-cli/internal/starlark/receiver_test.go` — Dict-based receiver ban enforcement
