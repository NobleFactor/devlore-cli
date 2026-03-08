# Phase 5: WalkTree Action

**Status**: Done
**PR**: pending

## Summary

Generic callable→func coercion in the reflection layer. No per-action
custom code needed — the reflected action infrastructure handles
`CallableResource` slots automatically. The callable must match the
full Go function signature (no arity truncation, no swallowed params).

## Changes

### Generic coercion — `pkg/op/callable.go`

- `initCallableSlots(ctx, slots, methodType, paramNames)` — pre-processes
  slots in `Do()` before `coerceArgs`. For each slot containing a
  `CallableResource` targeting a func-typed Go param: calls `Init(thread)`,
  builds adapted Go func via `buildCallableFunc`, replaces slot value.
- `buildCallableFunc(fn, thread, targetType)` — creates Go function
  matching `targetType` via `reflect.MakeFunc`. Marshals all Go args →
  Starlark, calls the function, unmarshals return values. The Starlark
  callable must accept all params matching the Go func signature.
- `makeErrorReturn(funcType, numOut, err)` — builds error return values
- `unmarshalReturn(funcType, numOut, result)` — converts Starlark result
  to Go return values

### Wiring — `pkg/op/action_reflect.go`

- `initCallableSlots` wired into all three `Do()` methods:
  `reflectedPureAction`, `reflectedFallibleAction`,
  `reflectedCompensableAction`. Runs before `coerceArgs`.

### Exported marshal helpers — `pkg/op/starvalue_marshal.go`

- Added exported `MarshalValue` and `UnmarshalAny` wrappers for
  provider package access.

## Design Decisions

- **No arity truncation**: The Starlark callable must accept all params
  matching the Go func type. The `Actor` convenience wrapper was a
  temporary workaround before marshaling code existed and has been removed.
- **No `+devlore:callable` annotation**: The annotation was only needed
  for arity truncation (`swallow=stack`). With full-signature matching,
  no annotation is needed.

## Files Modified

- `pkg/op/callable.go` — initCallableSlots, buildCallableFunc, helpers
- `pkg/op/callable_test.go` — 5 new tests: SimpleReturn, FullSignature,
  StarlarkError, ReplacesCallable, SkipsNonCallable
- `pkg/op/action_reflect.go` — wired initCallableSlots into Do() methods
- `pkg/op/starvalue_marshal.go` — exported wrappers
- `pkg/op/provider/file/callable_test.go` — 2 integration tests:
  WalkTreeAction_Integration, WalkTreeAction_DryRun
- `pkg/op/provider/file/provider.go` — removed `Actor` function,
  removed `+devlore:callable swallow=stack` annotation from `Reducer`
- `pkg/op/provider/starcode/provider.go` — converted `Actor` usage to
  full `Reducer` signature
