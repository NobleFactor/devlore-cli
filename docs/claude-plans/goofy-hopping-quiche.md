# Worker 2 Phase 4: Extension Loading and Config Integration

## Overview

Integrate extension discovery with the Starlark runtime, enabling extension-defined commands to be loaded and executed alongside traditional `ops/*.star` files.

## Branch Setup (First Step)

Phase 3 has been merged. Create Phase 4 branch:

```bash
git checkout develop
git pull origin develop
git checkout -b feature/ext-starlark-phase-4
```

## Deliverables

Per the plan document (docs/plans/star-agent-team-refactor.md):
1. Modify `runtime.go` - Load extensions before Starlark files
2. Modify `builtin_config.go` - Use new Config system
3. Flag resolution uses extension spec defaults

## Files to Modify

| File | Changes |
|------|---------|
| `internal/starlark/runtime.go` | Add extension loading, flag defaults, refactor predeclared |
| `internal/starlark/builtin_config.go` | Minor updates for extensible config support |

## Implementation Steps

### Step 1: Extend Runtime Struct (runtime.go)

Add fields for extensions:

```go
type Runtime struct {
    commands      map[string]*Command
    opsDir        string
    extensionsDir string                    // NEW: default "extensions"
    extConfig     *config.ExtensibleConfig  // NEW: for extension configs
}
```

Update `NewRuntime`:
```go
func NewRuntime(opsDir string) *Runtime {
    return &Runtime{
        commands:      make(map[string]*Command),
        opsDir:        opsDir,
        extensionsDir: "extensions",
    }
}
```

### Step 2: Add LoadExtensions Method (runtime.go)

New method to discover and load extensions:

```go
func (r *Runtime) LoadExtensions() error {
    // Skip if extensions directory doesn't exist
    if _, err := os.Stat(r.extensionsDir); os.IsNotExist(err) {
        return nil
    }

    specs, err := extension.Discover(r.extensionsDir)
    if err != nil {
        return fmt.Errorf("discover extensions: %w", err)
    }

    for _, spec := range specs {
        // Register with global extension registry
        extension.Register(spec) // ignore duplicate errors

        // Register config if extension has one
        if spec.HasConfig() {
            r.extConfig.RegisterExtension(spec.Extension, spec.ToConfigSpec())
        }

        // Load Starlark implementation if has command
        if spec.HasCommand() {
            r.loadExtensionCommand(spec)
        }
    }
    return nil
}
```

### Step 3: Add loadExtensionCommand Method (runtime.go)

Load individual extension's Starlark command:

```go
func (r *Runtime) loadExtensionCommand(spec *extension.ExtensionSpec) error {
    if spec.Command == nil || spec.Command.Implementation == "" {
        return nil
    }

    // Find extension directory and implementation path
    extDir, _ := extension.FindExtensionDir(spec.Extension)
    implPath := filepath.Join(extDir, spec.Command.Implementation)

    // Execute Starlark file with predeclared environment
    // Register commands with name from spec.Extension (e.g., "lint.copyright" -> "lint copyright")
    ...
}
```

### Step 4: Add applyFlagDefaults Method (runtime.go)

Merge flag defaults from extension spec into command:

```go
func (r *Runtime) applyFlagDefaults(cmd *Command, spec *extension.ExtensionSpec) {
    // For each flag in spec.Flags:
    //   - Set cmd.Flags[i].Default if not already set
    //   - Set cmd.Flags[i].Help if not already set
    //   - Add flag to cmd.Flags if not present
}
```

### Step 5: Refactor buildPredeclared (runtime.go)

Extract predeclared dict building to support extension context:

```go
func (r *Runtime) buildPredeclared(collector *commandCollector, spec *extension.ExtensionSpec) starlark.StringDict {
    predeclared := starlark.StringDict{
        "fs":             fsModule(),
        "json":           jsonModule(),
        // ... other modules
        "config":         r.configModuleWithExtension(spec),  // NEW: extension-aware
        "command":        starlark.NewBuiltin("command", collector.commandBuiltin),
        // ... output builtins
    }
    return predeclared
}
```

### Step 6: Update LoadAll (runtime.go)

Load extensions before ops files:

```go
func (r *Runtime) LoadAll() error {
    // Load extensions first
    if err := r.LoadExtensions(); err != nil {
        return fmt.Errorf("loading extensions: %w", err)
    }

    // Then walk ops directory (existing code)
    return filepath.WalkDir(r.opsDir, ...)
}
```

### Step 7: Update Load (runtime.go)

Use refactored buildPredeclared:

```go
func (r *Runtime) Load(path string) error {
    collector := &commandCollector{commands: make(map[string]*Command)}
    predeclared := r.buildPredeclared(collector, nil)  // nil spec for legacy files
    // ... rest unchanged
}
```

### Step 8: Add configModuleWithExtension (runtime.go)

New config module that uses ExtensibleConfig when available:

```go
func (r *Runtime) configModuleWithExtension(spec *extension.ExtensionSpec) *starlarkstruct.Module {
    return &starlarkstruct.Module{
        Name: "config",
        Members: starlark.StringDict{
            "get":  starlark.NewBuiltin("config.get", r.configGetWithExtension),
            "show": starlark.NewBuiltin("config.show", configShow),  // unchanged
            "sync": starlark.NewBuiltin("config.sync", configSync),  // unchanged
        },
    }
}

func (r *Runtime) configGetWithExtension(...) (starlark.Value, error) {
    if r.extConfig != nil {
        return config.WrapAsStarlarkValue(r.extConfig), nil
    }
    // Fall back to legacy config.Load()
    cfg, err := config.Load()
    return cfg.ToStarlark(), nil
}
```

### Step 9: Update builtin_config.go (minimal)

The existing `configModule()`, `configLoad()`, `configShow()`, `configSync()` remain unchanged for backward compatibility. The Runtime will provide its own config module when extensions are loaded.

## Required Imports (runtime.go)

```go
import (
    "github.com/NobleFactor/noblefactor-ops/internal/extension"
    "github.com/NobleFactor/noblefactor-ops/internal/config"
)
```

## APIs Used from Other Workers

**From Worker 3 (extension package):**
- `extension.Discover(dir) ([]*ExtensionSpec, error)` - scan for extensions
- `extension.Register(spec)` - register to global registry
- `extension.FindExtensionDir(name) (string, error)` - locate extension dir
- `ExtensionSpec.HasCommand()`, `HasConfig()` - check capabilities
- `ExtensionSpec.ToConfigSpec()` - convert config schema

**From Worker 1 (config package):**
- `config.NewExtensibleConfig(source)` - create config root
- `ExtensibleConfig.RegisterExtension(path, spec)` - register extension config
- `config.WrapAsStarlarkValue(c)` - wrap for Starlark access

## Test Strategy

1. **Unit Tests:**
   - `TestRuntime_LoadExtensions` - discovery, registration, config
   - `TestRuntime_loadExtensionCommand` - command loading, name transformation
   - `TestRuntime_applyFlagDefaults` - flag merging
   - `TestRuntime_buildPredeclared` - module availability

2. **Integration Tests:**
   - Create temp extension directory with YAML + .star
   - Load and execute extension command
   - Verify config access from Starlark

## Verification

```bash
go test ./internal/starlark/...
go build ./...

# Run existing commands to verify no regression
./star lint go
./star lint copyright
./star setup config
```

## Dependencies

- Phase 3 (command.go extraction) - COMPLETE
- Worker 3 (extension package) - COMPLETE
- Worker 1 (config package) - COMPLETE
- Worker 5 (wasm callbacks) - NOT REQUIRED for this phase

## Critical Files

- `/internal/starlark/runtime.go` - Primary modifications
- `/internal/starlark/builtin_config.go` - Minor updates
- `/internal/extension/discovery.go` - API to consume
- `/internal/extension/spec.go` - Types to use
- `/internal/config/root.go` - ExtensibleConfig API
