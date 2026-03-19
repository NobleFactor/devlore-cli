---
title: "StructValue support for methods with parameters"
issue: https://github.com/NobleFactor/devlore-cli/issues/255
status: complete
created: 2026-03-19
updated: 2026-03-19
---

# Plan: StructValue support for methods with parameters

## Summary

Add a `typeParamsRegistry` that maps `reflect.Type` to `MethodParams` for any struct type. Extend `discoverMethods` to accept parameterized methods when registered, and extend `StructValue.Attr()` to return `starlark.NewBuiltin` callables for them. Codegen detects parameterized Resource methods and emits `RegisterTypeParams` calls in `resource.gen.go`. Unblocks the Knowledge Extract CI workflow (`yaml.Resource.Validate`, `json.Resource.Validate`).

## Context

`StructValue` wraps Go structs for Starlark, exposing fields and zero-arg methods as auto-invoked properties. Methods with parameters -- like `yaml.Resource.Validate(schemaJSON string)` -- were filtered out in `discoverMethods` (`if numIn != 0 { continue }`). The Knowledge Extract CI workflow calls `doc.validate(schema_json)` and fails with "resource has no .validate attribute".

The existing `ExecutingReceiver` already handles parameterized methods for providers, but requires a `ReceiverFactory`. Resource types like `yaml.Resource` are not providers -- they're values returned by providers -- so they can't use that path.

## Goals

1. **`typeParamsRegistry`** -- type-level param registration independent of provider infrastructure
2. **Parameterized methods as callables** -- `StructValue.Attr()` returns `starlark.NewBuiltin` for methods with params
3. **Codegen-driven registration** -- `detect_resource_params()` in `generate.star` introspects Resource methods; template emits `ResourceParams` and `RegisterTypeParams`
4. **Unblock CI** -- `yaml.Resource.Validate` and `json.Resource.Validate` exposed to Starlark

## Implementation Phases

### Phase 1: `typeParamsRegistry` -- complete

- [x] Add `typeParamsRegistry` (`sync.Map` of `reflect.Type` to `MethodParams`) in `starvalue_marshal.go`
- [x] Add `RegisterTypeParams(t reflect.Type, params MethodParams)`
- [x] Add `lookupTypeParams(t reflect.Type) (MethodParams, bool)`

### Phase 2: Extend `methodInfo` and `discoverMethods` -- complete

- [x] Extend `methodInfo` with `paramNames []string`, `numIn int`, `methodType reflect.Type`
- [x] Extract `classifyMethodReturnOk` helper to keep cognitive complexity under threshold
- [x] Modify `discoverMethods`: when `numIn > 0`, check `lookupTypeParams` for matching method entry
- [x] Validate param count matches method signature

### Phase 3: Bridge function and `StructValue.Attr()` update -- complete

- [x] Add `buildStructMethodBridge` in `starvalue_struct.go`
- [x] Modify `StructValue.Attr()`: if `mi.numIn > 0`, return `starlark.NewBuiltin(...)` wrapping the bridge

### Phase 4: Codegen -- complete

- [x] Add `detect_resource_params(path)` in `generate.star` -- introspects Resource methods, excludes error-only returns and unnamed params
- [x] Wire into both resource generation paths (provider+resource and resource-only)
- [x] Update `resource.gen.go.template` -- conditional `ResourceParams` var and `RegisterTypeParams` call in `Init()`
- [x] Rename generated type to `resourceFactory` (parallel to provider `Factory`)
- [x] Remove hand-written resource descriptors from `yaml/resource.go` and `json/resource.go` (duplicated by codegen)

### Phase 5: Tests -- complete

- [x] Unit tests in `pkg/op/starvalue_struct_test.go`:
  - Parameterized method discovered and returned as `*starlark.Builtin`
  - Positional arg call works
  - Keyword arg call works
  - Error propagation from method
  - Missing required arg error
  - Zero-arg methods still auto-invoked (regression guard)
  - Unregistered param method excluded
  - `AttrNames()` includes parameterized methods
- [ ] Integration test deferred to CI -- Knowledge Extract workflow exercises this path end-to-end

### Cleanup -- complete

- [x] Remove `file.Resource.WriteTo` -- callers use `root.ReadFile` directly
- [x] Update `encryption.Provider.DecryptSopsFile` to use `root.ReadFile`
- [x] Update `file.Provider.read` helper to use `root.ReadFile`
- [x] Regenerate all 8 `resource.gen.go` files

## Files Modified

| File | Purpose |
| --- | --- |
| `pkg/op/starvalue_marshal.go` | `typeParamsRegistry`, `RegisterTypeParams`, `lookupTypeParams`, extended `methodInfo`, refactored `discoverMethods` |
| `pkg/op/starvalue_struct.go` | `buildStructMethodBridge`, `Attr()` parameterized method dispatch |
| `pkg/op/starvalue_struct_test.go` | 8 parameterized method unit tests |
| `pkg/op/provider/yaml/resource.go` | Removed hand-written descriptor |
| `pkg/op/provider/json/resource.go` | Removed hand-written descriptor |
| `pkg/op/provider/file/resource.go` | Removed `WriteTo` |
| `pkg/op/provider/file/provider.go` | `read()` uses `root.ReadFile` |
| `pkg/op/provider/encryption/provider.go` | Uses `root.ReadFile` |
| `star/.../generate.star` | `detect_resource_params()`, wired into resource gen paths |
| `star/.../resource.gen.go.template` | `resourceFactory`, conditional `ResourceParams` |
| 8x `gen/resource.gen.go` | Regenerated |

## Verification

1. `make test` passes
2. `AttrNames()` on yaml.Resource includes `validate`
3. Zero-arg methods on yaml.Resource (like `parsed`, `hash`) still work as properties
4. Knowledge Extract CI workflow passes

## Related Documents

- Issue #255 -- StructValue support for methods with parameters
- `pkg/op/receiver_reflect.go` -- `ExecutingReceiver` pattern (the model for parameterized method dispatch)
