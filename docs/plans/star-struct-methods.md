---
title: "Marshal Go struct methods to Starlark"
issue: TBD
status: complete
created: 2026-03-18
updated: 2026-03-18
---

# Plan: Marshal Go struct methods to Starlark

## Summary

Extend `Marshal()` so that Go struct methods are automatically exposed as Starlark attributes alongside fields. Today the marshaler handles fields; after this change it handles methods too. Go programmers write normal Go code — structs with fields and methods — and the marshaler figures it out. Annotations are occasional hints; custom marshaling is the rarest case.

Internally, marshaled structs shift from `starlarkstruct.Struct` (pre-baked dict, fields only) to a lightweight `StructValue` type with lazy `Attr()` dispatch (fields and methods computed on access). This is an implementation detail — callers of `Marshal()` don't see or construct `StructValue` directly.

## Goals

1. **Methods just work**: Exported zero-arg methods become Starlark attrs automatically, same as fields do today.
2. **Consistent semantics**: Lazy attr dispatch for both fields and methods, matching `ExecutingReceiver` behavior and Python's attribute protocol.
3. **starlark.Value passthrough**: Types already implementing `starlark.Value` are returned directly, not flattened.
4. **Stringer integration**: `fmt.Stringer` drives the value's representation, not exposed as an attr.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Field marshaling | Working | `Marshal()` handles fields via reflection + `starlark` tags |
| Method marshaling | Missing | Methods are silently discarded |
| starlark.Value passthrough | Partial | Top-level `Marshal()` catches it; recursive `marshalReflect` does not |

## Requirements

### R1: Method discovery in `getTypeInfo`

Extend the cached `typeInfo` to include eligible methods alongside fields. A method is eligible if:

- Exported
- Zero non-receiver parameters
- Returns exactly 1 value (`func() T`) or 2 values where the second is `error` (`func() (T, error)`)
- `func() error` alone is not eligible
- Not `String` matching the `fmt.Stringer` signature (reserved for value representation)

Methods are discovered from `reflect.PointerTo(structType)` so that both value-receiver and pointer-receiver methods are included.

Go prevents field/method name collisions at compile time. Tag-aliased collisions (e.g., field `FullText` tagged `starlark:"text"` alongside method `Text()`) are the author's responsibility to resolve — either by renaming or re-tagging.

### R2: Lazy `StructValue` replaces `starlarkstruct.Struct`

Replace the pre-baked `starlarkstruct.Struct` with an internal `StructValue` type. It holds a `reflect.Value` (the Go struct) and a `*typeInfo`, and implements `starlark.Value` + `starlark.HasAttrs`:

- `Attr(name)`: fields are marshaled on access; methods are called on access and their return values marshaled. Errors from `func() (T, error)` methods propagate.
- `AttrNames()`: returns sorted field + method names from `typeInfo`.
- `String()`: delegates to `fmt.Stringer` if the Go type implements it; otherwise formats as `type_name(field = val, ...)`.
- `Type()`: snake_case type name.
- `Freeze()`: no-op. `Truth()`: `True`. `Hash()`: error (unhashable).

For non-addressable struct values, store a pointer via `reflect.New` to enable pointer-receiver method calls.

### R3: Update unmarshal and `FormatLiteral` for `StructValue`

Sites that type-assert on `*starlarkstruct.Struct` need a parallel `*StructValue` case:

- `unmarshalStruct` — add case using `Attr()` (same pattern as existing `*starlarkstruct.Struct` case)
- `unmarshalToAny` — add case delegating to shared helper
- `FormatLiteral` / `formatStruct` in `pkg/op/provider/mem/literals.go` — add case using `AttrNames()` + `Attr()`

### R4: `starlark.Value` passthrough in `marshalReflect`

In the `reflect.Struct` case, for structs with no exported fields, check `starlark.Value` (value and pointer) before the existing `starvalue.Marshaler` check. Return directly if satisfied.

## Design Decisions

| Decision | Choice | Rationale |
| --- | --- | --- |
| Which methods? | All exported with supported signatures, minus `String` | Same as fields — the marshaler figures it out; `String` reserved for representation |
| Eager or lazy? | Lazy (computed in `Attr()`) | Consistent with `ExecutingReceiver`; Go author controls caching |
| Internal type | `StructValue` (unexported to callers) | Implementation detail of `Marshal()`; Go programmers never see it |
| Non-addressable values | Store pointer via `reflect.New` | Enables pointer-receiver methods; safe for read-only access |

## Implementation Phases

### Phase 1: Extend `typeInfo` with method discovery

- [x] Add `methodInfo` struct (Go name, Starlark name, hasError flag)
- [x] Add `methods []methodInfo` and `byMethod map[string]*methodInfo` to `typeInfo`
- [x] In `getTypeInfo`, after field loop, iterate `reflect.PointerTo(t).NumMethod()` and collect eligible methods
- [x] Exclude `String` if it matches `fmt.Stringer` signature
- [x] Include method names in `attrList` (sorted alongside field names)
- [x] Cache the snake_case type name in `typeInfo` (avoid recomputing `camelToSnake` per marshal)

**Files**:

- `pkg/op/starvalue_marshal.go` - Modify

### Phase 2: `StructValue` type

- [x] Define `StructValue` struct (typeName, goValue, info)
- [x] Implement `starlark.Value`: `String()`, `Type()`, `Freeze()`, `Truth()`, `Hash()`
- [x] Implement `starlark.HasAttrs`: `Attr()`, `AttrNames()`
- [x] `Attr()` resolves fields first (marshal via `marshalReflect`), then methods (call + marshal)
- [x] `String()` delegates to `fmt.Stringer` if available, otherwise formats attrs

**Files**:

- `pkg/op/starvalue_struct.go` - Create

### Phase 3: Wire `marshalStruct` to produce `StructValue`

- [x] Replace `marshalStruct` body: wrap Go value in `StructValue`
- [x] For non-addressable values, create pointer via `reflect.New`
- [x] Remove pre-baked `starlark.StringDict` construction

**Files**:

- `pkg/op/starvalue_marshal.go` - Modify

### Phase 4: Update unmarshal and `FormatLiteral`

- [x] Add `*StructValue` case to `unmarshalStruct`
- [x] Add `*StructValue` case to `unmarshalToAny`
- [x] Add `*StructValue` case to `FormatLiteral` in `pkg/op/provider/mem/literals.go`

**Files**:

- `pkg/op/starvalue_marshal.go` - Modify
- `pkg/op/provider/mem/literals.go` - Modify

### Phase 5: `starlark.Value` passthrough

- [x] In the `reflect.Struct` case of `marshalReflect`, check `starlark.Value` for fieldless structs
- [x] Place check before the existing `starvalue.Marshaler` check

**Files**:

- `pkg/op/starvalue_marshal.go` - Modify

### Phase 6: Tests

- [x] Test field access via `Attr()` returns correct marshaled value
- [x] Test method with `func() T` signature appears as attr (lazy, called on access)
- [x] Test method with `func() (T, error)` appears as attr (success path)
- [x] Test method with `func() (T, error)` propagates error on access
- [x] Test methods with unsupported signatures are ignored
- [x] Test `String()` excluded from attrs, used for representation
- [x] Test `AttrNames()` includes both fields and methods, sorted
- [x] Test type implementing `starlark.Value` is returned directly
- [x] Test unmarshal round-trip through `StructValue`
- [x] Test `FormatLiteral` handles `StructValue`
- [x] Update existing tests that assert `*starlarkstruct.Struct` to expect `*StructValue`
- [x] Run `make check` to verify no regressions

**Files**:

- `pkg/op/starvalue_struct_test.go` - Create
- `pkg/op/starvalue_marshal_test.go` - Modify
- `pkg/op/provider/mem/literals_test.go` - Modify (if needed)

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/starvalue_struct.go` | Create | Internal `StructValue` type with lazy `Attr()` dispatch |
| `pkg/op/starvalue_struct_test.go` | Create | Tests for field + method access, String, AttrNames |
| `pkg/op/starvalue_marshal.go` | Modify | Extend `typeInfo`, replace `marshalStruct`, add starlark.Value passthrough |
| `pkg/op/starvalue_marshal_test.go` | Modify | Update existing assertions for new output type |
| `pkg/op/provider/mem/literals.go` | Modify | Add `StructValue` case to `FormatLiteral` |
