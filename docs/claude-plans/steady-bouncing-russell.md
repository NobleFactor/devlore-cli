# Plan: Phase 1 - Core Configuration Infrastructure (Worker 1)

## Summary

Implement the foundational configuration system that supports runtime type generation from YAML extension specs. This creates the base infrastructure for the extension model.

## Context

Worker 1 is responsible for the `internal/config/` package. Phase 1 establishes the configuration hierarchy, runtime type generation, and Starlark adapter—the foundation for all other phases.

### Current State

The existing config package has two parallel systems:
- **Old Config struct** (`config.go`): Monolithic, type-safe at compile time, but can't add sections without code changes
- **New Registry system** (`registry.go`, `loader.go`): Section-based with dot-notation, more flexible but incomplete

Phase 1 creates a unified system with ConfigElement hierarchy and runtime type generation.

## Files to Create

### 1. `internal/config/element.go`
ConfigElement base type embedded by all config sections.

```go
type ConfigElement struct {
    path     string
    children map[string]interface{}
}

func (e *ConfigElement) Path() string
func (e *ConfigElement) Register(name string, child interface{})
func (e *ConfigElement) Get(name string) interface{}
func (e *ConfigElement) Navigate(path string) interface{}
```

Key behaviors:
- `Register()` sets child's path relative to parent
- `Navigate()` traverses hierarchy by dotted path (e.g., "lint.copyright.enabled")
- Handles both `*ConfigElement` and structs embedding `ConfigElement`

### 2. `internal/config/root.go`
Config root with Load/Save and extension registration.

```go
type Config struct {
    ConfigElement                    // path = "", children = top-level sections
    source   string                  // filename, e.g., "star.yaml"
    dirty    bool                    // modified since load
}

type ConfigSpec struct {
    Type     string            // Go type name
    Fields   map[string]string // field name → type (bool, string, int, []string, map[K]V)
    Defaults map[string]any    // default values
}

func Load(source string) (*Config, error)
func (c *Config) Save() error
func (c *Config) RegisterExtension(path string, spec ConfigSpec) error
func (c *Config) Accessor(path string) *ConfigAccessor
func (c *Config) ToStarlark() starlark.Value
```

Key behaviors:
- `Load()` builds hierarchy from extension defaults, then merges star.yaml overrides
- `RegisterExtension()` creates intermediate ConfigElements as needed
- `Save()` writes modified config back to source

### 3. `internal/config/types.go`
Runtime type generation from YAML specs.

```go
func generateConfigType(spec ConfigSpec) reflect.Type
func getOrCreateType(path string, spec ConfigSpec) reflect.Type  // cached
func newConfigInstance(typ reflect.Type, defaults map[string]any) interface{}

// Type cache
var typeCache struct {
    sync.RWMutex
    types map[string]reflect.Type
}
```

Key behaviors:
- Generate Go struct types at runtime using `reflect.StructOf`
- Cache generated types by extension path
- Support primitives, slices, maps, and nested structs
- Embed ConfigElement in generated types

### 4. `internal/config/accessor.go`
ConfigAccessor for typed field access in Go code.

```go
type ConfigAccessor struct {
    v reflect.Value
}

func (a *ConfigAccessor) Bool(name string) bool
func (a *ConfigAccessor) String(name string) string
func (a *ConfigAccessor) Int(name string) int
func (a *ConfigAccessor) StringSlice(name string) []string
func (a *ConfigAccessor) Struct(name string) *ConfigAccessor
```

Key behaviors:
- Wraps reflection in clean typed accessors
- Converts snake_case field names to PascalCase for reflection

## Files to Modify

### 5. `internal/config/starlark.go`
**ADD** ConfigValue implementing `starlark.HasAttrs`. Keep existing ToStarlark() methods.

The current file has type-specific ToStarlark() methods (lines 12-109) for Config, LintConfig, etc. These remain for backward compatibility. We ADD the new generic ConfigValue type.

```go
// NEW: ConfigValue wraps any ConfigElement for Starlark access
type ConfigValue struct {
    elem interface{}  // any ConfigElement or struct embedding ConfigElement
}

// Starlark interface
func (v *ConfigValue) String() string
func (v *ConfigValue) Type() string
func (v *ConfigValue) Freeze()
func (v *ConfigValue) Truth() starlark.Bool
func (v *ConfigValue) Hash() (uint32, error)

// starlark.HasAttrs
func (v *ConfigValue) Attr(name string) (starlark.Value, error)
func (v *ConfigValue) AttrNames() []string

// NEW: Generic conversion for dynamically-typed configs
func WrapAsStarlark(elem interface{}) starlark.Value  // returns ConfigValue
func goToStarlark(v interface{}) (starlark.Value, error)
```

Key behaviors:
- Attribute access via reflection (cfg.lint.copyright.enabled)
- Recursive wrapping for nested ConfigElements
- Proper Go→Starlark type conversion (bool, string, int, []string, map)
- Coexists with existing ToStarlark() methods during migration

## Interface Contracts

### Contract 1: Config Registration (Worker 1 → Worker 3)
```go
type ConfigSpec struct {
    Type     string
    Fields   map[string]string
    Defaults map[string]any
}

func (c *Config) RegisterExtension(path string, spec ConfigSpec) error
func (c *Config) Navigate(path string) interface{}
```

### Contract 2: Starlark Config (Worker 1 → Worker 2)
```go
type ConfigValue struct { elem interface{} }
func (v *ConfigValue) Attr(name string) (starlark.Value, error)
func ToStarlark(elem interface{}) starlark.Value
```

## Implementation Steps

1. **Create `element.go`**
   - Define ConfigElement struct
   - Implement Register(), Get(), Navigate()
   - Add toPascalCase() helper
   - Write tests for hierarchy navigation

2. **Create `types.go`**
   - Implement generateConfigType() with reflect.StructOf
   - Add type cache with sync.RWMutex
   - Implement newConfigInstance() with default population
   - Support nested types (map[string]Pattern)
   - Write tests for type generation

3. **Create `accessor.go`**
   - Implement ConfigAccessor wrapper
   - Add Bool(), String(), Int(), StringSlice(), Struct() methods
   - Write tests for field access

4. **Create `root.go`**
   - Define Config struct embedding ConfigElement
   - Define ConfigSpec struct
   - Implement RegisterExtension()
   - Implement Load() with YAML parsing
   - Implement Save()
   - Write tests for loading/saving

5. **Modify `starlark.go`**
   - Add ConfigValue type
   - Implement starlark.HasAttrs interface
   - Implement goToStarlark() conversion
   - Implement ToStarlark() factory
   - Write tests for Starlark integration

## Dependencies

- None (Phase 1 is independent)
- Uses: `go.starlark.net/starlark`, `gopkg.in/yaml.v3`, `reflect`, `sync`

## Verification

```bash
go test ./internal/config/...
go build ./...
```

## Critical Files

- `internal/config/element.go` - Foundation for config hierarchy
- `internal/config/types.go` - Runtime type generation (uses reflect.StructOf)
- `internal/config/starlark.go` - Boundary crossing for Starlark

## Branch

`feat/ext-config-phase-1`

## Notes

- Existing files (`schema.go`, `value.go`, `registry.go`, etc.) remain unchanged in Phase 1
- Legacy deletion happens in Phase 5 after migration
- ConfigSpec must match what Worker 3 will send from extension YAML parsing
