---
title: "Star Extensions"
description: "Architecture for extending star CLI with new capabilities via YAML specs, Starlark commands, and Go bindings"
status: draft
created: 2025-02-07
updated: 2026-03-23
---

# Star Extensions

This document defines how to extend the star CLI with new capabilities.

## Runtime Types

Star has two core runtime types in package `star` (`cmd/star/star/`). These types
serve double duty: they are both the YAML serialization targets and the runtime
objects. There are no intermediate spec or definition types.

### Extension Lifecycle

Extensions progress through three states:

| State | Gate | Description |
|-------|------|-------------|
| **Discovered** | YAML parsed, validated, deduplicated | Extension found and deserialized from `extension.yaml` |
| **Registered** | In registry, config schema registered, config files loaded | Extension known to the system, config available |
| **Activated** | `.star` files parsed, `RunFunc` set, commands in cobra tree | Extension fully operational |

Each state is a one-way progression. The `State` field on `star.Extension` tracks
the current lifecycle stage.

### star.Extension

Represents a loaded extension. Deserialized directly from `extension.yaml` using
`document.Read` (from `internal/document`), then runtime fields are set as the
extension progresses through its lifecycle. YAML fields are immutable after parsing;
runtime fields are set once during the appropriate lifecycle transition.

| Field | Source | Type | Description |
|-------|--------|------|-------------|
| `Name` | YAML | `string` | Reverse domain identifier (e.g., `com.noblefactor.star.LintGo`) |
| `Description` | YAML | `string` | Brief summary |
| `Commands` | YAML | `[]*Command` | Command definitions from `extension.yaml` |
| `Config` | YAML | `*ConfigSchema` | Config schema (nil if no config) |
| `State` | runtime | `State` | Lifecycle state: Discovered, Registered, Activated |
| `Source` | runtime | `Source` | Origin: `Embedded`, `ProjectLocal`, `User`, `System` |
| `Dir` | runtime | `string` | Absolute path (filesystem) or relative path (embedded) |
| `FS` | runtime | `fs.FS` | Non-nil for embedded extensions |

Config values are resolved on demand:

```go
// Config returns the resolved config values for this extension.
func (e *Extension) Config() *config.ConfigAccessor { ... }
```

### star.Command

Represents a single command within an extension. Deserialized from the `commands:`
section of `extension.yaml`, then runtime fields are set during activation.

| Field | Source | Type | Description |
|-------|--------|------|-------------|
| `Name` | YAML | `string` | Dotted command path (e.g., `lint.go`) |
| `Help` | YAML | `string` | Help text for `--help` |
| `Args` | YAML | `[]Arg` | Positional argument definitions |
| `Flags` | YAML | `[]Flag` | Flag definitions |
| `Implementation` | YAML | `string` | Path to `.star` file (e.g., `commands/lint-go.star`) |
| `Extension` | runtime | `*Extension` | Parent extension |
| `RunFunc` | runtime | `starlark.Callable` | The starlark `run` function |

### Serialization Strategy

`star.Extension` and `star.Command` implement custom `UnmarshalYAML` methods.
YAML deserialization populates only the YAML-sourced fields. Runtime fields
(`State`, `Source`, `Dir`, `FS`, `Extension`, `RunFunc`) are set by the discovery
and loading code as the extension moves through its lifecycle.

Discovery uses `document.Read` from `internal/document` to deserialize
`extension.yaml` files directly into `*star.Extension`. This eliminates
intermediate spec/definition types. The `extension` package handles discovery
and returns `*star.Extension` objects directly.

### Relationship

```
star.Extension (1) ──── (*) star.Command
     │                        │
     ├── Name (yaml)          ├── Name (yaml)
     ├── Description (yaml)   ├── Help (yaml)
     ├── Commands (yaml)      ├── Args (yaml)
     ├── Config (yaml)        ├── Flags (yaml)
     ├── State (runtime)      ├── Implementation (yaml)
     ├── Source (runtime)     ├── Extension → parent (runtime)
     ├── Dir (runtime)        └── RunFunc (runtime)
     ├── FS (runtime)
     └── Config()
```

## Command Execution

Starlark command functions receive two arguments: the command and the
starlark runtime context.

```python
def run(command, ctx):
    cfg = command.extension.config()
    path = ctx.args.get("path", ".")

    if ctx.dry_run:
        ui.note("Would process: " + path)
        return

    files = fs.glob(path + "/**/*.go")
    ui.success("Processed " + str(len(files)) + " files")
```

**`command`** — the immutable `star.Command`, exposed as a starlark struct:
- `command.name` — the command name (e.g., `lint.go`)
- `command.extension` — the parent extension
- `command.extension.name` — extension identifier
- `command.extension.dir` — extension directory
- `command.extension.config()` — resolved config values

**`ctx`** — per-invocation context from the starlark runtime:
- `ctx.args` — resolved flag and positional argument values
- `ctx.dry_run` — `True` when `--dry-run` is set

## Extension Source Structure

Extensions use a reverse domain naming convention:

```
com.noblefactor.star.LintCopyright/
├── extension.yaml               # Manifest (single source of truth)
└── commands/
    └── lint-copyright.star      # Implementation (defines run function)
```

### Directory Naming

- Directory name = extension identifier (reverse domain format)
- Example: `com.noblefactor.star.LintCopyright`
- Commands go in `commands/` subdirectory

## Extension Discovery and Loading

### Search Path

Extensions are discovered from four sources in priority order.
First seen wins — if the same extension name appears in multiple sources,
the highest-priority source takes precedence.

| Priority | Source | Path |
|----------|--------|------|
| 1 (highest) | Project-local | `${GIT_WORKSPACE_ROOT}/star/extensions/` |
| 2 | User | `${XDG_DATA_HOME}/star/extensions/` (default `~/.local/share`) |
| 3 | System | `/usr/local/share/star/extensions/` |
| 4 (lowest) | Embedded | Compiled into the binary via `//go:embed` |

### Loading Process

The loading process follows the three lifecycle states:

**1. Discover** → State: **Discovered**

Discovery walks search paths in priority order (project-local → user → system →
embedded). For each `extension.yaml` found, it deserializes into `*star.Extension`
using `document.Read`, sets runtime fields (`Source`, `Dir`, `FS`), sets
`Extension` back-pointers on child commands, and validates. Deduplication happens
inside discovery: a map keyed by extension name skips any extension already seen.
The output is an ordered `[]*star.Extension`.

**2. Register** → State: **Registered**

For each discovered extension: add to `extension.Registry`, register its config
schema with the config system. After all extensions are registered, load config
files so that merged values are available. Config must be fully loaded before
activation because commands may read config at activation time.

**3. Activate** → State: **Activated**

For each registered extension: parse its `.star` files, set `RunFunc` on each
command, wire commands into the cobra command tree.

Extensions are independent. There are no cross-extension dependencies.
Order within the set is irrelevant for registration and activation.

### Configuration Hierarchy

Config files are loaded in priority order (highest to lowest):

```
1. ./star/config.yaml                   # Project config
2. ~/.config/star/config.yaml           # User config ($XDG_CONFIG_HOME)
3. Built-in defaults from extension.yaml
```

Config values are resolved on demand when accessed via `command.extension.config()`.

## Extension Specification

An extension is described by an `extension.yaml` file:

```yaml
extension: com.noblefactor.star.LintCopyright
description: Check or fix copyright headers in source files

commands:
  - name: lint.copyright
    help: Check or fix copyright headers in source files
    implementation: commands/lint-copyright.star
    flags:
      - name: fix
        type: bool
        default: "false"
        help: Add missing headers and update old format
      - name: path
        type: string
        default: "."
        help: Path to check

config:
  type: CopyrightConfig
  fields:
    enabled: bool
    license: string
    holder: string
    exclude: "[]string"
  defaults:
    enabled: false
    license: "auto"
```

### Naming Convention

| Element | Value | Derivation |
|---------|-------|------------|
| Extension | `com.noblefactor.star.LintCopyright` | Reverse domain identifier |
| Directory | `com.noblefactor.star.LintCopyright/` | Same as extension name |
| Command | `star lint copyright` | Command name with dots as subcommands |
| Config path | `lint.copyright` | Command name |
| Env var prefix | `STAR_LINT_COPYRIGHT_` | `STAR_` + command name (dots → underscores, uppercase) |

### Flag Resolution

Each flag resolves in priority order:

1. CLI argument: `--fix`
2. Environment variable: `STAR_LINT_COPYRIGHT_FIX`
3. Config file: `lint.copyright.fix`
4. Default: `"false"`

## Extension Examples

### Simple Extension

Orchestrates existing providers. No new Go code needed.

```yaml
extension: com.noblefactor.star.LintAll
description: Run all configured linters

commands:
  - name: lint.all
    help: Run all linters
    implementation: commands/lint-all.star
```

### Extension with Config

Commands plus a typed configuration schema with defaults.

```yaml
extension: com.noblefactor.star.LintCopyright
description: Check or fix copyright headers

commands:
  - name: lint.copyright
    help: Check or fix copyright headers
    implementation: commands/lint-copyright.star
    flags:
      - name: fix
        type: bool
        default: "false"
        help: Auto-fix issues

config:
  type: CopyrightConfig
  fields:
    enabled: bool
    license: string
    holder: string
  defaults:
    enabled: false
    license: "auto"
```

## Extension Components

| Component | Required | Language | Purpose |
|-----------|----------|----------|---------|
| **Commands** | Required | Starlark | CLI subcommand implementations |
| **Config Schema** | Optional | YAML | Typed configuration with defaults |

**Minimum requirement:** An extension must define at least one command.

Providers (file, shell, regexp, yaml, ui, etc.) are part of the framework — they are
available to all extensions automatically. Extensions do not declare or define providers.

## Files

| File | Purpose |
|------|---------|
| `cmd/star/star/extension.go` | star.Extension — runtime type, YAML unmarshaler, starlark interface |
| `cmd/star/star/command.go` | star.Command — runtime type, YAML unmarshaler, starlark execution |
| `cmd/star/star/application.go` | Application runtime, discover-and-load orchestration |
| `cmd/star/extension/discovery.go` | Extension discovery, search paths |
| `cmd/star/extension/registry.go` | Global extension registry |
| `cmd/star/config/config.go` | Config loading/saving |
| `cmd/star/cli/selfinstall.go` | Self-install command |
| `cmd/star/extensions.go` | `//go:embed extensions` (bundled extensions) |
| `cmd/star/main.go` | Cobra root command, flag wiring |

## Design Principles

1. **Reverse domain naming** — Extension identifier = directory name
2. **One file per command** — Commands in `commands/` subdirectory
3. **Three lifecycle states** — Discovered → Registered → Activated, one-way progression
4. **Deserialize into runtime types** — `document.Read` into `*star.Extension` directly, no intermediate spec types
5. **Discover then register then activate** — Dedup by priority in discovery, register all config before activating any extension
6. **Convention over configuration** — Command name determines CLI paths
7. **Starlark-first** — Commands implemented in Starlark
8. **Providers are framework** — Extensions consume providers, they don't declare them
9. **`run(command, ctx)`** — Command is identity, context is per-invocation state
