# Cobra Extractor

Extracts Starlark binding metadata from Go source files that use [spf13/cobra](https://github.com/spf13/cobra).

## Status: Proof of Concept (Working)

**Created:** 2026-01-11
**Last tested:** docker-cli v27.4.1 on 2026-01-11

### What Works
- Package discovery and loading via `golang.org/x/tools/go/packages`
- AST traversal via `golang.org/x/tools/go/ast/inspector`
- Extracts `cobra.Command` struct literals (Use, Short, Long, Deprecated, Hidden)
- Extracts flag definitions from `flags.StringVarP()`, `BoolVar()`, etc.
- Type inference from method names (StringSlice → string_list)
- Function-level scope for command/flag association
- **Qualified command names** - Uses package directory for prefixes (`container_create`, `config_create`)

### Known Limitations

See [TODO.md Section 7: Bindgen Tool](../../../TODO.md#7-bindgen-tool) for tracked issues and roadmap.

## Test Results

```
Source: docker-cli v27.4.1
Location: ~/Workspace/OSS/docker-cli/

Extracted: 144 commands, 272 flags (up from 86/175 before prefix fix)

All 10 "create" commands now captured:
  checkpoint_create, config_create, container_create, context_create,
  manifest_create, network_create, plugin_create, secret_create,
  service_create, volume_create
```

See [TODO.md Section 7](../../../TODO.md#7-bindgen-tool) for detailed issue tracking.

## Architecture

```
ExtractDir(dir)
    │
    ├── findPackages(dir)           # Walk tree, collect package patterns
    │
    ├── packages.Load(patterns)     # golang.org/x/tools loads AST
    │
    └── for each package:
            │
            ├── Check imports cobra?
            │
            ├── inspector.New(syntax)   # Efficient AST inspector
            │
            └── inspector.Preorder(FuncDecl):
                    │
                    └── extractFromFunction(fn)
                            │
                            ├── Find &cobra.Command{} literal
                            │   └── Extract Use, Short, Long, etc.
                            │
                            └── Find flags.StringVarP() calls
                                └── Extract name, short, type, default, desc
```

## Usage

```bash
cd /path/to/lore  # clone of github.com/NobleFactor/lore

# Extract from docker-cli (requires go.mod symlink, see below)
go run ./cmd/bindgen extract-cobra ~/Workspace/OSS/docker-cli/cli/command/ > docker.yaml

# With verbose output
go run ./cmd/bindgen extract-cobra ~/Workspace/OSS/docker-cli/cli/command/ --verbose
```

### Docker-CLI Setup

Docker-cli uses `vendor.mod` instead of `go.mod`. Symlinks were created:

```bash
cd ~/Workspace/OSS/docker-cli
ln -sf vendor.mod go.mod
ln -sf vendor.sum go.sum
```

## Files

```
internal/bindgen/cobra/
├── extractor.go    # Main extraction logic (~430 lines)
└── README.md       # This file

cmd/bindgen/
└── main.go         # CLI with extract-cobra subcommand
```

## Dependencies

```go
import (
    "golang.org/x/tools/go/ast/inspector"  // Efficient AST traversal
    "golang.org/x/tools/go/packages"       // Package loading
)
```

Added to go.mod:
- `golang.org/x/tools v0.40.0`
- `golang.org/x/mod v0.31.0`
- `golang.org/x/sync v0.19.0`

## Related Documentation

- ADR-016 in `lore/design/07-lore-design-decisions.md` documents:
  - Prior art research on Starlark binding generation
  - Source introspection approach (this extractor)
  - Comparison with --help parsing
  - Stability analysis of CLI APIs

## Test Data

- Source: `~/Workspace/OSS/docker-cli/` (v27.4.1, shallow clone)
- Output: 144 commands, 272 flags (after prefix fix)

## End-to-End Code Generation Analysis

**Question:** Is binding generation fully automated, or is the extractor output used to bootstrap manual coding?

**Answer:** The pipeline is fully automated but produces incomplete bindings. Human refinement is required for production use.

### Generated Output

Running the full pipeline:
```bash
bindgen extract-cobra ~/Workspace/OSS/docker-cli/cli/command/ > docker.yaml
bindgen generate docker.yaml
```

Produces:
- `command_gen.go` — 4,124 lines, 144 commands
- `command_gen.star` — IDE stubs (currently broken, template bug)

### Generated Code Sample

```go
// containerRun executes command container_run.
// Create and run a new container from an image
func (b *CommandBindings) containerRun(...) (starlark.Value, error) {
    var detach bool
    var name string
    // ... only 6 flags extracted ...

    cmdArgs := []string{"container_run"}  // BUG: should be "container", "run"
    if detach {
        cmdArgs = append(cmdArgs, "--detach")
    }
    return b.runCommand(cmdArgs)
}
```

### Comparison: Generated vs Hand-Written

| Metric | Hand-written (`docker.go`) | Generated (`command_gen.go`) |
|--------|---------------------------|------------------------------|
| Commands | 15 (curated high-value) | 144 (all extracted) |
| Flags per command | Complete | Partial (~6 of ~100 for `run`) |
| Positional args | Handled | **Not generated** |
| Command invocation | Correct (`docker run`) | **Broken** (`docker container_run`) |
| Error handling | Varies | Consistent |
| Code size | 634 lines | 4,124 lines |
| Production ready | Yes | No |

### Intended Workflow

The generator is a **development accelerator**, not a production-ready solution:

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│ 1. Extract      │ ──▶ │ 2. Generate     │ ──▶ │ 3. Refine       │
│                 │     │                 │     │                 │
│ bindgen extract │     │ bindgen generate│     │ Human review:   │
│ 144 commands    │     │ 4,124 lines Go  │     │ - Fix command   │
│ 272 flags       │     │ (scaffold)      │     │ - Add flags     │
│                 │     │                 │     │ - Add pos args  │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

**Value proposition:**
- Saves writing boilerplate for 144 commands from scratch
- Ensures consistent structure
- Documents all available commands and their descriptions
- Identifies which flags exist (even if not all captured)

### Conclusion

**The extractor works. The generator produces scaffolding. Human refinement is required.**

See [TODO.md Section 7](../../../TODO.md#7-bindgen-tool) for known issues and roadmap.
