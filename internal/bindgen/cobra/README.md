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

### What Doesn't Work
- **No subcommand hierarchy tree** - We flatten with prefixes, don't build parent-child tree
- **Limited receiver detection** - Only detects `flags` or `f` variable names
- **Helper function flags** - Flags added via `addFlags(flags)` helper functions are missed

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

## Honest Assessment

### Extractor Issues

| Issue | Severity | Status | Impact | Location |
|-------|----------|--------|--------|----------|
| Command name collision | Critical | **FIXED** | Was losing ~20% of commands | `extractor.go:237-253` |
| Limited receiver detection | Medium | Open | Missing ~60% of flags | `extractor.go:373-384` |
| Helper function flags missed | Medium | Open | Flags added via `addFlags()` not captured | N/A |
| No subcommand hierarchy tree | Low | Open | We flatten with prefixes, don't build tree | N/A |
| Silent unquote failures | Low | Open | Malformed strings silently dropped | `extractor.go:332, 455` |

### Generator Issues

| Issue | Severity | Status | Impact | Location |
|-------|----------|--------|--------|----------|
| Command name not split | Critical | Open | `container_run` → should be `container run` | `codegen.go` template |
| No positional args | Critical | Open | Can't pass image names, container IDs, etc. | `codegen.go` template |
| Stub template broken | Medium | Open | `title` function undefined | `stubgen.go:9` |

**Bottom line:** Extractor captures all 144 commands but misses ~60% of flags. Generator produces scaffolding with critical bugs. **Not production-ready** — use as development accelerator with manual refinement.

## Known Issues

### FIXED: Command Name Collision

**Status:** Fixed on 2026-01-11

**Solution:** Use package directory as command prefix. Commands in `cli/command/container/` get prefixed with `container_`.

**Location:** `extractor.go:178-200` (`calculatePrefix`) and `extractor.go:237-253` (key generation)

```go
// Generate qualified key to avoid collisions
// e.g., "container" + "create" -> "container_create"
key := currentCmd.Name
if prefix != "" && prefix != currentCmd.Name {
    key = prefix + "_" + currentCmd.Name
}
```

### Low: No Subcommand Hierarchy Tree

**Problem:** We flatten the command tree using prefixes rather than building a true hierarchy.

**Impact:** Minor - the prefixed names work well for binding generation. A true tree would only be needed for generating CLI help text or command groupings.

**Not blocking for current use case.**

### Medium: Limited Receiver Detection

**Problem:** `isFlagReceiver()` only matches `flags` or `f` as variable names.

**Location:** `extractor.go:329-340`

```go
case *ast.Ident:
    return v.Name == "flags" || v.Name == "f"  // Misses "fs", "flagSet", etc.
```

**Fix:** Be more permissive or track actual variable assignments.

### Low: Dead Parameter

**Problem:** `fset` parameter passed to `extractFromFunction` but never used.

**Location:** `extractor.go:168`

```go
func (e *Extractor) extractFromFunction(fn *ast.FuncDecl, fset *token.FileSet) {
    // fset is never used
```

**Fix:** Remove parameter or use it for position information in error messages.

### Low: Silent Unquote Failures

**Problem:** `strconv.Unquote` errors are silently ignored.

**Location:** `extractor.go:289, 411`

```go
s, _ := strconv.Unquote(v.Value)  // Error ignored
```

**Fix:** Log warnings in verbose mode.

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

## Next Steps (Priority Order)

1. ~~**Fix command collision**~~ - DONE: Using package directory prefixes
2. **Improve receiver detection** - Track variable assignments or match more patterns
3. **Handle helper functions** - Track flags added via helper functions like `addFlags()`
4. **Add tests** - Unit tests for extraction logic
5. **Build hierarchy tree** (optional) - Parse `AddCommand()` calls if tree structure needed

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

### Why Generated Bindings Are Incomplete

1. **Missing flags (~60%)** — Extractor only recognizes `flags` or `f` receivers. Docker-cli uses `opts.Flags()`, `copts.AddFlags()`, etc.

2. **Broken command invocation** — Generator emits `container_run` as a single arg. Should be `container`, `run` as separate args.

3. **No positional args** — Generator doesn't emit code to handle positional arguments (e.g., image name for `docker run`).

4. **No helper patterns** — Flags added via `addCommonFlags(cmd)` helpers aren't captured.

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

**What humans must still do:**
- Fix command invocation (`container_run` → `container run`)
- Add missing flags (check `docker <cmd> --help`)
- Add positional argument handling
- Add return value parsing where useful (e.g., `docker ps --format json`)

### Generator Bugs to Fix

| Bug | Location | Impact |
|-----|----------|--------|
| Command name splitting | `codegen.go` template | Commands don't execute |
| Missing positional args | `codegen.go` template | Can't pass image names, etc. |
| Stub template broken | `stubgen.go:9` | IDE stubs don't generate |

### Conclusion

**The extractor works. The generator produces scaffolding. Human refinement is required.**

For now, the practical approach is:
1. Use generated code as reference (what commands exist, what flags are documented)
2. Hand-write high-value bindings (as done in `docker.go`)
3. Use extraction diff to detect new commands/flags on version bumps
