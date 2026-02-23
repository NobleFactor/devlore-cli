# Phase 2: Add `go.generate()` with Templates

## Context

Phase 0 (single-operation nodes) is merged in devlore-cli (PR #97). Phase 1 (parameter
extraction) is merged in noblefactor-ops (PR #65). Phase 2 adds the `go.generate()`
method to GoReceiver that produces Go source code from analyzed method descriptors.

The generator reads method signatures (now available from Phase 1) and produces three
kinds of boilerplate: planned receivers, graph operations, and immediate receivers. Each
template follows the exact patterns already established by hand-written code in
devlore-cli.

**Repo**: noblefactor-ops
**New files**:
- `internal/starlark/receiver_go_gen.go` -- templates, generate method, gate validation
- `internal/starlark/receiver_go_gen_test.go` -- all tests

**Modified files**:
- `internal/starlark/receiver_go.go` -- wire `generate` into Attr/AttrNames

## Design Decisions

### Template approach: `text/template`

Templates use Go's `text/template` with a descriptor struct passed as data. The
descriptor is built by the caller (the .star command in Phase 3) and passed as a
Starlark dict. `go.generate()` converts the dict to a Go struct, validates gates,
and executes the template.

### Name normalization: `camelToSnake()`

A new helper converts Go names to snake_case for Starlark identifiers, slot names,
and operation names. Examples: `Copy` -> `copy`, `WalkTree` -> `walk_tree`,
`ConfigGet` -> `config_get`. This is internal to the generator -- no exported API.

### Gate enforcement happens in Go, not Starlark

The two gates (type mapping and return signature) are validated inside `go.generate()`
before template execution. This keeps the .star command simple -- it passes the raw
method descriptor and gets either generated code or a clear error.

## Step 1: Name normalization helper

Add `camelToSnake(s string) string` to `receiver_go_gen.go`:

```go
func camelToSnake(s string) string
```

Rules:
- Insert `_` before each uppercase letter that follows a lowercase letter
- Insert `_` between an uppercase run and the next uppercase+lowercase pair
  (`HTMLParser` -> `html_parser`, not `htmlparser`)
- Lowercase everything
- No leading/trailing underscores

| Input | Output |
|---|---|
| `Copy` | `copy` |
| `WalkTree` | `walk_tree` |
| `ConfigGet` | `config_get` |
| `HTMLParser` | `html_parser` |
| `ID` | `id` |

## Step 2: Type mapping

A `goTypeToStarlark` table maps Go type strings (from `typeToString()`) to their
Starlark unpacking and conversion patterns:

```go
type typeMapping struct {
    starlarkType string // e.g., "starlark.String"
    unpackType   string // e.g., "string" (for starlark.UnpackArgs)
    slotReader   string // e.g., "node.GetSlot(%q)" (for ops)
    converter    string // e.g., "starlark.String(%s)" (result -> Starlark)
}
```

| Go Type | UnpackArgs type | Starlark type | Slot reader |
|---|---|---|---|
| `string` | `string` | `starlark.String` | `node.GetSlot(%q)` |
| `bool` | `bool` | `starlark.Bool` | `node.GetSlot(%q) == "true"` |
| `int` | `int` | `starlark.MakeInt` | `strconv.Atoi(node.GetSlot(%q))` |
| `int64` | `int64` | `starlark.MakeInt64` | `strconv.ParseInt(node.GetSlot(%q), 10, 64)` |
| `[]string` | `*starlark.List` | `*starlark.List` | `strings.Split(node.GetSlot(%q), ",")` |

Gate 1 error when a type is not in this table.

## Step 3: Gate validation functions

### `validateReturnSignature(returns string) (valueType string, err error)`

Parses the return string from `go.methods()` / `go.funcs()`. Valid forms:

| Returns string | Value type | Error |
|---|---|---|
| `(string, error)` | `string` | nil |
| `(bool, error)` | `bool` | nil |
| `(int, error)` | `int` | nil |
| `([]string, error)` | `[]string` | nil |
| `error` | -- | "must return (T, error)" |
| `string` | -- | "must return (T, error)" |
| `(string, int, error)` | -- | "must return (T, error)" |
| `` (empty) | -- | "must return (T, error)" |

### `validateParamTypes(params []paramInfo) error`

Checks each param's type against the type mapping table. Returns error listing
all unmapped types (not just the first).

## Step 4: Descriptor struct

The `go.generate()` method receives a Starlark dict and converts it to:

```go
type generateDescriptor struct {
    Template    string       // "planned_receiver", "graph_ops", "immediate_receiver"
    Package     string       // Go package name for generated file
    Category    string       // snake_case struct name (e.g., "file")
    StructName  string       // Go struct name (e.g., "FileOps")
    Namespace   string       // dotted namespace (e.g., "plan.file")
    Methods     []methodInfo // analyzed methods
}

type methodInfo struct {
    GoName      string      // original Go name (e.g., "Copy")
    SnakeName   string      // snake_case name (e.g., "copy")
    Params      []paramInfo // from go.methods() params
    ReturnType  string      // value portion of (T, error)
    Doc         string      // doc comment
}

type paramInfo struct {
    GoName    string // original param name
    SnakeName string // snake_case name
    GoType    string // Go type string
    Variadic  bool
}
```

## Step 5: Template 1 -- Planned Receiver

Generates `plan_xxx_gen.go`. Pattern matches `plan_file.go`:

```
// Code generated by go.generate; DO NOT EDIT.

package starlark

import (
    "fmt"
    "go.starlark.net/starlark"
    "github.com/NobleFactor/devlore-cli/internal/execution"
    "github.com/NobleFactor/devlore-cli/internal/host"
)

type {{.StructName}}Plan struct {
    graph   *execution.Graph
    host    host.Host
    project string
}

func New{{.StructName}}Plan(graph *execution.Graph, h host.Host, project string) *{{.StructName}}Plan {
    return &{{.StructName}}Plan{graph: graph, host: h, project: project}
}

func (p *{{.StructName}}Plan) String() string        { return "{{.Namespace}}" }
func (p *{{.StructName}}Plan) Type() string          { return "{{.Namespace}}" }
func (p *{{.StructName}}Plan) Freeze()               {}
func (p *{{.StructName}}Plan) Truth() starlark.Bool  { return true }
func (p *{{.StructName}}Plan) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: {{.Namespace}}") }

func (p *{{.StructName}}Plan) Attr(name string) (starlark.Value, error) {
    switch name {
    {{range .Methods -}}
    case "{{.SnakeName}}":
        return starlark.NewBuiltin("{{$.Namespace}}.{{.SnakeName}}", p.{{.SnakeName}}), nil
    {{end -}}
    default:
        return nil, starlark.NoSuchAttrError(fmt.Sprintf("{{.Namespace}} has no attribute %q", name))
    }
}

func (p *{{.StructName}}Plan) AttrNames() []string {
    return []string{ {{attrNamesList .Methods}} }
}

// Per-method: unpack args, create node, fill slots, return Output
{{range .Methods}}
func (p *{{$.StructName}}Plan) {{.SnakeName}}(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    {{unpackArgs .}}

    node := &execution.Node{
        ID:        generateNodeID("{{.SnakeName}}"),
        Operation: "{{$.Category}}.{{.SnakeName}}",
        Project:   p.project,
    }

    {{range .Params -}}
    if err := FillSlot(node, p.graph, "{{.SnakeName}}", {{.GoName}}Val); err != nil {
        return nil, fmt.Errorf("{{$.SnakeName}}: {{.SnakeName}}: %w", err)
    }
    {{end}}

    p.graph.Nodes = append(p.graph.Nodes, node)
    return NewOutput(node, p.graph, ""), nil
}
{{end}}
```

Template functions:
- `attrNamesList`: produces sorted, quoted, comma-separated names
- `unpackArgs`: generates the `starlark.UnpackArgs(...)` call with proper types

## Step 6: Template 2 -- Graph Operations

Generates `ops_xxx_gen.go`. Pattern matches `ops_package.go`:

```
// Code generated by go.generate; DO NOT EDIT.

package execution

import "fmt"

{{range .Methods}}
type {{$.StructName}}{{.GoName}}Op struct{}

func (o *{{$.StructName}}{{.GoName}}Op) Name() string         { return "{{$.Category}}.{{.SnakeName}}" }
func (o *{{$.StructName}}{{.GoName}}Op) Category() OpCategory { return OpDirect }

func (o *{{$.StructName}}{{.GoName}}Op) Execute(ctx *Context, node Executable) error {
    {{slotReaders .Params}}

    if ctx.DryRun {
        _, _ = fmt.Fprintf(ctx.Logger, "[dry-run] {{$.Category}}.{{.SnakeName}} {{dryRunArgs .Params}}\n"{{dryRunVars .Params}})
        return nil
    }

    _, _ = fmt.Fprintf(ctx.Logger, "[{{$.Category}}] {{.SnakeName}} {{dryRunArgs .Params}}\n"{{dryRunVars .Params}})
    // TODO: call backing implementation
    return nil
}
{{end}}

func {{.StructName}}Ops() []Operation {
    return []Operation{
        {{range .Methods -}}
        &{{$.StructName}}{{.GoName}}Op{},
        {{end -}}
    }
}
```

Template functions:
- `slotReaders`: generates `node.GetSlot()` calls with type conversion
- `dryRunArgs`/`dryRunVars`: generates printf format string and args

Note: The `Execute` method body has a `// TODO` marker for the backing
implementation call. The generator produces the structural scaffold; the
actual implementation dispatch (calling the Go struct's method) is wired
in Phase 3 or by hand.

## Step 7: Template 3 -- Immediate Receiver

Generates `receiver_xxx_gen.go`. Pattern matches `receiver_git.go`:

```
// Code generated by go.generate; DO NOT EDIT.

package starlark

import (
    "fmt"
    "go.starlark.net/starlark"
)

type {{.StructName}}Receiver struct {
    Receiver
}

func New{{.StructName}}Receiver() *{{.StructName}}Receiver {
    return &{{.StructName}}Receiver{Receiver: NewReceiver("{{.Category}}")}
}

func (r *{{.StructName}}Receiver) Attr(name string) (starlark.Value, error) {
    switch name {
    {{range .Methods -}}
    case "{{.SnakeName}}":
        return MakeAttr("{{$.Category}}.{{.SnakeName}}", r.{{.SnakeName}}), nil
    {{end -}}
    default:
        return nil, NoSuchAttrError("{{.Category}}", name)
    }
}

func (r *{{.StructName}}Receiver) AttrNames() []string {
    return []string{ {{attrNamesList .Methods}} }
}

{{range .Methods}}
func (r *{{$.StructName}}Receiver) {{.SnakeName}}(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    {{unpackArgs .}}
    // TODO: call backing implementation, convert result
    return starlark.None, nil
}
{{end}}
```

## Step 8: `go.generate()` method

```go
func (r *GoReceiver) goGenerate(_ *starlark.Thread, _ *starlark.Builtin,
    args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)
```

Parameters:
- `template` (string): one of `"planned_receiver"`, `"graph_ops"`, `"immediate_receiver"`
- `descriptor` (dict): Starlark dict matching `generateDescriptor` shape

Steps:
1. Validate `template` is one of the three known values
2. Convert `descriptor` dict to `generateDescriptor` struct
3. Apply `camelToSnake()` to populate `SnakeName` fields
4. Gate 1: validate all param types map to Starlark
5. Gate 2: validate all return signatures are `(T, error)`
6. Execute the selected template
7. Run `format.Source()` (from `go/format`) on the output
8. Return the formatted Go source as `starlark.String`

## Step 9: Wire into GoReceiver

Add to `Attr()`:
```go
case "generate":
    return MakeAttr("go.generate", r.goGenerate), nil
```

Add `"generate"` to `AttrNames()` (alphabetical position).

## Step 10: Tests

### `TestCamelToSnake`
Table-driven test covering: `Copy`, `WalkTree`, `ConfigGet`, `HTMLParser`,
`ID`, `HTTPSProxy`, `A`, `ABC`.

### `TestValidateReturnSignature`
Table-driven: valid `(string, error)`, `(bool, error)`, `([]string, error)`;
invalid `error`, `string`, `(string, int, error)`, empty string.

### `TestValidateParamTypes`
Valid params (string, bool, int, []string); invalid params (chan, func, *Node);
mixed valid+invalid (reports all invalid).

### `TestGeneratePlanReceiver`
Build a descriptor with 2 methods (`Copy(source, path string)` and
`Remove(path string)`), call `go.generate("planned_receiver", desc)`.
Validate output:
- Contains `type FilePlan struct`
- Contains `case "copy":` and `case "remove":` in Attr switch
- Contains `func (p *FilePlan) copy(` and `func (p *FilePlan) remove(`
- Passes `go/format.Source()` (valid Go syntax)
- AttrNames list is sorted

### `TestGenerateGraphOps`
Same descriptor, template `"graph_ops"`. Validate:
- Contains `type FileCopyOp struct{}` and `type FileRemoveOp struct{}`
- Contains `Name() string { return "file.copy" }`
- Contains `func FileOps() []Operation`
- Valid Go syntax

### `TestGenerateRealtimeReceiver`
Same descriptor, template `"immediate_receiver"`. Validate the immediate receiver:
- Contains `type FileReceiver struct`
- Contains `NewReceiver("file")`
- Contains `MakeAttr("file.copy"`
- Valid Go syntax

### `TestGenerateGateRejectsUnmappedType`
Descriptor with a method that has `chan string` param. Verify `go.generate()`
returns an error mentioning the unmapped type.

### `TestGenerateGateRejectsBadReturn`
Descriptor with a method that returns `error` (not `(T, error)`). Verify
`go.generate()` returns an error.

### `TestGenerateVariadicParam`
Method with `items ...string`. Verify the generated planned receiver unpacks it
correctly (variadic becomes `*args` or joined slot).

### `TestGoGenerateAttr`
Verify `go.generate` appears in `AttrNames()` and `Attr("generate")` returns
a callable builtin.

## Verification

```bash
cd /path/to/noblefactor-ops

# New tests pass
go test ./internal/starlark/ -run "TestCamelToSnake|TestValidate|TestGenerate|TestGoGenerate" -count=1

# Existing tests still pass (no regressions)
go test ./internal/starlark/ -run TestGo -count=1

# Full package
go test ./internal/starlark/ -count=1

# Build + vet
go build ./... && go vet ./internal/starlark/
```
