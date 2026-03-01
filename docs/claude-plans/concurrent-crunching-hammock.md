# Plan: Worker 2 - Phase 3 Starlark Package

## Summary

Extract `Command` and `Flag` types from `runtime.go` to a new `command.go` file as part of the star extension model refactoring. This separates command-related code from the runtime engine.

## Context

- **Branch**: `feature/ext-starlark-phase-3`
- **Current state**: Branch is behind `origin/develop` by 5 commits (missing Phase 2 extension/wasm work)
- **Merge order**: Worker 4 (commands-phase-3) merges first, then Worker 2 (this branch)

## Step 1: Update Branch

Merge latest from `origin/develop` to get Phase 2 infrastructure (extension and wasm packages):

```bash
git fetch origin
git merge origin/develop --no-edit
```

## Step 2: Create `internal/starlark/command.go`

Move the following from `runtime.go` to new `command.go`:

### Types to Move
- `Command` struct (lines 128-135) - command definition with RunFunc
- `Flag` struct (lines 138-143) - flag definition

### Methods to Move
- `(c *Command) Run(args)` (lines 146-175) - command execution

### Internal Types to Move
- `commandCollector` struct (lines 178-180)
- `commandCollector.commandBuiltin()` method (lines 182-222)
- `parseFlag()` function (lines 224-252)

### File Structure

```go
// internal/starlark/command.go
package starlark

// Command types and execution
// - Command struct
// - Flag struct
// - Command.Run method
// - commandCollector (internal)
// - parseFlag (internal)
```

## Step 3: Update `runtime.go`

No changes needed beyond the deletions - the Runtime struct already references `*Command` which will be in the same package.

## Step 4: Verify

```bash
go build ./...
go test ./internal/starlark/...
```

## Critical Files

| File | Action |
|------|--------|
| `internal/starlark/command.go` | CREATE - command types |
| `internal/starlark/runtime.go` | MODIFY - remove extracted code |

## Acceptance Criteria

- `Command` and `Flag` types in dedicated `command.go` file
- No changes to external API (types remain in `starlark` package)
- All tests pass
- Build succeeds
