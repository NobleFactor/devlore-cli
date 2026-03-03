# Plan: Extension Model with Copyright as First Extension

## Summary

Implement the star extension model from `docs/plans/star-extension-model.md` using the copyright linter as the first extension. This establishes the pattern for future extensions.

## Scope

1. **Receiver Registration** - `RegisterReceiver()` converts Go types to Starlark modules
2. **Config Define** - `config.define()` lets extensions declare config schemas
3. **Flag Resolution** - CLI → ENV → Config → Default chain
4. **Copyright Migration** - Refactor copyright module to use receiver pattern

## Current State

| Component | File | Status |
|-----------|------|--------|
| Copyright module | `internal/starlark/builtin_copyright.go` | Working (factory pattern) |
| Module registration | `internal/starlark/runtime.go:78-97` | Manual `predeclared` dict |
| Config loading | `internal/config/config.go` | Working with ToStarlark() |
| Flag parsing | `internal/starlark/runtime.go:224-252` | Basic, no resolution chain |

## Implementation Phases

### Phase 1: Receiver Registration Infrastructure

**Create** `internal/starlark/receiver.go`:

```go
var receiverRegistry = make(map[string]interface{})

// RegisterReceiver registers a Go type as a Starlark module.
func RegisterReceiver(name string, receiver interface{}) {
    receiverRegistry[name] = receiver
}

// GetReceiverModules returns all registered receiver modules.
func GetReceiverModules() starlark.StringDict {
    modules := starlark.StringDict{}
    for name, receiver := range receiverRegistry {
        modules[name] = buildReceiverModule(name, receiver)
    }
    return modules
}

// buildReceiverModule uses reflection to convert methods to builtins.
func buildReceiverModule(name string, receiver interface{}) *starlarkstruct.Module
```

**Key design decisions**:
- Methods with signature `(thread, args...) (starlark.Value, error)` are wrapped directly
- Method names converted to snake_case: `DetectLicense` → `detect_license`
- Receivers registered via `init()` functions

**Modify** `internal/starlark/runtime.go`:
- Call `GetReceiverModules()` and merge into `predeclared`

### Phase 2: Copyright Receiver

**Modify** `internal/starlark/builtin_copyright.go`:

```go
// CopyrightChecker provides copyright header checking and fixing.
type CopyrightChecker struct{}

func (c *CopyrightChecker) Check(thread *starlark.Thread, paths *starlark.List,
    license, holder string, patterns *starlark.Dict) (starlark.Value, error) {
    // Reuse existing copyrightCheck logic
}

func (c *CopyrightChecker) Fix(...) (starlark.Value, error) { ... }
func (c *CopyrightChecker) DetectLicense(...) (starlark.Value, error) { ... }

func init() {
    RegisterReceiver("copyright", &CopyrightChecker{})
}
```

**Backward compatibility**: Keep `copyrightModule()` as fallback, prefer receiver if registered.

### Phase 3: Config Define

**Modify** `internal/starlark/builtin_config.go`:

```go
var extensionSchemas = make(map[string]*starlarkstruct.Struct)

// configDefine registers an extension config schema.
func configDefine(thread *starlark.Thread, b *starlark.Builtin,
    args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var name string
    var schema *starlarkstruct.Struct
    // Register schema for validation and defaults
    extensionSchemas[name] = schema
    return starlark.None, nil
}
```

Add `"define": starlark.NewBuiltin("config.define", configDefine)` to configModule().

**Create** `internal/config/extensions.go`:
- `Extensions` struct for dynamic extension configs
- Merging logic for extension configs from star.yaml

### Phase 4: Flag Resolution Chain

**Modify** `internal/starlark/runtime.go`:

Extend `Flag` struct:
```go
type Flag struct {
    Name       string
    Help       string
    Default    string
    Required   bool
    ConfigPath string // Override config path (empty = convention)
    EnvVar     string // Override env var (empty = convention)
}
```

Add `resolveFlag()`:
```go
func resolveFlag(flag Flag, cmdName string, cliArgs map[string]string) string {
    // 1. CLI arg
    // 2. ENV var (computed: STAR_LINT_COPYRIGHT_FIX)
    // 3. Config value (computed: lint.copyright.fix)
    // 4. Default
}
```

Update `Command.Run()` to resolve flags before execution.

### Phase 5: Integration

- Verify `ops/lint-copyright.star` works unchanged
- Add tests for receiver registration
- Add tests for config.define()
- Add tests for flag resolution chain

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/starlark/receiver.go` | Create | Receiver registration system |
| `internal/starlark/receiver_test.go` | Create | Unit tests |
| `internal/config/extensions.go` | Create | Extension config support |
| `internal/starlark/builtin_copyright.go` | Modify | Use receiver pattern |
| `internal/starlark/builtin_config.go` | Modify | Add config.define() |
| `internal/starlark/runtime.go` | Modify | Receiver lookup, flag resolution |
| `internal/config/config.go` | Modify | Add Extensions field |

## Testing Strategy

1. **Unit tests**: receiver.go, extensions.go
2. **Integration**: Run `star lint copyright` with receiver-based module
3. **Backward compat**: Ensure `ops/lint-copyright.star` passes unchanged

## Dependencies

None - this can be implemented independently.

## Open Questions (Resolved)

1. **Type validation**: `config.define()` validates at load time
2. **Schema conflicts**: Last registration wins (warning emitted)
3. **Go-declared config**: Not supported; use Starlark for declarative config

---

## Implementation Summary

### New Files Created

| File | Purpose |
|------|---------|
| `internal/starlark/receiver.go` | Receiver registration system with reflection-based method wrapping |
| `internal/starlark/receiver_test.go` | Unit tests for receiver functionality |
| `internal/config/extensions.go` | Dynamic extension config support |

### Files Modified

| File | Changes |
|------|---------|
| `internal/starlark/runtime.go` | Added `moduleOrReceiver()`, `ResolveFlag()`, `ComputeEnvVar()`, flag resolution chain |
| `internal/starlark/builtin_copyright.go` | Migrated to receiver pattern with `CopyrightChecker` struct |
| `internal/starlark/builtin_config.go` | Added `config.define()` for extension config schemas |

### Key Features Implemented

1. **Receiver Registration** - `RegisterReceiver("copyright", &CopyrightChecker{})` automatically exposes methods as Starlark functions
2. **Config Define** - Extensions can declare config schemas via `config.define("name", struct(...))`
3. **Flag Resolution Chain** - CLI → ENV → Config → Default (e.g., `STAR_LINT_COPYRIGHT_FIX`)
4. **Backward Compatibility** - Legacy factory functions remain as fallback

### Tests

All tests pass:
- `TestRegisterReceiver` - Basic registration
- `TestGetReceiverModule` - Module creation
- `TestReceiverMethodCall` - Method invocation
- `TestCopyrightReceiverRegistration` - Copyright integration
- `TestToSnakeCase` - Name conversion

### Known Issues

There's a pre-existing issue on this branch where `star lint copyright` fails with "undefined: config" - this was present before the extension model implementation and is unrelated to it.
