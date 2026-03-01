# Worker 3: Extension Package - Phase 2 Implementation Plan

## Overview

Worker 3 is responsible for the `internal/extension/` package that parses extension YAML specs, maintains a registry, and discovers extensions from the filesystem.

**Branch:** `feat/ext-extension-phase-2`

## Dependencies

- **Worker 1 (Config):** Phase 1 COMPLETE - `ConfigSpec` type available in `internal/config/root.go`
- **Worker 5 (Wasm):** Working in parallel - define interface stubs for integration

## Files to Create

### 1. `internal/extension/spec.go` - Extension Specification Types

```go
// ExtensionSpec describes an extension parsed from YAML.
type ExtensionSpec struct {
    Extension    string          `yaml:"extension"`    // e.g., "lint.copyright"
    Description  string          `yaml:"description"`
    Receivers    []ReceiverSpec  `yaml:"receivers"`    // 0 or more binding functions
    Capabilities *Capabilities   `yaml:"capabilities"` // sandboxing (nil = no Wasm)
    Command      *CommandSpec    `yaml:"command"`      // optional CLI command
    Flags        []FlagSpec      `yaml:"flags"`
    Config       *ConfigDef      `yaml:"config"`       // optional config schema
}

// ReceiverSpec describes binding functions provided by an extension.
type ReceiverSpec struct {
    Name        string            `yaml:"name"`        // module name (e.g., "copyright")
    Type        string            `yaml:"type"`        // Go type for built-in
    Wasm        string            `yaml:"wasm"`        // .wasm file for external
    Builtin     bool              `yaml:"builtin"`     // true = compiled into binary
    Description string            `yaml:"description"`
    Functions   map[string]string `yaml:"functions"`   // function name → description
}

// CommandSpec describes a CLI subcommand.
type CommandSpec struct {
    Help           string `yaml:"help"`
    Implementation string `yaml:"implementation"` // .star file path
}

// FlagSpec describes a command flag.
type FlagSpec struct {
    Name    string `yaml:"name"`
    Type    string `yaml:"type"`    // bool, string, int, glob
    Default string `yaml:"default"`
    Help    string `yaml:"help"`
}

// ConfigDef describes the config schema (used to generate config.ConfigSpec).
type ConfigDef struct {
    Type     string                 `yaml:"type"`
    Fields   map[string]string      `yaml:"fields"`
    Nested   map[string]ConfigDef   `yaml:"-"` // inline nested types
    Defaults map[string]interface{} `yaml:"defaults"`
}

// Capabilities describes sandboxing for Wasm extensions.
type Capabilities struct {
    FS        FSCapabilities `yaml:"fs"`
    HostCalls []string       `yaml:"host_calls"` // e.g., ["shell.run", "http.get"]
}

type FSCapabilities struct {
    Read  []string `yaml:"read"`
    Write []string `yaml:"write"`
}
```

**Key functions:**
- `ParseSpec(path string) (*ExtensionSpec, error)` - parse YAML file
- `(s *ExtensionSpec) Validate() error` - validate spec has command OR receivers
- `(s *ExtensionSpec) CommandPath() []string` - split "lint.copyright" → ["lint", "copyright"]
- `(s *ExtensionSpec) ToConfigSpec() config.ConfigSpec` - convert ConfigDef to config.ConfigSpec
- `(s *ExtensionSpec) IsBuiltin() bool` - true if all receivers are builtin

### 2. `internal/extension/registry.go` - Extension Registry

```go
// registry is the global extension registry.
var registry = &Registry{
    specs: make(map[string]*ExtensionSpec),
}

type Registry struct {
    mu    sync.RWMutex
    specs map[string]*ExtensionSpec
}

// Register adds an extension to the registry.
func Register(spec *ExtensionSpec) error

// Get returns an extension by name, or nil if not found.
func Get(name string) *ExtensionSpec

// All returns a copy of all registered extensions.
func All() map[string]*ExtensionSpec

// Names returns sorted list of registered extension names.
func Names() []string
```

### 3. `internal/extension/discovery.go` - Extension Discovery

```go
// Discover scans a directory for extension.yaml files.
func Discover(dir string) ([]*ExtensionSpec, error)

// LoadAll discovers and registers extensions from directories.
// Searches for "extension.yaml" or "*.yaml" with "extension:" key.
func LoadAll(dirs ...string) error

// DefaultSearchPaths returns standard extension search paths.
func DefaultSearchPaths() []string
```

**Discovery logic:**
1. Walk directory tree looking for `extension.yaml` files
2. Also check for `*.yaml` files that start with `extension:` key
3. Parse each found spec and validate
4. Return list of valid specs (log warnings for invalid)

### 4. `internal/extension/wasm.go` - Wasm Integration Stub

```go
// WasmHost interface for Worker 5 integration.
// Worker 5 will implement this interface in internal/wasm/host.go
type WasmHost interface {
    LoadModule(wasmPath string) (WasmModule, error)
    Call(module WasmModule, function string, args []byte) ([]byte, error)
    Close() error
}

type WasmModule interface {
    Name() string
    Functions() []string
}

// wasmHost is set by internal/wasm package during init.
var wasmHost WasmHost

// SetWasmHost registers the Wasm runtime (called by internal/wasm).
func SetWasmHost(host WasmHost)

// GetWasmHost returns the registered Wasm host.
func GetWasmHost() WasmHost

// HasWasmSupport returns true if a Wasm runtime is available.
func HasWasmSupport() bool
```

## Integration with Config Package

When registering an extension with config:

```go
func RegisterWithConfig(spec *ExtensionSpec, cfg *config.ExtensibleConfig) error {
    if spec.Config == nil {
        return nil
    }

    configSpec := spec.ToConfigSpec()
    return cfg.RegisterExtension(spec.Extension, configSpec)
}
```

## Validation Rules

An extension MUST have at least one of:
- Non-empty `receivers` list (binding functions)
- Non-nil `command` (CLI subcommand)

Extensions with Wasm receivers must have:
- `capabilities` defined
- Valid `.wasm` file path in receiver spec

## Test Plan

1. **spec_test.go:**
   - Parse valid extension YAML
   - Parse extension with only receivers (binding-only)
   - Parse extension with only command (command-only)
   - Parse full extension (receivers + command)
   - Validate rejects empty extension
   - ToConfigSpec converts correctly

2. **registry_test.go:**
   - Register and Get
   - All returns copy
   - Concurrent access safety

3. **discovery_test.go:**
   - Discover finds extension.yaml
   - Discover ignores invalid YAML
   - LoadAll registers found extensions

## File Paths

**Create:**
- `internal/extension/spec.go`
- `internal/extension/spec_test.go`
- `internal/extension/registry.go`
- `internal/extension/registry_test.go`
- `internal/extension/discovery.go`
- `internal/extension/discovery_test.go`
- `internal/extension/wasm.go`
- `internal/extension/doc.go` (package documentation)

**Reference (read-only):**
- `internal/config/root.go` - ConfigSpec type
- `internal/starlark/receiver.go` - RegisterReceiver pattern
- `docs/architecture/devlore-extension-model.md` - Extension specification

## Verification

```bash
# Run tests
go test ./internal/extension/...

# Build verification
go build ./...

# Verify no import cycles
go build ./cmd/star/...
```
