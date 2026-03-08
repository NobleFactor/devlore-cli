# Phase 4: Thread + Bridge

**Status**: Done
**PR**: #200

## Summary

Add `Thread` field to `op.Context`, create the thread in the executor,
define the `CallableResource` interface in `pkg/op`, and implement
callable detection in both the planned and immediate bridge paths.

## Changes

### Thread on Context — `pkg/op/context.go`

- Added `Thread *starlark.Thread` field to `op.Context`

### Executor — `internal/execution/executor.go`

- `newThread()` creates a Starlark thread with print→writer handler
- Thread set on context in `runFlat`, `RunPhased`, `RunNodes`

### CallableResource interface — `pkg/op/callable.go` (new file)

- `CallableResource` interface: `Resource` + `Init(*starlark.Thread) error` +
  `Fn() starlark.Callable` + `FuncTypeName() string`
- `RegisterCallableExtractor` / `ExtractCallable` — callback registry
  pattern allowing `mem` package to register its extractor without import cycle
- `isCallableResource(v)` — type check helper
- `isFuncType(t)` — reflect.Type check for func kind
- `validateSlotType` — accepts `CallableResource` for func-typed targets

### Planned bridge — `pkg/op/planned_reflect.go`

- `buildPlannedBridge` detects `*starlark.Function` targeting func-typed
  Go params, extracts via `ExtractCallable`, stores as slot immediate

### Immediate bridge — `pkg/op/action_reflect.go`

- `validateSlotType` accepts `CallableResource` for func-typed targets

### Extractor registration — `pkg/op/provider/mem/resource.go`

- Registered callable extractor in `init()`: Extract + Compile → CallableResource

### Callable identity — `pkg/op/provider/mem/callable.go`

- Added `FuncTypeName()` method implementing `op.CallableResource`

## Files Created/Modified

- `pkg/op/callable.go` — new: interface, registry, helpers
- `pkg/op/callable_test.go` — 6 tests: interface compliance, registry, type checks
- `pkg/op/context.go` — Thread field
- `pkg/op/planned_reflect.go` — callable detection in planned bridge
- `pkg/op/action_reflect.go` — validateSlotType for callable
- `pkg/op/provider/mem/resource.go` — extractor registration
- `pkg/op/provider/mem/callable.go` — FuncTypeName method
- `internal/execution/executor.go` — thread creation
