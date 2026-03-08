# Phase 6: E2E Tests

**Status**: Done
**PR**: pending (combined with Phase 5)

## Summary

E2E Starlark test scripts exercising the full WalkTree callable flow
through both immediate and planned modes. Immediate bridge callable
support in `callNonVariadic`.

## Changes

### Immediate bridge — `pkg/op/receiver_reflect.go`

- `callNonVariadic` detects `starlark.Callable` values targeting
  func-typed Go params and adapts via `buildCallableFunc`.
- Thread passed through from the `builtinFunc` closure.

### Test runner — `internal/e2e/testrunner/runner.go`

- Blank import of `mem` package for callable extractor registration
  (needed by planned-mode `ExtractCallable`).

### Closure binding serialization — `pkg/op/provider/mem/literals.go`

- `FormatLiteral` now handles `*starlarkstruct.Struct` values by
  serializing as dict literals with sorted keys. This enables
  full-fidelity serialization of Resources captured in closures.

### E2E test scripts

All callables use the full 4-param signature matching the Go
`Reducer` type: `(initial, resource, path, stack)`.

| Script | Mode | Tests |
|--------|------|-------|
| `test_walk_tree.star` | Immediate | Walk temp dir, collect+sort paths, verify 4 entries |
| `test_walk_tree_planned.star` | Planned | `plan.file.walk_tree` with callable reducer, 1 graph node |
| `test_walk_tree_gitignore.star` | Immediate | `.gitignore` filtering: *.log excluded, .gitignore included |
| `test_walk_tree_closure.star` | Immediate | Def capturing closure variable (`ext=".py"`), filter by extension |

## Files Created/Modified

- `pkg/op/receiver_reflect.go` — callable detection in callNonVariadic
- `internal/e2e/testrunner/runner.go` — mem package import
- `internal/e2e/testrunner/runner_test.go` — 4 test functions
- `internal/e2e/testrunner/data/test_walk_tree.star` — new
- `internal/e2e/testrunner/data/test_walk_tree_planned.star` — new
- `internal/e2e/testrunner/data/test_walk_tree_gitignore.star` — new
- `internal/e2e/testrunner/data/test_walk_tree_closure.star` — new
- `pkg/op/provider/mem/literals.go` — struct serialization support
- `pkg/op/provider/mem/literals_test.go` — struct serialization tests
- `pkg/op/provider/mem/callable.go` — removed stale "excluding swallowed" comment
