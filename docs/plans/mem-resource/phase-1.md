# Phase 1: mem.Resource + Callable

**Status**: Done
**PR**: #197

## Summary

Introduce the `mem.Resource` type (typed byte buffer with opaque URI)
and `mem.Callable` (embeds `mem.Resource`, adds bytecode storage and
metadata). This phase establishes the data structures; extraction,
compilation, and runtime support come in later phases.

## Changes

### mem.Resource — `pkg/op/provider/mem/resource.go`

- `Resource` struct: embeds `op.ResourceBase`, adds `ContentType`,
  `Qualifier`, `Data` ([]byte), `Hash` (SHA-256)
- Opaque URI: `mem:<content-type>/<qualifier>`
- `NewResource(contentType, qualifier)` constructor
- `NewResourceWithData(contentType, qualifier, data)` constructor
- `ComputeHash()` sets SHA-256 of Data
- `String()` via `ResourceBase.Format`
- Constructor registered in `init()` for catalog/slot deserialization

### mem.Callable — `pkg/op/provider/mem/callable.go`

- `Callable` struct: embeds `mem.Resource`, adds `Compiled` ([]byte),
  `FuncType`, `Name` (URI identity), `FuncName`, `ParamNames`,
  `NumParams`, `CompilerVersion`, `OriginalPos`, unexported `fn`
- URI: `mem:callable/<FuncType>/<Name>`
- `NewCallable(funcType, name)` constructor
- `SetSource(source)` sets Data and recomputes hash

## Files Created

- `pkg/op/provider/mem/resource.go`
- `pkg/op/provider/mem/callable.go`
- `pkg/op/provider/mem/callable_test.go` — construction, URI generation, hash tests
