# Phase 1: Extend GoReceiver AST Primitives

## Context

Phase 0 (single-operation nodes) is merged in devlore-cli (PR #97). Phase 1 extends
the GoReceiver in noblefactor-ops so `go.methods()` and `go.funcs()` return parameter
information. This is a prerequisite for Phase 2 (code generation), which needs method
signatures to generate planned receivers, graph operations, and immediate receivers.

**Repo**: noblefactor-ops
**Files**:
- `internal/starlark/receiver_go.go` -- all production changes
- `internal/starlark/receiver_go_ast_test.go` -- all test additions

## Step 1: Extend `typeToString()` (line 1063)

Add 6 missing AST type cases before the `default` fallback:

| AST Type | Output | Example |
|---|---|---|
| `*ast.Ellipsis` | `"..." + Elt` | `...string` |
| `*ast.InterfaceType` (empty methods) | `"any"` | `interface{}` |
| `*ast.FuncType` | `"func(...) ..."` | `func(string) error` |
| `*ast.ChanType` | `"chan T"` / `"chan<- T"` / `"<-chan T"` | direction via `t.Dir` bitmask |
| `*ast.IndexExpr` | `"X[Index]"` | `Result[string]` |
| `*ast.IndexListExpr` | `"X[I1, I2]"` | `Map[K, V]` |

`FuncType` uses existing `returnTypeString()` (line 115) for the result portion.
`ChanType` uses `ast.SEND` (1), `ast.RECV` (2); bidirectional is `default` case.
No new imports needed -- `go/ast` and `strings` already imported.

## Step 2: Add `extractParams()` helper (after `typeToString()`)

```go
func extractParams(params *ast.FieldList) starlark.Value
```

Iterates `params.List`, producing a Starlark list of param structs with fields:
- `name` (string) -- parameter name, empty for unnamed params
- `type` (string) -- type from `typeToString()`, with `"..."` prefix stripped for variadics
- `variadic` (bool) -- true when last field's type is `*ast.Ellipsis`

**Multi-name fields**: `a, b int` produces two separate param entries (one per name).
**Variadic detection**: `field.Type.(*ast.Ellipsis)` on the last field.
**Type for variadics**: Strip `"..."` prefix from `typeToString()` output since
`variadic: true` carries that information separately.

## Step 3: Wire `params` into `goMethods()`

Add one field to the StringDict:

```go
"params": extractParams(fn.Type.Params),
```

## Step 4: Wire `params` into `goFuncs()`

Same one-line addition:

```go
"params": extractParams(fn.Type.Params),
```

## Step 5: Tests

### `getBoolAttr` test helper
Added to test helpers section -- mirrors `getStructAttr` but returns `bool`.

### `TestGoFuncsParams`
Fixture with 12 functions covering all parameter patterns:
- `singleParam(x string)` -- basic case
- `multiSameType(a, b int)` -- multi-name field expansion
- `multiDiffType(name string, count int, verbose bool)` -- mixed types
- `variadicFunc(prefix string, items ...string)` -- variadic detection + type stripping
- `pointerParam(node *Node)` -- pointer type
- `sliceParam(items []string)` -- slice type
- `mapParam(data map[string]int)` -- map type
- `noParams()` -- empty params list
- `interfaceParam(v interface{})` -- empty interface as `"any"`
- `funcParam(fn func(string) error)` -- function-typed param
- `chanParam(ch chan string)` -- channel type
- `genericParam(r Result[string])` -- generic instantiation

Each subtest: call `go.funcs(dir, name=X)`, extract `params` list, validate
name/type/variadic on each param entry.

### `TestGoMethodsParams`
Fixture with `FileOps` struct and 3 methods:
- `Copy(source, dest string)` -- multi-name field on method
- `Install(packages ...string)` -- variadic on method
- `Check()` -- no params

Confirms `go.methods()` wiring works (shared `extractParams` logic).

### `TestTypeToStringExtended`
Fixture with `TypeShowcase` struct whose fields exercise all new AST types.
Uses `go.structs()` to extract field types, then validates each against expected
string representation. Covers: `any`, `func(string) error`, `chan string`,
`chan<- int`, `<-chan bool`, `Result[string]`.

## Verification

```bash
cd /path/to/noblefactor-ops

# Existing tests still pass (no regressions)
go test ./internal/starlark/ -run TestGo -count=1

# New tests pass
go test ./internal/starlark/ -run "TestGoFuncsParams|TestGoMethodsParams|TestTypeToString" -count=1

# Full package
go test ./internal/starlark/ -count=1

# Build + vet
go build ./... && go vet ./internal/starlark/
```
