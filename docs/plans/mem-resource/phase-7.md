# Phase 7: Codegen

**Status**: Pending

## Summary

Teach the `star` code generator to recognize callable-typed parameters
and generate the appropriate param registration and bridge code. This
eliminates the manual `fn` entry in `MethodParams` and the hand-edited
`gen/params.gen.go`.

## Planned Changes

- `star` tool recognizes func-typed parameters on Provider methods
- Generates `fn` param in `params.gen.go` for callable-typed parameters
- Generates bridge code that passes `starlark.Callable` through to the
  reflection layer (which handles adaptation via `buildCallableFunc`)
- No `+devlore:callable` annotation needed — the code generator inspects
  the Go function type directly

## Files to Modify

- `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star` —
  callable param detection and bridge generation
- `pkg/op/provider/file/gen/params.gen.go` — generated with `fn` param
- `pkg/op/provider/file/gen/immediate.gen.go` — generated bridge code
- `pkg/op/provider/file/gen/planned.gen.go` — generated bridge code

## Notes

This phase lives in the `star` tool (noblefactor-ops repo), not in
devlore-cli. The `generate.star` codegen script currently has
`+devlore:callable swallow=...` parsing code that should be removed
and replaced with direct func-type inspection.
