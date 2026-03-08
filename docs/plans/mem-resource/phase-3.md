# Phase 3: Compilation

**Status**: Done
**PR**: #199

## Summary

Implement `Compile()` and `Init(thread)` methods on `mem.Callable`.
Compile produces serialized bytecode; Init loads bytecode (fast path)
or recompiles from source (version mismatch fallback). This completes
the three-tier storage: source text → compiled bytecode → live callable.

## Changes

### Callable lifecycle — `pkg/op/provider/mem/callable.go`

- `Compile()` — `SourceProgramOptions` → `Program.Write` → bytecode in
  `Compiled` field. Records `CompilerVersion`. Idempotent.
- `Init(thread)` — loads compiled program via `CompiledProgram` (fast path)
  or recompiles via `SourceProgramOptions` (version mismatch). Extracts
  the named function from globals. Stores as `c.fn`.
- `Fn()` — returns live callable; panics if `Init` not called.

### Version fallback

When `CompilerVersion` doesn't match the runtime's `starlark.CompilerVersion`,
`Init` falls back to recompiling from source text. Source is always present
as the authoritative representation.

## Files Modified

- `pkg/op/provider/mem/callable.go` — added `Compile()`, `Init()`, `Fn()` methods

## Tests Added

- `pkg/op/provider/mem/callable_test.go` — 12 new tests: Compile (4),
  Init (6), BytecodeRoundTrip, ExtractCompileInitRoundTrip
