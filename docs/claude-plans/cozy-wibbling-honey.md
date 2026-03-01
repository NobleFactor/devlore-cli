# Plan: Star Agent Team Refactoring

---
title: "Star Agent Team Refactoring"
description: "Multi-agent implementation of the star extension model"
status: draft
created: 2025-02-08
updated: 2025-02-08
---

## Summary

Implement the full star extension model using a team of Claude Code agents working on separate packages. Each agent owns specific files and packages, with the Lead coordinating integration.

## Decisions

1. **Scope:** Full extension model (all 6 phases)
2. **Distill:** Rename of "knowledge" command - codebase knowledge extraction for LLMs
3. **Coordination:** Separate branches per package with strict file boundaries
4. **Extension package:** `internal/extension/`

---

## Extension Model

An extension can define **zero or more commands** and **zero or more binding functions**. All binding functions are defined using the receiver API.

### Extension Distribution

Extensions are distributed as **WebAssembly modules** for cross-platform compatibility:
- Single `.wasm` artifact per extension (no platform-specific binaries)
- Sandboxed execution via WASI (WebAssembly System Interface)
- Host callbacks for privileged operations (shell, network)
- Runtime: [wazero](https://wazero.io/) (pure Go, no CGO)

### Extension Types

| Type | Commands | Bindings | Example |
|------|----------|----------|---------|
| Command-only | 1+ | 0 | `lint.all` - orchestrates other linters |
| Binding-only | 0 | 1+ | `copyright` - provides primitives for other extensions |
| Full | 1+ | 1+ | `lint.copyright` - has command AND binding functions |
| Built-in | varies | varies | Core extensions compiled into star binary |

### Extension Packaging Structure

```
star/
├── cmd/star/                    # CLI entry point
│   └── main.go
│
├── internal/
│   ├── config/                  # Configuration system
│   │   ├── element.go          # ConfigElement base type
│   │   ├── root.go             # Config root
│   │   ├── types.go            # Runtime type generation
│   │   ├── accessor.go         # Typed field access
│   │   └── starlark.go         # Starlark adapter
│   │
│   ├── extension/               # Extension loading (NEW)
│   │   ├── spec.go             # ExtensionSpec types
│   │   ├── registry.go         # Extension registry
│   │   ├── discovery.go        # Directory scanning
│   │   └── wasm.go             # Wasm runtime integration
│   │
│   ├── wasm/                    # Wasm host runtime (NEW)
│   │   ├── host.go             # wazero host setup
│   │   ├── callbacks.go        # Host callback implementations
│   │   └── capabilities.go     # Capability validation
│   │
│   ├── starlark/                # Starlark runtime
│   │   ├── runtime.go          # Script execution
│   │   ├── receiver.go         # Receiver API
│   │   ├── command.go          # Command types
│   │   └── builtin_*.go        # Built-in binding functions
│   │
│   └── cli/                     # Output formatting
│
├── extensions/                   # Extension packages (NEW)
│   ├── lint-copyright/
│   │   ├── extension.yaml      # Metadata + capabilities
│   │   ├── lint-copyright.wasm # Compiled binding functions
│   │   └── lint-copyright.star # Starlark command implementation
│   │
│   ├── lint-go/
│   │   ├── extension.yaml
│   │   └── lint-go.star        # Command-only (uses built-in receivers)
│   │
│   └── ...
│
└── ops/                          # Legacy location (migrating to extensions/)
```

### Extension Specification Format

```yaml
# extensions/lint-copyright/extension.yaml
extension: lint.copyright
description: "Check or fix copyright headers in source files"

# Receivers this extension provides (binding functions)
# Can be built-in (compiled into star) or wasm (loaded at runtime)
receivers:
  - name: copyright
    wasm: lint-copyright.wasm    # Wasm module providing bindings
    functions:
      - check: "Verify files have correct headers"
      - fix: "Add or update headers"
      - detect_license: "Detect SPDX from LICENSE file"

# Capabilities required by this extension (sandboxing)
capabilities:
  fs:
    read: ["/workspace"]         # Read project files
    write: ["/workspace"]        # Write fixes
  host_calls: []                 # No external tool calls needed

# Command this extension provides
command:
  help: |
    Check or fix copyright headers.
  implementation: lint-copyright.star

flags:
  - name: fix
    type: bool
    default: false
    help: Add missing headers

# Configuration schema
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

### Built-in Extension Example

Core extensions compiled into the star binary (no Wasm):

```yaml
# extensions/lint-go/extension.yaml
extension: lint.go
description: "Run Go linters via golangci-lint"

receivers:
  - name: go
    builtin: true                # Compiled into star binary
    type: GoRunner               # Go type in internal/starlark/

capabilities:
  fs:
    read: ["/workspace"]
  host_calls:
    - shell.run                  # Needs to run golangci-lint

command:
  help: "Run Go linters"
  implementation: lint-go.star
```

### Third-Party Extension Example (Wasm)

```yaml
# extensions/custom-linter/extension.yaml
extension: custom.linter
description: "Custom code analysis"

receivers:
  - name: customlint
    wasm: customlint.wasm
    functions:
      - analyze
      - report

capabilities:
  fs:
    read: ["/workspace"]
    write: []                    # Read-only
  host_calls: []                 # Pure analysis, no external tools

command:
  help: "Run custom analysis"
  implementation: custom-linter.star
```

### Sandboxing Model

Extensions run in a WASI sandbox with declared capabilities:

| Capability | Description | Granted By |
|------------|-------------|------------|
| `fs.read` | Read files in listed directories | Host validates paths |
| `fs.write` | Write files in listed directories | Host validates paths |
| `host_calls.shell.run` | Execute shell commands | Host executes, returns result |
| `host_calls.http.get` | Make HTTP requests | Host executes, returns result |

**Host Callback Protocol:**
```
Extension (Wasm) → Host Request → Host Validates → Host Executes → Result
```

Extensions cannot:
- Access files outside granted directories
- Spawn processes directly
- Make network requests directly
- Access environment variables not explicitly passed

---

## Package Structure

| Package | Location | Purpose | Owner |
|---------|----------|---------|-------|
| `cmd/star` | `cmd/star/` | CLI entry point, command discovery | Lead |
| `config` | `internal/config/` | Configuration hierarchy, type generation | Worker 1 |
| `starlark` | `internal/starlark/` | Starlark runtime, receiver API, builtins | Worker 2 |
| `extension` | `internal/extension/` | Extension spec parsing, registry, discovery | Worker 3 |
| `wasm` | `internal/wasm/` | Wasm host runtime, host callbacks, capabilities | Worker 5 |
| `cli` | `internal/cli/` | Output formatting | (unchanged) |

### Command Areas

- **config** - Configuration management commands
- **distill** - Codebase knowledge extraction for LLMs (rename from "knowledge")
- **lint** - Quality gate commands (go, shell, markdown, copyright, starlark, all)
- **self** - Self-install, upgrade, version commands

---

## Agent Team

### Lead Agent
**Branch:** `feat/ext-lead-integration`
**Owned Files:**
- `cmd/star/main.go` - Entry point modifications
- `cmd/star/config.go` - Configuration initialization
- Integration tests in `tests/integration/`

**Responsibilities:**
- Coordinate interface contracts between agents
- Review PRs from all workers before merging
- Manage merge order and resolve conflicts
- Run integration tests after each phase

### Worker 1: Config Package
**Branch:** `feat/ext-config-phase-{N}`
**Owned Files (exclusive):**
- `internal/config/element.go` (NEW) - ConfigElement base type
- `internal/config/root.go` (NEW) - Config root with Load/Save
- `internal/config/types.go` (NEW) - Runtime type generation
- `internal/config/accessor.go` (NEW) - Typed field access
- `internal/config/starlark.go` (MODIFY) - ConfigValue adapter
- Legacy files (DELETE in Phase 5): `schema.go`, `value.go`, `registry.go`, `loader.go`, `extensions.go`

### Worker 2: Starlark Package
**Branch:** `feat/ext-starlark-phase-{N}`
**Owned Files (exclusive):**
- `internal/starlark/runtime.go` (MODIFY)
- `internal/starlark/builtin_config.go` (MODIFY)
- `internal/starlark/command.go` (NEW) - Extracted from runtime.go
- `internal/starlark/receiver.go` (MODIFY)
- `internal/starlark/builtin_*.go` (MODIFY for receiver migration)

### Worker 3: Extension Package
**Branch:** `feat/ext-extension-phase-{N}`
**Owned Files (exclusive):**
- `internal/extension/spec.go` (NEW) - ExtensionSpec struct
- `internal/extension/registry.go` (NEW) - Extension registry
- `internal/extension/discovery.go` (NEW) - Directory scanning

### Worker 4: Extensions and Commands
**Branch:** `feat/ext-commands-phase-{N}`
**Owned Files (exclusive):**
- `extensions/*/extension.yaml` (NEW) - All extension specs
- `extensions/*/*.star` (NEW) - Starlark implementations
- `ops/*.star` (MODIFY) - Update to extension pattern
- `docs/guides/writing-extensions.md` (NEW)

### Worker 5: Wasm Runtime
**Branch:** `feat/ext-wasm-phase-{N}`
**Owned Files (exclusive):**
- `internal/wasm/host.go` (NEW) - wazero host setup
- `internal/wasm/callbacks.go` (NEW) - Host callback implementations (shell.run, http.get)
- `internal/wasm/capabilities.go` (NEW) - Capability validation and enforcement
- `internal/wasm/protocol.go` (NEW) - Extension ↔ Host communication protocol

---

## Interface Contracts

### Contract 1: Config Registration (Worker 3 → Worker 1)
```go
// Worker 1 implements, Worker 3 consumes
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
// Worker 1 implements, Worker 2 consumes
type ConfigValue struct { elem interface{} }
func (v *ConfigValue) Attr(name string) (starlark.Value, error)
func ToStarlark(elem interface{}) starlark.Value
```

### Contract 3: Extension Spec (Worker 3 → Worker 4)
```go
// Worker 3 implements, Worker 4 uses for YAML validation
type ExtensionSpec struct {
    Extension string      `yaml:"extension"`
    Command   CommandSpec `yaml:"command"`
    Flags     []FlagSpec  `yaml:"flags"`
    Config    ConfigSpec  `yaml:"config"`
}
func ParseSpec(yamlPath string) (*ExtensionSpec, error)
```

### Contract 4: Runtime Extension Loading (Worker 3 → Worker 2)
```go
// Worker 3 implements, Worker 2 consumes
func All() map[string]*ExtensionSpec
func Get(name string) *ExtensionSpec
```

### Contract 5: Wasm Host Runtime (Worker 5 → Worker 3)
```go
// Worker 5 implements, Worker 3 consumes
type WasmHost struct { ... }

func NewHost(capabilities Capabilities) (*WasmHost, error)
func (h *WasmHost) LoadModule(wasmPath string) (*WasmModule, error)
func (h *WasmHost) Call(module *WasmModule, function string, args []byte) ([]byte, error)
func (h *WasmHost) Close() error
```

### Contract 6: Host Callbacks (Worker 5 implements)
```go
// Host callback functions available to Wasm extensions
type HostCallbacks interface {
    ShellRun(cmd string, args []string, dir string) (stdout, stderr string, exitCode int, err error)
    HTTPGet(url string, headers map[string]string) (body []byte, statusCode int, err error)
    FSRead(path string) ([]byte, error)   // Within granted paths
    FSWrite(path string, data []byte) error // Within granted paths
}
```

### Contract 7: Capabilities (Worker 5 implements, Worker 3 validates)
```go
type Capabilities struct {
    FS        FSCapabilities        `yaml:"fs"`
    HostCalls []string              `yaml:"host_calls"`
}

type FSCapabilities struct {
    Read  []string `yaml:"read"`   // Directories extension can read
    Write []string `yaml:"write"`  // Directories extension can write
}

func (c *Capabilities) Validate() error
func (c *Capabilities) AllowsRead(path string) bool
func (c *Capabilities) AllowsWrite(path string) bool
func (c *Capabilities) AllowsHostCall(name string) bool
```

---

## Phase Execution

```
Phase 1: Core Configuration Infrastructure
         └── Worker 1 only (no dependencies)

Phase 2: Extension Registration + Wasm Runtime
         ├── Worker 3 (depends on Worker 1's ConfigSpec)
         └── Worker 5 (Wasm host, can work in parallel)

Phase 3: Migrate Commands to Extensions
         ├── Worker 4 (depends on Worker 3's spec format)
         └── Worker 2 starts command.go extraction

Phase 4: Update Runtime + Wasm Integration
         ├── Worker 2 (depends on Workers 1, 3)
         ├── Worker 5 (host callbacks integration)
         └── Lead: Integration testing begins

Phase 5: Remove Legacy Code
         ├── Worker 1 (cleanup)
         └── Lead: Verify no regressions

Phase 6: Documentation and Testing
         ├── Worker 4 (documentation)
         └── All: Final testing
```

### Merge Order per Phase

| Phase | Order | Branch | Agent |
|-------|-------|--------|-------|
| 1 | 1 | `feat/ext-config-phase-1` | Worker 1 |
| 2 | 1 | `feat/ext-extension-phase-2` | Worker 3 |
| 2 | 2 | `feat/ext-wasm-phase-2` | Worker 5 |
| 3 | 1 | `feat/ext-commands-phase-3` | Worker 4 |
| 3 | 2 | `feat/ext-starlark-phase-3` | Worker 2 |
| 4 | 1 | `feat/ext-wasm-phase-4` | Worker 5 |
| 4 | 2 | `feat/ext-starlark-phase-4` | Worker 2 |
| 4 | 3 | `feat/ext-lead-integration` | Lead |
| 5 | 1 | `feat/ext-config-phase-5` | Worker 1 |
| 6 | 1 | `feat/ext-commands-phase-6` | Worker 4 |

---

## Phase Details

### Phase 1: Core Configuration Infrastructure (Worker 1)

**Create:**
- `internal/config/element.go` - ConfigElement with path, children, Navigate()
- `internal/config/root.go` - Config root with Load(), Save(), RegisterExtension()
- `internal/config/types.go` - generateConfigType() via reflect.StructOf
- `internal/config/accessor.go` - ConfigAccessor with Bool(), String(), Int()

**Modify:**
- `internal/config/starlark.go` - Add ConfigValue implementing starlark.HasAttrs

### Phase 2: Extension Registration (Worker 3) + Wasm Runtime (Worker 5)

**Worker 3 Creates:**
- `internal/extension/spec.go` - ExtensionSpec, ReceiverSpec, CommandSpec, FlagSpec, ConfigSpec
- `internal/extension/registry.go` - Register(), Get(), All()
- `internal/extension/discovery.go` - Discover(), LoadAll()
- `internal/extension/wasm.go` - Integration with wasm.Host

**Worker 5 Creates:**
- `internal/wasm/host.go` - wazero host setup, module loading
- `internal/wasm/capabilities.go` - Capability validation, path checking
- `internal/wasm/protocol.go` - Extension ↔ Host message format

### Phase 3: Migrate Commands (Worker 4 + Worker 2)

**Worker 4 Creates:**
- `extensions/lint-go.yaml`
- `extensions/lint-shell.yaml`
- `extensions/lint-copyright.yaml`
- `extensions/lint-all.yaml`
- `extensions/distill.yaml`
- `extensions/self.yaml`

**Worker 2 Creates:**
- `internal/starlark/command.go` - Extract Command, Flag types

### Phase 4: Update Runtime (Worker 2 + Worker 5 + Lead)

**Worker 2 Modifies:**
- `runtime.go` - Load extensions before Starlark files
- `builtin_config.go` - Use new Config system

**Worker 5 Creates:**
- `internal/wasm/callbacks.go` - Host callback implementations (shell.run, http.get, fs.*)

**Lead Modifies:**
- `cmd/star/main.go` - Extension discovery and loading

### Phase 5: Remove Legacy Code (Worker 1)

**Delete:** `schema.go`, `value.go`, `registry.go`, `loader.go`, `extensions.go`, `config.go`

### Phase 6: Documentation (Worker 4)

**Create:**
- `docs/guides/writing-extensions.md`
- `docs/guides/config-migration.md`

---

## Verification

### Worker Self-Verification
```bash
# All workers run before PR
go test ./internal/{package}/...
go build ./...
```

### Integration Verification (Lead)
```bash
go test ./...
./star lint all
./star lint copyright --fix
./star setup config
```

---

## Critical Files

1. `internal/config/element.go` - Foundation for config hierarchy
2. `internal/extension/spec.go` - Extension YAML structure with Wasm support
3. `internal/wasm/host.go` - wazero runtime, sandboxed execution
4. `internal/wasm/capabilities.go` - Security boundary enforcement
5. `internal/starlark/runtime.go` - Integration point
6. `cmd/star/main.go` - Entry point changes

---

## Agent Prompts

These prompts provide each agent with the context needed to start work independently. All agents should read the architecture documents before beginning.

### Lead Agent Prompt

```
You are the Lead Agent for the star extension model refactoring.

## Your Role
- Coordinate interface contracts between workers
- Review and merge PRs from all workers
- Run integration tests after each phase
- Own cmd/star/main.go and integration tests

## Key Documents
Read these before starting:
- docs/architecture/devlore-extension-model.md (canonical architecture)
- docs/plans/star-extension-model.md (implementation phases)
- docs/plans/star-agent-team-refactor.md (this plan)

## Your Files (exclusive ownership)
- cmd/star/main.go
- cmd/star/config.go
- tests/integration/

## Branch
feat/ext-lead-integration

## Phase Responsibilities
- Phase 4: Modify cmd/star/main.go for extension discovery
- All phases: Coordinate merges, run integration tests

## Success Criteria
All existing commands (lint go, lint shell, lint copyright, etc.) work
after refactoring. No regressions.

## Interface Oversight
You approve all interface contract changes. Workers propose, you decide.
```

### Worker 1: Config Package Prompt

```
You are Worker 1, responsible for the config package.

## Your Role
Build the configuration infrastructure that supports runtime type generation
from YAML extension specs.

## Key Documents
Read these before starting:
- docs/architecture/devlore-extension-model.md
  - Section: "Configuration Architecture" (ConfigElement, Config root)
  - Section: "Runtime Type Generation" (reflect.StructOf)
  - Section: "ConfigValue (Starlark Adapter)"
- docs/plans/star-agent-team-refactor.md (this plan)

## Your Files (exclusive ownership)
Create:
- internal/config/element.go - ConfigElement base type with path, children, Navigate()
- internal/config/root.go - Config root with Load(), Save(), RegisterExtension()
- internal/config/types.go - generateConfigType() via reflect.StructOf
- internal/config/accessor.go - ConfigAccessor with Bool(), String(), Int()

Modify:
- internal/config/starlark.go - Add ConfigValue implementing starlark.HasAttrs

Delete (Phase 5 only):
- internal/config/schema.go
- internal/config/value.go
- internal/config/registry.go
- internal/config/loader.go
- internal/config/extensions.go

## Branch
feat/ext-config-phase-{N}

## Interface Contract (you implement, Worker 3 consumes)
```go
type ConfigSpec struct {
    Type     string
    Fields   map[string]string
    Defaults map[string]any
}

func (c *Config) RegisterExtension(path string, spec ConfigSpec) error
func (c *Config) Navigate(path string) interface{}
```

## Interface Contract (you implement, Worker 2 consumes)
```go
type ConfigValue struct { elem interface{} }
func (v *ConfigValue) Attr(name string) (starlark.Value, error)
func ToStarlark(elem interface{}) starlark.Value
```

## Phase 1 Deliverables
- ConfigElement with hierarchical navigation
- Config root that can register extensions at dotted paths
- Runtime type generation from ConfigSpec
- ConfigValue Starlark adapter

## Verification
go test ./internal/config/...
go build ./...
```

### Worker 2: Starlark Package Prompt

```
You are Worker 2, responsible for the starlark package.

## Your Role
Integrate the extension system with the Starlark runtime. Update runtime
to load extensions and use the new config system.

## Key Documents
Read these before starting:
- docs/architecture/devlore-extension-model.md
  - Section: "Binding Functions (Go)" (receiver pattern)
  - Section: "Receiver Registration"
  - Section: "Command (Starlark)"
- docs/plans/star-agent-team-refactor.md (this plan)

## Your Files (exclusive ownership)
Create:
- internal/starlark/command.go - Extract Command, Flag types from runtime.go

Modify:
- internal/starlark/runtime.go - Load extensions before Starlark files
- internal/starlark/builtin_config.go - Use new Config system
- internal/starlark/receiver.go - Ensure receiver API supports extension specs
- internal/starlark/builtin_*.go - Migrate to receiver pattern as needed

## Branch
feat/ext-starlark-phase-{N}

## Interface Contract (Worker 1 implements, you consume)
```go
type ConfigValue struct { elem interface{} }
func (v *ConfigValue) Attr(name string) (starlark.Value, error)
func ToStarlark(elem interface{}) starlark.Value
```

## Interface Contract (Worker 3 implements, you consume)
```go
func extension.All() map[string]*ExtensionSpec
func extension.Get(name string) *ExtensionSpec
```

## Key Concept: Receiver API
All binding functions are defined using the receiver API. A receiver is a
Go struct whose exported methods become Starlark module functions.

```go
type CopyrightChecker struct{}

func (c *CopyrightChecker) Check(...) (starlark.Value, error) { ... }

func init() {
    RegisterReceiver("copyright", &CopyrightChecker{})
}
```

Methods are exposed as copyright.check() in Starlark.

## Phase 3 Deliverables
- Extract Command, Flag types to command.go

## Phase 4 Deliverables
- Runtime loads extensions before Starlark files
- Flag resolution uses extension spec defaults
- builtin_config.go uses new Config system

## Verification
go test ./internal/starlark/...
go build ./...
# Run existing commands to verify no regression
```

### Worker 3: Extension Package Prompt

```
You are Worker 3, responsible for the extension package.

## Your Role
Build the extension loading system that parses YAML specs and registers
them with the config and runtime systems.

## Key Documents
Read these before starting:
- docs/architecture/devlore-extension-model.md
  - Section: "Extension Specification" (YAML format)
  - Section: "Registration from YAML"
- docs/plans/star-agent-team-refactor.md
  - Section: "Extension Model" (extension types)
  - Section: "Extension Specification Format"

## Your Files (exclusive ownership)
Create:
- internal/extension/spec.go - ExtensionSpec, ReceiverSpec, CommandSpec, FlagSpec, ConfigSpec
- internal/extension/registry.go - Register(), Get(), All()
- internal/extension/discovery.go - Discover(), LoadAll()

## Branch
feat/ext-extension-phase-{N}

## Interface Contract (you implement, Worker 2 consumes)
```go
func All() map[string]*ExtensionSpec
func Get(name string) *ExtensionSpec
```

## Interface Contract (Worker 1 implements, you consume)
```go
func (c *Config) RegisterExtension(path string, spec ConfigSpec) error
```

## Key Types
```go
type ExtensionSpec struct {
    Extension   string         `yaml:"extension"`
    Description string         `yaml:"description"`
    Receivers   []ReceiverSpec `yaml:"receivers"`  // 0 or more
    Command     *CommandSpec   `yaml:"command"`    // optional
    Flags       []FlagSpec     `yaml:"flags"`
    Config      *ConfigSpec    `yaml:"config"`     // optional
}

type ReceiverSpec struct {
    Name        string   `yaml:"name"`
    Type        string   `yaml:"type"`        // Go type name
    Description string   `yaml:"description"`
    Functions   []string `yaml:"functions"`   // exposed methods
}

type CommandSpec struct {
    Help string `yaml:"help"`
}

type FlagSpec struct {
    Name    string `yaml:"name"`
    Type    string `yaml:"type"`
    Default string `yaml:"default"`
    Help    string `yaml:"help"`
}
```

## Extension Types
An extension MUST have at least one of:
- receivers (binding functions via receiver API)
- command (CLI subcommand)

Three patterns:
1. Binding-only: receivers but no command
2. Command-only: command but no receivers
3. Full: both receivers and command

## Phase 2 Deliverables
- Parse extension YAML specs
- Registry with Register/Get/All
- Discovery from extensions/ directory
- Integration with config.RegisterExtension()

## Verification
go test ./internal/extension/...
go build ./...
```

### Worker 4: Extensions and Commands Prompt

```
You are Worker 4, responsible for extension specs and Starlark commands.

## Your Role
Create extension YAML specs for existing commands. Update Starlark
implementations to work with the extension system.

## Key Documents
Read these before starting:
- docs/architecture/devlore-extension-model.md
  - Section: "Extension Specification" (canonical YAML format)
  - Section: "Command (Starlark)" (command implementation)
- docs/plans/star-agent-team-refactor.md
  - Section: "Extension Specification Format"
  - Section: "Extension Types"

## Your Files (exclusive ownership)
Create:
- extensions/lint-go.yaml
- extensions/lint-shell.yaml
- extensions/lint-markdown.yaml
- extensions/lint-copyright.yaml
- extensions/lint-all.yaml
- extensions/distill.yaml (rename from knowledge)
- extensions/self.yaml
- extensions/config.yaml
- docs/guides/writing-extensions.md

Modify:
- ops/*.star - Update to work with extension system

## Branch
feat/ext-commands-phase-{N}

## Extension Spec Template
```yaml
extension: lint.copyright
description: "Check or fix copyright headers in source files"

# OPTIONAL: Receivers (binding functions via receiver API)
receivers:
  - name: copyright
    type: CopyrightChecker
    description: "Copyright header primitives"
    functions:
      - check
      - fix
      - detect_license

# OPTIONAL: Command (CLI subcommand)
command:
  help: |
    Check or fix copyright headers.

    Examples:
      star lint copyright
      star lint copyright --fix

# Flags for the command
flags:
  - name: fix
    type: bool
    default: "false"
    help: Add missing headers

# OPTIONAL: Configuration schema
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

## Existing Implementations to Package
Review these files to understand what needs extension specs:
- internal/starlark/builtin_copyright.go → extensions/lint-copyright.yaml
- internal/starlark/builtin_lint.go → extensions/lint-*.yaml
- internal/starlark/builtin_go.go → extensions/lint-go.yaml (uses go receiver)
- ops/devlore-registry/build-knowledge.star → extensions/distill.yaml

## Phase 3 Deliverables
- Extension YAML specs for all existing commands
- Updated ops/*.star files

## Phase 6 Deliverables
- docs/guides/writing-extensions.md
- docs/guides/config-migration.md

## Verification
yamllint extensions/*.yaml
go build ./...
# Run each command to verify
```

### Worker 5: Wasm Runtime Prompt

```
You are Worker 5, responsible for the Wasm runtime package.

## Your Role
Build the WebAssembly host runtime that loads and executes extension
Wasm modules with sandboxed capabilities.

## Key Documents
Read these before starting:
- docs/plans/star-agent-team-refactor.md
  - Section: "Extension Distribution" (Wasm model)
  - Section: "Sandboxing Model" (capabilities)
- https://wazero.io/docs/ (wazero documentation)
- https://github.com/aspect-build/rules_ts/tree/main/examples (for Wasm patterns)

## Your Files (exclusive ownership)
Create:
- internal/wasm/host.go - wazero runtime setup, module compilation/caching
- internal/wasm/capabilities.go - Capability struct, path validation
- internal/wasm/protocol.go - Message format for extension ↔ host communication
- internal/wasm/callbacks.go - Host callback implementations

## Branch
feat/ext-wasm-phase-{N}

## Interface Contract (you implement, Worker 3 consumes)
```go
type WasmHost struct {
    runtime wazero.Runtime
    config  wazero.ModuleConfig
}

func NewHost(capabilities Capabilities) (*WasmHost, error)
func (h *WasmHost) LoadModule(wasmPath string) (*WasmModule, error)
func (h *WasmHost) Call(module *WasmModule, function string, args []byte) ([]byte, error)
func (h *WasmHost) Close() error
```

## Capabilities System
```go
type Capabilities struct {
    FS        FSCapabilities `yaml:"fs"`
    HostCalls []string       `yaml:"host_calls"`
}

type FSCapabilities struct {
    Read  []string `yaml:"read"`
    Write []string `yaml:"write"`
}

// Validation
func (c *Capabilities) AllowsRead(path string) bool
func (c *Capabilities) AllowsWrite(path string) bool
func (c *Capabilities) AllowsHostCall(name string) bool
```

## Host Callbacks
Extensions call back to host for privileged operations:
```go
type HostCallbacks interface {
    // shell.run - Execute shell command
    ShellRun(cmd string, args []string, dir string) (stdout, stderr string, exitCode int, err error)

    // http.get - Make HTTP request
    HTTPGet(url string, headers map[string]string) (body []byte, statusCode int, err error)

    // fs.read - Read file (within granted paths)
    FSRead(path string) ([]byte, error)

    // fs.write - Write file (within granted paths)
    FSWrite(path string, data []byte) error
}
```

## wazero Integration
Use wazero (pure Go, no CGO):
```go
import "github.com/tetratelabs/wazero"
import "github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

func NewHost(caps Capabilities) (*WasmHost, error) {
    ctx := context.Background()

    runtime := wazero.NewRuntime(ctx)
    wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

    // Configure WASI with preopened directories based on capabilities
    config := wazero.NewModuleConfig().
        WithStdout(os.Stdout).
        WithStderr(os.Stderr)

    for _, dir := range caps.FS.Read {
        config = config.WithFSConfig(
            wazero.NewFSConfig().WithReadOnlyDirMount(dir, dir))
    }

    return &WasmHost{runtime: runtime, config: config}, nil
}
```

## Phase 2 Deliverables
- wazero host setup
- Capabilities validation
- Module loading and caching
- Basic protocol for function calls

## Phase 4 Deliverables
- Host callback implementations (shell.run, http.get, fs.*)
- Integration tests with sample Wasm modules

## Verification
go test ./internal/wasm/...
go build ./...
# Test with a simple Wasm module
```

---

## Architecture Document Updates

Update `docs/architecture/devlore-extension-model.md` to clarify:

### Change 1: Extension Components (Section "Extension Components")

**Current:**
> An extension provides three components:
> | Component | Language | Purpose |
> | Binding Functions | Go | Low-level primitives |
> | Config Schema | Starlark | Typed configuration |
> | Command | Starlark | CLI subcommand |

**Updated:**
> An extension provides **zero or more** of these components:
> | Component | Required | Language | Purpose |
> |-----------|----------|----------|---------|
> | Binding Functions | Optional | Go | Low-level primitives via receiver API |
> | Config Schema | Optional | YAML | Typed configuration with defaults |
> | Command | Optional | Starlark | CLI subcommand implementation |
>
> **Minimum requirement:** An extension must provide at least one binding function OR one command.

### Change 2: Add "Extension Types" Section (after Extension Specification)

Add new section:
```markdown
## Extension Types

Extensions fall into three categories based on what they provide:

### Binding-Only Extension
Provides primitives for other extensions to use. No CLI command.

```yaml
extension: copyright
description: "Copyright header checking primitives"

receivers:
  - name: copyright
    type: CopyrightChecker
    functions: [check, fix, detect_license]
```

Usage in other extensions:
```python
# Another extension can use these bindings
result = copyright.check(paths=files, license="MIT")
```

### Command-Only Extension
Orchestrates existing bindings. No new Go code.

```yaml
extension: lint.all
description: "Run all configured linters"

command:
  help: "Run all linters"
```

### Full Extension
Provides both bindings and a command.

```yaml
extension: lint.copyright
description: "Check or fix copyright headers"

receivers:
  - name: copyright
    type: CopyrightChecker

command:
  help: "Check or fix copyright headers"
```
```

### Change 3: Receiver API Clarification (Section "Binding Functions")

Add at the start of section:
> All binding functions are defined using the receiver API. A receiver is a Go struct whose exported methods become Starlark module functions. The receiver pattern is the **only** way to expose Go functions to Starlark.

### Change 4: Add "Extension Distribution" Section

Add new section after "Extension Types":
```markdown
## Extension Distribution

Extensions are distributed as WebAssembly modules for cross-platform compatibility.

### Why WebAssembly?
- **Single artifact**: One `.wasm` file works on all platforms (no per-OS binaries)
- **Sandboxed**: Extensions can only access resources explicitly granted
- **No CGO**: Uses wazero, a pure Go Wasm runtime
- **Third-party safe**: Untrusted extensions can't escape sandbox

### Built-in vs External Extensions
| Type | Distribution | Bindings | Performance |
|------|--------------|----------|-------------|
| Built-in | Compiled into star binary | Go code in internal/starlark/ | Native |
| External | .wasm file in extension package | Wasm module | ~2-5x slower |

### Sandboxing Model
Extensions declare required capabilities in extension.yaml:
```yaml
capabilities:
  fs:
    read: ["/workspace"]    # Directories extension can read
    write: ["/workspace"]   # Directories extension can write
  host_calls:
    - shell.run             # Can request host to run commands
    - http.get              # Can request host to make HTTP calls
```

Host validates all capability requests. Extensions cannot:
- Access files outside granted directories
- Spawn processes directly
- Make network requests directly
- Access environment variables not explicitly passed

### Host Callback Protocol
For privileged operations, extensions call back to the host:
```
Extension (Wasm) → Request(shell.run, args) → Host validates → Host executes → Result
```
```

---

## Acceptance Criteria

All existing commands must be functional after refactoring:
- `star lint go`
- `star lint shell`
- `star lint markdown`
- `star lint copyright`
- `star lint copyright --fix`
- `star lint all`
- `star setup config`
- `star hook pre-commit`

Each command should work identically to its current behavior. The refactoring packages existing implementations as extensions without changing functionality.

---

## Related Documents

- [Star Extension Model](./star-extension-model.md) - Implementation plan
- [Devlore Extension Model](../architecture/devlore-extension-model.md) - Architecture
- Issue #29 - Star extension model tracking
