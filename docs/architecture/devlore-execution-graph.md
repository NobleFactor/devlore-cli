# Execution Graph Architecture

This document describes the execution graph design that unifies all lifecycle commands (deploy, upgrade, reconcile, decommission).

See also: [devlore-graph-convergence-operations.md](devlore-graph-convergence-operations.md) — Graph operations: convergence, control flow, and system interaction (probe, guard, choose, gather, retry, rollback, elevate).

## Design Principles

1. **Single Responsibility**: Commands parse flags, the graph does the work
2. **State Machine**: The graph transitions from plan → executed → serialized
3. **Unified Serialization**: Same structure represents both plans and receipts

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Command Layer                            │
│          (runDeploy, runUpgrade, runReconcile, runDecommission) │
├─────────────────────────────────────────────────────────────────┤
│  1. parseConfig(cmd, args) → Config                             │
│  2. builder.Build(config)  → ExecutionGraph                     │
│  3. graph.Run() or graph.Serialize()                            │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                        GraphBuilder                              │
│                    (internal/graph/builder.go)                   │
├─────────────────────────────────────────────────────────────────┤
│  Build(Config) → ExecutionGraph                                  │
│                                                                  │
│  - Collects sources (layers, segments)                          │
│  - Resolves file tree with precedence                           │
│  - Detects collisions                                           │
│  - Loads identities and engine data                             │
│  - Returns ready-to-execute graph                               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                       ExecutionGraph                             │
│                    (internal/graph/graph.go)                     │
├─────────────────────────────────────────────────────────────────┤
│  State: pending → executed                                       │
│                                                                  │
│  Run() error                                                     │
│    - Preflight checks                                           │
│    - Conflict resolution                                        │
│    - Execute operations                                         │
│    - Update node states                                         │
│                                                                  │
│  Serialize(w io.Writer) error                                    │
│    - Before Run(): outputs plan (what would happen)             │
│    - After Run(): outputs receipt (what happened)               │
│    - Computes checksum, optional signature                      │
└─────────────────────────────────────────────────────────────────┘
```

## Command Pattern

All lifecycle commands follow the same pattern:

```go
func runDeploy(cmd *cobra.Command, args []string) error {
    config := parseDeployConfig(cmd, args)

    graph, err := builder.Build(config)
    if err != nil {
        return err
    }

    if config.DryRun {
        return graph.Serialize(os.Stdout)
    }

    if err := graph.Run(); err != nil {
        return err
    }

    return graph.Serialize(receiptFile)
}
```

Target complexity: ~5-10 per command (down from 45-75).

## Config Types

Each lifecycle command has its own config type containing all resolved settings:

```go
// DeployConfig contains all settings for a deploy operation.
type DeployConfig struct {
    // Sources
    LayerSources []tree.LayerSource
    SourceRoot   string  // single-repo mode (when no layers configured)
    TargetRoot   string

    // Selection
    Projects []string
    Segments segment.Segments

    // Behavior
    DryRun             bool
    Verbose            bool
    ConflictResolution engine.ConflictResolution

    // Data
    TemplateData map[string]any
    Identities   []age.Identity
    SigningKey   *age.X25519Identity
}

// UpgradeConfig contains all settings for an upgrade operation.
type UpgradeConfig struct {
    Projects   []string
    TargetRoot string
    Force      bool
    DryRun     bool
    Verbose    bool
    // ...
}

// ReconcileConfig contains all settings for a reconcile operation.
type ReconcileConfig struct {
    Projects   []string
    TargetRoot string
    CheckDrift bool
    Verbose    bool
    // ...
}

// DecommissionConfig contains all settings for a decommission operation.
type DecommissionConfig struct {
    Projects   []string
    TargetRoot string
    Force      bool
    Prune      bool   // Remove empty parent directories
    DryRun     bool
    Verbose    bool
    // ...
}
```

Config parsing rolls up the entire settings hierarchy:
1. Defaults
2. Config file (`~/.config/devlore/config.yaml`)
3. Environment variables (`WRIT_*`)
4. Command-line flags

## ExecutionGraph

The graph is a stateful container for operations:

```go
type ExecutionGraph struct {
    // Identity
    Tool      string    // "writ" or "lore"
    Timestamp time.Time

    // Context
    Config    *Config   // resolved configuration
    Platform  Platform  // OS, arch

    // Content
    Nodes     []*Node   // operations to perform
    Edges     []Edge    // dependencies
    Collisions []Collision

    // State (mutated by Run)
    State     GraphState  // pending, executed, failed
    Results   []Result    // populated after Run()
    Summary   Summary     // computed from results

    // Integrity
    Checksum  string
    Signature *Signature
}

type GraphState int

const (
    StatePending GraphState = iota
    StateExecuted
    StateFailed
)
```

## Node States

Each node tracks its own state:

```go
type Node struct {
    ID         string
    Operations []string
    Source     string
    Target     string
    Project    string
    Layer      string

    // State (mutated by Run)
    Status         NodeStatus  // pending, completed, skipped, failed
    Timestamp      time.Time
    SourceChecksum string
    TargetChecksum string
    Error          string
    Annotations    map[string]string
}

type NodeStatus string

const (
    StatusPending   NodeStatus = "pending"
    StatusCompleted NodeStatus = "completed"
    StatusSkipped   NodeStatus = "skipped"
    StatusFailed    NodeStatus = "failed"
)
```

## Serialization

The same graph structure serializes differently based on state:

### Before Run() - Plan Output

```yaml
tool: writ
timestamp: 2025-01-29T10:30:00Z
state: pending
platform:
  os: darwin
  arch: arm64
context:
  source_root: ~/.local/share/devlore/repos
  target_root: ~
  projects: [base, team, personal]
nodes:
  - id: .config/git/config
    operations: [link]
    status: pending
    source: /Users/me/.local/share/devlore/repos/base/.config/git/config
    target: /Users/me/.config/git/config
```

### After Run() - Receipt Output

```yaml
tool: writ
timestamp: 2025-01-29T10:30:00Z
state: executed
platform:
  os: darwin
  arch: arm64
context:
  source_root: ~/.local/share/devlore/repos
  target_root: ~
  projects: [base, team, personal]
nodes:
  - id: .config/git/config
    operations: [link]
    status: completed
    timestamp: "2025-01-29T10:30:01Z"
    source: /Users/me/.local/share/devlore/repos/base/.config/git/config
    target: /Users/me/.config/git/config
summary:
  total_files: 42
  links: 38
  templates: 3
  secrets: 1
checksum: "sha256:a7b9c3d4..."
```

### Migration Output (`writ migrate`)

The `writ migrate` command produces an extended graph format that includes
LLM-generated analysis alongside the execution graph. This format supports
both human review (analysis section) and machine execution (nodes/edges).

```yaml
tool: writ
timestamp: 2025-01-29T10:30:00Z
state: pending
platform:
  os: darwin
  arch: arm64
analysis:
  source_root: /Users/me/dotfiles
  system: tuckr
  system_confidence: 0.95
  input_summary: |
    Repository with Home/Configs/ containing 13 group directories.
    Root has Install-UnixUserConfiguration and Install-WindowsUserConfiguration.ps1
    scripts that invoke tuckr commands.
  structure:
    groups_path: Home/Configs
    naming_convention: "<group>-<Platform>"
    groups:
      - all
      - all-Darwin
      - all-Linux
      - noblefactor
      - noblefactor-Unix
    platforms:
      - Darwin
      - Linux
      - Unix
      - Windows
  observations:
    - "Tuckr-managed repository with groups in Home/Configs/"
    - "Install scripts at root invoke tuckr add/rm for deployment"
    - "Uses git-crypt for secret encryption"
  warnings:
    - "git-crypt detected — writ uses SOPS; consider migration"
  recommendations:
    - "After migration, update Install-UnixUserConfiguration to use new group names"
    - "Create .sops.yaml to migrate from git-crypt to SOPS"
context:
  source_root: /Users/me/dotfiles
nodes:
  - id: rename-all-darwin
    operations: [rename]
    status: pending
    source: Home/Configs/all-Darwin
    target: Home/Configs/all.Darwin
  - id: rename-all-linux
    operations: [rename]
    status: pending
    source: Home/Configs/all-Linux
    target: Home/Configs/all.Linux
  - id: rename-noblefactor-unix
    operations: [rename]
    status: pending
    source: Home/Configs/noblefactor-Unix
    target: Home/Configs/noblefactor.Unix
edges:
  - from: rename-all-darwin
    to: rename-all-linux
    relation: orders
  - from: rename-all-linux
    to: rename-noblefactor-unix
    relation: orders
```

The `analysis` section is generated by the LLM and provides:

| Field | Purpose |
|-------|---------|
| `system` | Detected dotfile manager (tuckr, stow, chezmoi, yadm, etc.) |
| `system_confidence` | LLM confidence in detection (0.0–1.0) |
| `input_summary` | Human-readable description of what the LLM analyzed |
| `structure` | Detected repository structure and naming conventions |
| `observations` | Notable patterns or configurations |
| `warnings` | Potential issues requiring attention |
| `recommendations` | Suggested actions after migration |

The `nodes` section contains rename operations to convert from the source
system's naming convention to writ's `<project>.<Platform>` convention.

## Run() Implementation

```go
func (g *ExecutionGraph) Run() error {
    if g.State != StatePending {
        return fmt.Errorf("graph already executed")
    }

    // 1. Preflight checks
    conflicts := g.preflight()
    if err := g.handleConflicts(conflicts); err != nil {
        g.State = StateFailed
        return err
    }

    // 2. Execute operations
    eng := g.createEngine()
    results, err := eng.Run(context.Background(), g.toEngineGraph())
    if err != nil {
        g.State = StateFailed
        return err
    }

    // 3. Update node states from results
    g.applyResults(results)
    g.State = StateExecuted
    g.computeSummary()

    return nil
}
```

## Serialize() Implementation

```go
func (g *ExecutionGraph) Serialize(w io.Writer) error {
    // Compute checksum on canonical content
    canonical := g.canonicalContent()
    filename := g.filename()
    g.Checksum = GitStyleChecksum(filename, canonical)

    // Optional signing
    if g.Config.SigningKey != nil && g.State == StateExecuted {
        g.sign(g.Config.SigningKey)
    }

    // Write YAML
    return yaml.NewEncoder(w).Encode(g)
}

func (g *ExecutionGraph) filename() string {
    return fmt.Sprintf("%s-%s.yaml", g.Tool, g.Timestamp.Format("2006-01-02T15-04-05"))
}
```

## File Locations

```
~/.local/state/devlore/
├── receipts/
│   ├── writ-2025-01-29T10-30-00.yaml
│   ├── lore-2025-01-29T11-00-00.yaml
│   ├── writ-latest.yaml → writ-2025-01-29T10-30-00.yaml
│   └── lore-latest.yaml → lore-2025-01-29T11-00-00.yaml
└── state.yaml  # aggregate state across receipts
```

## Migration from Current Design

The current implementation has:
- `internal/engine/` - operation execution (keep)
- `internal/writ/tree/` - file tree building (refactor into GraphBuilder)
- `internal/writ/receipt/` - receipt types (merge into ExecutionGraph)
- `internal/writ/deploystate/` - state tracking (keep, fed by ExecutionGraph)
- `internal/writ/commands.go` - 360-line god functions (refactor to 10-line commands)

New structure:
```
internal/
├── engine/           # operation execution (unchanged)
├── graph/
│   ├── graph.go      # ExecutionGraph type
│   ├── builder.go    # GraphBuilder.Build()
│   ├── config.go     # Config types
│   └── serialize.go  # Serialization logic
├── writ/
│   ├── commands.go   # thin command handlers
│   └── config.go     # parseDeployConfig, etc.
└── lore/
    ├── commands.go   # thin command handlers
    └── config.go     # parseLoreConfig, etc.
```

## Benefits

1. **Complexity**: Commands drop from 45-75 to 5-10
2. **Testability**: GraphBuilder and ExecutionGraph are independently testable
3. **Reusability**: Same graph infrastructure for writ and lore
4. **Clarity**: Clear separation of config → build → execute → serialize
5. **Debugging**: Graph state is inspectable at any point
