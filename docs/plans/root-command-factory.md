---
title: "Root Command Factory"
status: complete
created: 2026-03-10
updated: 2026-03-10
---

# Plan: Root Command Factory

## Summary

Extract duplicated cobra root command setup from `internal/lore/root.go` and
`internal/writ/root.go` into a shared `cli.NewRootCmd(cfg RootConfig)` factory.
Eliminates structural drift between the two CLI entry points.

## Goals

1. **Single source of truth** for shared flags, Viper init, and metadata commands
2. **Prevent drift** between lore and writ CLI configuration
3. **Fix existing drift bugs** discovered during analysis

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `lore/root.go` | 154 lines | Duplicated setup, missing `cmd.Root()` in BindFlags |
| `writ/root.go` | 145 lines | Duplicated setup, missing `DisableAutoGenTag` |
| Shared factory | Missing | No shared abstraction exists |

## Implementation Phases

### Phase 1: Extract factory (complete)

- [x] Create `internal/cli/root.go` with `RootConfig` struct and `NewRootCmd` factory
- [x] Move `initConfig` into factory as `initRootConfig` (unexported)
- [x] Simplify `internal/lore/root.go` to call factory + add tool-specific flag and subcommands
- [x] Simplify `internal/writ/root.go` to call factory + add tool-specific flag and subcommands
- [x] Fix writ missing `DisableAutoGenTag: true`
- [x] Fix lore using `cmd` instead of `cmd.Root()` in BindFlags
- [x] Apply Go style guidelines (doc comments with Parameters/Returns)
- [x] `make build`, `make test`, `make vet` pass

**Files**:

- `internal/cli/root.go` - Create (shared factory + initRootConfig)
- `internal/lore/root.go` - Rewrite (154 -> 68 lines)
- `internal/writ/root.go` - Rewrite (145 -> 59 lines)

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `internal/cli/root.go` | Create | Shared `NewRootCmd` factory and `initRootConfig` |
| `internal/lore/root.go` | Rewrite | Tool-specific flag + subcommands only |
| `internal/writ/root.go` | Rewrite | Tool-specific flag + subcommands only |

## Related Documents

- [CODE-CONSOLIDATION-ANALYSIS.md](../../CODE-CONSOLIDATION-ANALYSIS.md) - Item 2.1: Root Command Initialization
