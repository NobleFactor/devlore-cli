e---
title: "Drop resourceID from ResourceConstructor"
issue: TBD
status: draft
created: 2026-04-03
updated: 2026-04-03
---

# Plan: Drop resourceID from ResourceConstructor

## Summary

Remove the `resourceID` parameter from `ResourceConstructor` and all resource `NewResource` functions. Resources compute their own URI from the value they receive. For `mem.Resource` and `mem.Function`, introduce `mem.ResourceSpec` to carry identity fields (content type, qualifier) through the value parameter.

## Goals

1. **Eliminate unused parameter**: Most resources pass `""` for `resourceID` — it's dead weight in the signature.
2. **Resources own their URI**: Each resource type computes its URI from its input data, not from a pre-formatted string the caller must construct.
3. **Typed identity for mem resources**: Replace URI string parsing with a structured `mem.ResourceSpec`.

## Current State

| Resource | Uses resourceID? | Notes |
| --- | --- | --- |
| `file.Resource` | Partially | Falls back to `value` when empty; internal callers always pass `""` |
| `mem.Resource` | Yes | Parses `contentType` and `qualifier` from `mem:` URI |
| `mem.Function` | Yes | Parses `funcType` and `name` from `mem:function/` URI |
| `pkg.Resource` | No | Gen wrapper receives it but doesn't forward it |
| `appnet.Resource` | No | Gen wrapper receives it but doesn't forward it |
| `git.Resource` | No | Gen wrapper receives it but doesn't forward it |
| `json.Resource` | No | Gen wrapper receives it but doesn't forward it |
| `yaml.Resource` | No | Gen wrapper receives it but doesn't forward it |
| `service.Resource` | No | Gen wrapper receives it but doesn't forward it |

## Implementation Phases

### Phase 1: Introduce mem.ResourceSpec

Add `ResourceSpec` to `pkg/op/provider/mem` and adopt it in `mem.Resource` and `mem.Function` constructors. The existing `resourceID`-based constructors continue to work during this phase — the spec is an alternative construction path.

```go
// ResourceSpec carries the identity fields for constructing a mem.Resource.
type ResourceSpec struct {
    ContentType string // "callable", "json", "template", "function", etc.
    Qualifier   string // type-specific: "file.Reducer/myfn", "config", etc.
    Data        []byte // optional payload
}
```

- [ ] Define `ResourceSpec` in `pkg/op/provider/mem/resource.go`
- [ ] Add `NewResourceFromSpec(ctx *op.ExecutionContext, spec ResourceSpec) (Resource, error)` to `mem/resource.go`
- [ ] Add `NewFunctionFromSpec(ctx *op.ExecutionContext, spec ResourceSpec, fn *starlark.Function) (Function, error)` to `mem/function.go` — parses `funcType` and `name` from `spec.Qualifier`
- [ ] Migrate all callers of `mem.NewResource` and `mem.NewFunction` to use the spec variants
- [ ] Remove the old `resourceID`-based `NewResource` and `NewFunction` once all callers are migrated
- [ ] Update `mem` gen files (`resource.gen.go`) to use spec-based constructors
- [ ] Tests

**Files**:

- `pkg/op/provider/mem/resource.go` — Modify (add ResourceSpec, new constructor)
- `pkg/op/provider/mem/function.go` — Modify (new constructor)
- `pkg/op/provider/mem/gen/resource.gen.go` — Modify (update registration wrapper)
- `pkg/op/provider/mem/resource_test.go` — Modify

### Phase 2: Drop resourceID from ResourceConstructor

Change the `ResourceConstructor` signature from `func(ctx *ExecutionContext, resourceID string, value any) (any, error)` to `func(ctx *ExecutionContext, value any) (any, error)`. Update all resource constructors, gen files, and callers.

- [ ] Change `ResourceConstructor` type in `pkg/op/receiver_type.go`
- [ ] Update `AnnounceResource` in `pkg/op/receiver_registry.go`
- [ ] Update all `NewResource` signatures to drop `resourceID`:
  - `file.NewResource(ctx, value)` — value is the path string
  - `mem.NewResource(ctx, value)` — value is `ResourceSpec`
  - `mem.NewFunction(ctx, value)` — value is `ResourceSpec` (starlark.Function in spec.Data or separate)
  - `pkg.NewResource(ctx, value)` — already ignores resourceID
  - `appnet.NewResource(ctx, value)` — already ignores resourceID
  - `git.Resource`, `json.Resource`, `yaml.Resource`, `service.Resource` — same
- [ ] Update all `gen/resource.gen.go` files (template change + regenerate)
- [ ] Update the `resource.gen.go.template` in `star/extensions/com.noblefactor.devlore.Actions/templates/`
- [ ] Update all internal callers that pass `""` as resourceID
- [ ] Tests

**Files**:

- `pkg/op/receiver_type.go` — Modify (ResourceConstructor signature)
- `pkg/op/receiver_registry.go` — Modify (AnnounceResource)
- `pkg/op/provider/*/resource.go` — Modify (all resource constructors)
- `pkg/op/provider/*/gen/resource.gen.go` — Modify (all gen files)
- `star/extensions/com.noblefactor.devlore.Actions/templates/resource.gen.go.template` — Modify
- `pkg/op/provider/file/provider.go` — Modify (internal NewResource calls)
- `pkg/op/provider/encryption/provider.go` — Modify (internal NewResource call)

## Decisions

- `mem.Function` constructor takes `(ctx *ExecutionContext, value any)` where value is `mem.ResourceSpec`. The `*starlark.Function` goes in `ResourceSpec.Data` as `any`.
- `ResourceSpec` lives in `pkg/op/provider/mem`.
