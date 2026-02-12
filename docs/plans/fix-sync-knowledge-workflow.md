# Plan: Fix sync-knowledge Workflow

---
title: Fix sync-knowledge Workflow
issue: https://github.com/NobleFactor/devlore-cli/issues/86
status: implemented
created: 2026-02-10
updated: 2026-02-11
blocked_by: https://github.com/NobleFactor/devlore-cli/issues/84
---

## Summary

Fix the sync-knowledge GitHub Actions workflow that fails after PR #85 introduced the `star/extensions/` directory structure. The blocking issue (missing `go.parse_devlore_api` receiver) is resolved by compiling the Go receiver to WASM and shipping it with the extensions.

## Quick Start for Future Sessions

**To pick up this work:**

1. Read this plan
2. Implement Phase 2 (choose Option A, B, or C based on user preference)
3. If Option B chosen, also implement Phase 3

**Current blocker:** The Starlark script `build-knowledge.star` calls `go.parse_devlore_api()` but no binary has that receiver wired up.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Binary path | :white_check_mark: Fixed | Changed to `bin/star` |
| Extension discovery | :white_check_mark: Fixed | Run from devlore-cli directory |
| Receiver `go.parse_devlore_api` | :x: Blocking | Not wired to any binary |

## Error Progression

**Error 1** - Binary path conflict (FIXED):
```
./star: Is a directory
```

**Error 2** - Extension discovery (FIXED):
```
Error: unknown command "devlore-registry" for "star"
```

**Error 3** - Missing receiver (CURRENT BLOCKER):
```
Error: go has no .parse_devlore_api attribute
  build-knowledge.star:79:13: in build_onboarding_knowledge
```

## Implemented Solution: WASM Go Receiver

Options A/B/C from the original plan are superseded by the WASM approach.

The Go receiver functions are compiled to WASM (`wasm32-wasip1`, reactor mode) and
shipped with the extensions. The star runtime loads them via its existing WASM support —
no binary changes needed.

### Architecture

```
star/extensions/com.noblefactor.devlore.registry.BuildKnowledge/
├── go.mod              # Go module (stdlib only)
├── build.mk            # Build rules
├── extension.yaml      # wasm: receivers/go.wasm
├── src/
│   ├── main.go         # JSON-RPC protocol (reactor mode, init())
│   ├── devlore_api.go  # parse_devlore_api
│   ├── migrate.go      # parse_migrate_knowledge
│   └── execution.go    # parse_execution_ops, parse_execution_schema
├── commands/
│   └── build-knowledge.star
└── receivers/
    └── go.wasm         # Compiled artifact (committed)
```

### Companion change (noblefactor-ops)

`internal/starlark/wasm_receiver.go`: Added `jsonToStarlarkStruct` function so WASM
receiver results use `starlarkstruct.Struct` (attribute access) instead of `starlark.Dict`
(key access). This ensures WASM receivers behave identically to builtin receivers.

### Protocol

JSON-RPC over stdin/stdout (reactor mode):
- Star runtime writes request to stdin, calls `_initialize`
- Module reads stdin in `init()`, dispatches by method, writes response to stdout

### Known issue

The gitignore WASM receiver (`com.noblefactor.star.Gitignore`) is built as a WASI
command (exports `_start`) but the star runtime expects a reactor (`_initialize`).
Tracked in issue #86 comment.

## Files Reference

| File | Purpose |
| --- | --- |
| `star/extensions/.../src/*.go` | WASM Go receiver source (4 parsing functions) |
| `star/extensions/.../receivers/go.wasm` | Compiled WASM artifact |
| `star/extensions/.../extension.yaml` | Extension config (wasm: receivers/go.wasm) |
| `.github/workflows/sync-knowledge.yaml` | The workflow that uses these receivers |

## Acceptance Criteria

- [x] Go receiver compiles to WASM with `_initialize` export (reactor mode)
- [x] All 4 parsing functions ported (parse_devlore_api, parse_migrate_knowledge, parse_execution_ops, parse_execution_schema)
- [x] WASM binary committed to both extensions
- [x] extension.yaml updated (builtin:true → wasm: receivers/go.wasm)
- [x] `internal/starlark/devlore/api.go` removed (replaced by WASM module)
- [x] devlore-cli tests pass
- [ ] End-to-end test with star runtime (requires noblefactor-ops companion PR)

## Related Documents

- [Issue #86](https://github.com/NobleFactor/devlore-cli/issues/86) - This workflow issue
- [Issue #84](https://github.com/NobleFactor/devlore-cli/issues/84) - Wire up devlore receiver (BLOCKING)
- [PR #85](https://github.com/NobleFactor/devlore-cli/pull/85) - Add star extensions
- [PR #52 noblefactor-ops](https://github.com/NobleFactor/noblefactor-ops/pull/52) - Removed devlore from ops