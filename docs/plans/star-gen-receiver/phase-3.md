# Phase 3: Create the Star Extension

## Context

Phase 0 (single-operation nodes) is merged in devlore-cli (PR #97). Phase 1 (parameter
extraction) is merged in noblefactor-ops (PR #65). Phase 2 (`go.generate()` with
templates) is merged in noblefactor-ops (PR #66). Phase 3 creates the
`com.noblefactor.star.GenReceiver` extension that orchestrates the full pipeline:
read a Go implementation struct, filter and analyze its methods, build the descriptor,
and call `go.generate()` for each template.

**Repo**: noblefactor-ops
**New files**:
- `star/extensions/com.noblefactor.star.GenReceiver/extension.yaml`
- `star/extensions/com.noblefactor.star.GenReceiver/commands/gen-receiver.star`

## Design Decisions

### The .star command does orchestration, not generation

The heavy lifting (templates, type mapping, gate validation, formatting) lives in
`go.generate()` (Phase 2). The `.star` command is a thin orchestrator:
1. Call `go.methods()` to discover method signatures
2. Filter: public methods, skip starlark.Value interface methods, apply `--methods` inclusion list
3. Build the descriptor dict
4. Call `go.generate()` for each requested template
5. Write output files with `file.write()`

### Skip list is hardcoded in the .star command

Methods from `starlark.Value` and `starlark.HasAttrs` interfaces are automatically
excluded: `String`, `Type`, `Freeze`, `Truth`, `Hash`, `Attr`, `AttrNames`. These are
boilerplate the templates generate themselves -- including them would create duplicates.

### Output file naming follows established conventions

| Template | Output File | Example |
|---|---|---|
| `planned_receiver` | `planned_{category}_gen.go` | `planned_file_gen.go` |
| `graph_ops` | `ops_{category}_gen.go` | `ops_file_gen.go` |
| `immediate_receiver` | `immediate_{category}_gen.go` | `immediate_file_gen.go` |

### Namespace derivation

The namespace is derived from the template and category:
- `planned_receiver`: `"plan.{category}"` (e.g., `"plan.file"`)
- `graph_ops`: not used (ops don't have namespaces)
- `immediate_receiver`: `"{category}"` (e.g., `"file"`)

### Package derivation

The package name is derived from the template:
- `planned_receiver`: `"starlark"` (planned receivers live in `internal/starlark/`)
- `graph_ops`: `"execution"` (ops live in `internal/execution/`)
- `immediate_receiver`: `"starlark"` (immediate receivers live in `internal/starlark/`)

These defaults can be overridden with the `--package` flag for non-standard layouts.

### Category and struct name derivation

Given `--struct FileOps`:
- `struct_name`: `"File"` (strip common suffixes: `Ops`, `Impl`, `Service`, `Handler`)
- `category`: `"file"` (snake_case of struct_name)

The `--category` flag overrides the derived value.

### Dry-run by default

The command writes generated files to stdout by default. The `--write` flag writes
files to the `--output` directory. This matches the principle of least surprise
and lets users inspect output before committing.

## Step 1: `extension.yaml`

```yaml
# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

extension: com.noblefactor.star.GenReceiver
description: Generate planned receivers, graph operations, and immediate receivers from Go structs

receivers:
  - name: go
    builtin: true
    type: GoReceiver
    description: Go source parsing and code generation
  - name: file
    builtin: true
    type: FileReceiver
    description: Filesystem operations

commands:
  - name: gen.receiver
    help: Generate boilerplate receivers and operations from a Go struct
    implementation: commands/gen-receiver.star
    flags:
      - name: path
        type: string
        default: ""
        help: Path to the Go package containing the struct
      - name: struct
        type: string
        default: ""
        help: Name of the Go struct to generate from
      - name: category
        type: string
        default: ""
        help: Override the derived category name (e.g., "file")
      - name: package
        type: string
        default: ""
        help: Override the Go package name for generated files
      - name: methods
        type: string
        default: ""
        help: Comma-separated list of methods to include (default all public)
      - name: templates
        type: string
        default: "planned_receiver,graph_ops,immediate_receiver"
        help: Comma-separated list of templates to generate
      - name: output
        type: string
        default: ""
        help: Output directory for generated files (default stdout)
      - name: write
        type: bool
        default: "false"
        help: Write files to output directory instead of stdout
```

## Step 2: `gen-receiver.star` -- Argument Parsing and Validation

```python
# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# gen-receiver.star - Generate receivers and operations from Go structs
#
# Reads a Go implementation struct's methods via go.methods(), then calls
# go.generate() to produce planned receivers, graph operations, and immediate
# receivers.

# Methods from starlark.Value and starlark.HasAttrs -- always excluded
SKIP_METHODS = [
    "String", "Type", "Freeze", "Truth", "Hash",
    "Attr", "AttrNames",
]

# Common struct name suffixes to strip when deriving category
STRIP_SUFFIXES = ["Ops", "Impl", "Service", "Handler"]

# Template to output filename mapping
TEMPLATE_FILES = {
    "planned_receiver": "planned_%s_gen.go",
    "graph_ops": "ops_%s_gen.go",
    "immediate_receiver": "immediate_%s_gen.go",
}

# Template to default package mapping
TEMPLATE_PACKAGES = {
    "planned_receiver": "starlark",
    "graph_ops": "execution",
    "immediate_receiver": "starlark",
}

def run(ctx):
    """Generate receivers and operations from a Go struct."""
    # ... (see Step 3-7 below)
```

The `run(ctx)` function body is detailed in Steps 3-7.

## Step 3: Validate Required Arguments

```python
    path = ctx.args.get("path", "")
    struct_name = ctx.args.get("struct", "")

    if not path:
        fail("--path is required")
    if not struct_name:
        fail("--struct is required")
```

## Step 4: Discover and Filter Methods

```python
    # Get all methods for the struct
    methods = go.methods(path, receiver_type=struct_name)

    if len(methods) == 0:
        fail("no methods found for " + struct_name + " in " + path)

    # Filter to public methods, excluding skip list
    methods_filter = ctx.args.get("methods", "")
    include_list = []
    if methods_filter:
        include_list = methods_filter.split(",")

    filtered = []
    for m in methods:
        # Skip non-public methods (lowercase first letter)
        if m.name[0].islower():
            continue
        # Skip starlark.Value / HasAttrs interface methods
        if m.name in SKIP_METHODS:
            continue
        # Apply inclusion filter if specified
        if include_list and m.name not in include_list:
            continue
        filtered.append(m)

    if len(filtered) == 0:
        fail("no eligible methods after filtering for " + struct_name)

    note("Found " + str(len(filtered)) + " methods for " + struct_name)
```

## Step 5: Derive Names and Build Descriptor

```python
    # Derive struct_name_short by stripping common suffixes
    struct_short = struct_name
    for suffix in STRIP_SUFFIXES:
        if struct_short.endswith(suffix) and len(struct_short) > len(suffix):
            struct_short = struct_short[:-len(suffix)]
            break

    # Category override or derive from struct name
    category = ctx.args.get("category", "")
    if not category:
        # Simple CamelCase to snake_case for single words
        # For multi-word names, insert _ before uppercase letters
        category = to_snake(struct_short)

    pkg_override = ctx.args.get("package", "")

    # Build method descriptors
    method_descriptors = []
    for m in filtered:
        params = []
        for p in m.params:
            params.append({
                "name": p.name,
                "type": p.type,
                "variadic": p.variadic,
            })
        method_descriptors.append({
            "name": m.name,
            "returns": m.returns,
            "doc": m.doc,
            "params": params,
        })
```

## Step 6: Generate Code for Each Template

```python
    templates_str = ctx.args.get("templates", "planned_receiver,graph_ops,immediate_receiver")
    templates = templates_str.split(",")
    output_dir = ctx.args.get("output", "")
    write_files = ctx.args.get("write", "false") == "true"

    for tmpl in templates:
        tmpl = tmpl.strip()
        if tmpl not in TEMPLATE_FILES:
            fail("unknown template: " + tmpl + " (valid: planned_receiver, graph_ops, immediate_receiver)")

        # Derive namespace
        if tmpl == "planned_receiver":
            namespace = "plan." + category
        elif tmpl == "immediate_receiver":
            namespace = category
        else:
            namespace = category

        # Derive package
        pkg = pkg_override if pkg_override else TEMPLATE_PACKAGES[tmpl]

        descriptor = {
            "package": pkg,
            "category": category,
            "struct_name": struct_short,
            "namespace": namespace,
            "methods": method_descriptors,
        }

        note("Generating " + tmpl + " for " + struct_short + "...")
        code = go.generate(tmpl, descriptor)

        filename = TEMPLATE_FILES[tmpl] % category
        if write_files and output_dir:
            out_path = file.join(output_dir, filename)
            file.write(out_path, code)
            success("Wrote " + out_path)
        else:
            note("--- " + filename + " ---")
            note(code)
```

## Step 7: `to_snake` Helper

```python
def to_snake(name):
    """Convert CamelCase to snake_case."""
    result = []
    for i, ch in enumerate(name.elems()):
        if ch.isupper():
            if i > 0:
                prev = name.elems()[i - 1]
                if prev.islower():
                    result.append("_")
                elif prev.isupper() and i + 1 < len(name) and name.elems()[i + 1].islower():
                    result.append("_")
            result.append(ch.lower())
        else:
            result.append(ch)
    return "".join(result)
```

This mirrors the `camelToSnake()` logic in Go. It's needed in Starlark for deriving
the category from the struct name (before passing to `go.generate()`, which applies
its own snake_case conversion to method and param names internally).

## Step 8: Full Command File

The complete `gen-receiver.star` combines Steps 2-7 into a single file. The final
structure is:

```
gen-receiver.star
├── SKIP_METHODS constant
├── STRIP_SUFFIXES constant
├── TEMPLATE_FILES constant
├── TEMPLATE_PACKAGES constant
├── to_snake(name) helper
└── run(ctx) main function
    ├── Validate arguments (Step 3)
    ├── Discover and filter methods (Step 4)
    ├── Derive names and build descriptor (Step 5)
    └── Generate code for each template (Step 6)
```

## Step 9: End-to-End Test

Manually test against devlore-cli's `FileOps` struct (or equivalent implementation
struct) to verify the pipeline:

```bash
cd /path/to/noblefactor-ops

# Preview output (dry-run)
star gen.receiver --path /path/to/devlore-cli/internal/host --struct FileOps

# Write to temp directory
star gen.receiver \
  --path /path/to/devlore-cli/internal/host \
  --struct FileOps \
  --output /tmp/gen-test \
  --write

# Verify generated code is valid Go
gofmt -e /tmp/gen-test/plan_file_gen.go
gofmt -e /tmp/gen-test/ops_file_gen.go
gofmt -e /tmp/gen-test/receiver_file_gen.go

# Verify gate errors work
star gen.receiver --path /path/to/bad-types --struct BadStruct
# Should fail with "unmapped parameter types" error
```

## Verification

```bash
cd /Users/david-noble/Workspace/NobleFactor/noblefactor-ops

# Extension YAML is valid (parsed without error)
go test ./internal/extension/ -run TestParseSpec -count=1

# Existing tests still pass
go test ./internal/starlark/ -count=1

# Build succeeds
go build ./...

# Extension structure is correct
ls star/extensions/com.noblefactor.star.GenReceiver/
# extension.yaml
# commands/gen-receiver.star
```

## Example Usage

After Phase 3, a developer adding a new `DockerOps` struct would run:

```bash
# Generate all three files
star gen.receiver \
  --path ./internal/host \
  --struct DockerOps \
  --output ./internal \
  --write

# Files created:
# ./internal/starlark/plan_docker_gen.go
# ./internal/execution/ops_docker_gen.go
# ./internal/starlark/receiver_docker_gen.go
```

Then wire in with three one-liners:
1. `plan_root.go`: add `dockerPlan` field + `case "docker":` in `Attr()`
2. `ops.go`: add `DockerOps()` to `AllOps()`
3. `bindings.go`: add `"docker": NewDockerReceiver()` to `Globals()`
