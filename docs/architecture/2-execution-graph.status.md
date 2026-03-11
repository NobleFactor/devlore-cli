# Status: Execution Graph

**Architecture document:** [2-execution-graph.md](2-execution-graph.md)

This document is an **early design spec** that describes the conceptual architecture — command pattern, graph lifecycle, serialization model. The conceptual flow (parseConfig → Build → Run/Serialize) is still valid. The YAML serialization examples are accurate. The code examples use outdated type names and package paths.

## Completion

| Component | Status | Completed | PR |
|-----------|--------|-----------|-----|
| Graph data model | Complete | 2025-12-01 | [#10](https://github.com/NobleFactor/devlore-cli/pull/10), [#43](https://github.com/NobleFactor/devlore-cli/pull/43) |
| Phase execution | Complete | 2026-02-16 | [#100](https://github.com/NobleFactor/devlore-cli/pull/100), [#103](https://github.com/NobleFactor/devlore-cli/pull/103) |
| Serialization and checksums | Complete | 2025-12-01 | [#7](https://github.com/NobleFactor/devlore-cli/pull/7), [#106](https://github.com/NobleFactor/devlore-cli/pull/106) |
| Graph builder | Complete | 2026-03-10 | [#207](https://github.com/NobleFactor/devlore-cli/pull/207) (returns `[]*op.Graph`) |
| Migration from old design | Complete | — | The "Migration from Current Design" section describes work that has been executed |

## Document Discrepancies

The conceptual architecture (Sections: Design Principles, Command Pattern, Serialization, File Locations) is correct. The code examples need updating:

- **Type name**: `ExecutionGraph` → `Graph` (in `pkg/op/graph.go`)
- **Package path**: `internal/graph/` → `pkg/op/` and `internal/writ/`
- **GraphState type**: Document says `type GraphState int` with `iota` — actual is `type GraphState string` with string constants
- **Serialize signature**: `Serialize(w io.Writer)` → `Serialize(enc Encoder)`
- **Run() location**: Document puts `Run()` on Graph — actual execution is on `execution.GraphExecutor`
- **GitStyleChecksum**: Document shows 2-arg — actual is 3-arg
- **CanonicalContent()**: Document shows unexported — actual is exported
- **Build() return**: `Build(Config) → ExecutionGraph` → `Build() ([]*op.Graph, error)` (returns slice)
- **Graph fields**: `Config` and `Results` don't exist; `Context GraphContext`, `Phases`, `Rollback`, `Catalog`, `Version` are not documented
- **`engine.ConflictResolution`** → `execution.ConflictResolution`
- **Node fields**: `SourceChecksum` and `TargetChecksum` don't exist; `Retry *RetryPolicy` is not documented
- **"Migration from Current Design" section**: Describes completed work — should be removed or marked as done

## Outstanding Work

1. **Update code examples** — type names, package paths, method signatures, field lists
2. **Remove or mark "Migration" section** as completed
