# Phase 2: Extraction

**Status**: Done
**PR**: #198

## Summary

Implement `Extract(*starlark.Function)` which introspects a Starlark
function and produces a self-contained `mem.Callable` with synthesized
source text. Closure bindings are captured and inlined as module-level
constants. Lambda and named def extraction supported.

## Changes

### Extraction — `pkg/op/provider/mem/extract.go`

- `Extract(fn, funcType)` — entry point, handles lambda naming
- `ExtractWithName(fn, funcType, name)` — full extraction with custom name
- `synthesize(fn, params)` — builds synthetic source file:
  - Header comment with original position
  - Closure bindings as module-level constants via `FormatLiteral`
  - Lambda → `def _callable(...)` transformation
  - Named def → source extraction via AST walk
- `extractLambdaBody(fn)` — parses source file, finds lambda at position,
  extracts body expression via `syntax.Walk`
- `extractDefSource(fn)` — parses source file, finds def at position,
  extracts full def statement
- `extractSpan(data, start, end)` — extracts text between two AST positions
- `ValidateArity(fn, minParams, maxParams)` — checks required/total params

### Literal serialization — `pkg/op/provider/mem/literals.go`

- `FormatLiteral(v)` — serializes frozen Starlark values as valid source literals
- Supports: None, Bool, Int, Float, String, List, Dict, Tuple, Struct
- Struct values serialized as dict literals with sorted keys (full fidelity)
- Depth limit 20 (circular reference protection)
- Rejects Set type (use List instead)

## Files Created

- `pkg/op/provider/mem/extract.go`
- `pkg/op/provider/mem/extract_test.go` — 13 tests: lambda, closure, named def, round-trips, arity
- `pkg/op/provider/mem/literals.go`
- `pkg/op/provider/mem/literals_test.go` — 14+ tests: all literal types, escaping, nesting, struct serialization
