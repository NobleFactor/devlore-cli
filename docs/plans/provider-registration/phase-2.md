---
title: "Phase 2: Flow Actions"
status: draft
created: 2026-03-06
updated: 2026-03-06
parent: ../provider-registration.md
---

# Phase 2: Flow Actions

## Summary

Create a handwritten flow provider descriptor that uses the same
announce-and-callback model as generated resource providers. This
immediately fixes #188 (`flow.choose` not registered in `ActionRegistry`)
and proves the model works for non-generated providers.

Flow actions are handwritten. They register exactly the same way as
generated code — the descriptor struct implements the same `Provider`
interface and calls `op.Announce()` in `init()`. The only difference is
that a human wrote it instead of a code generator.

## Why Handwritten

Flow actions (`choose`, `gather`, `elevate`, `wait_until`) are control-flow
primitives, not resource operations. They don't follow the provider method
contract (compensable/non-compensable return signatures) and don't have
Starlark plan sub-namespaces. They are authored by hand and will stay that
way. But the registration path is identical.

## Deliverables

### 1. Flow provider descriptor (`internal/execution/flow/provider.go`)

```go
package flow

import (
    "github.com/NobleFactor/devlore-cli/pkg/op"
)

// flowProvider is the provider descriptor for flow control actions.
// Handwritten — same structure as generated provider descriptors.
type flowProvider struct{}

func (p *flowProvider) Name() string { return "flow" }

func (p *flowProvider) Register(reg *op.ActionRegistry, _ op.Context) {
    reg.Register(&Choose{})
    reg.Register(&Gather{})
    reg.Register(&Elevate{})
    reg.Register(&WaitUntil{})
}

func init() {
    op.Announce(&flowProvider{})
}
```

This replaces the current `flow.Register(reg)` free function in
`internal/execution/flow/register.go`.

### 2. Update blank imports

Add `internal/execution/flow` to the blank import list in
`pkg/op/provider/register.go` so the flow `init()` fires alongside
resource providers.

### 3. Verify fix for #188

After this phase, `op.InitAll(reg, ctx)` registers flow actions into the
same `ActionRegistry` as all resource actions. `plan.choose` calls
`reg.MustGet("flow.choose")` — it now succeeds.

## Tasks

- [ ] Create `internal/execution/flow/provider.go` — flow provider descriptor with `init()` calling `op.Announce()`
- [ ] Delete `internal/execution/flow/register.go` — the free `Register()` function is replaced by the descriptor
- [ ] Add `_ "github.com/NobleFactor/devlore-cli/internal/execution/flow"` to `pkg/op/provider/register.go` blank imports
- [ ] Update any direct callers of `flow.Register(reg)` (currently only `flow_test.go`) to use `op.InitAll` or direct action registration
- [ ] Add test: verify `op.Providers()` includes `"flow"` after import
- [ ] Add test: verify `op.InitAll` registers `flow.choose`, `flow.gather`, `flow.elevate`, `flow.wait_until`
- [ ] Verify `make check` passes
- [ ] Close #188

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `internal/execution/flow/provider.go` | Create | Flow provider descriptor |
| `internal/execution/flow/register.go` | Delete | Replaced by provider descriptor |
| `pkg/op/provider/register.go` | Modify | Add flow blank import |
| `internal/execution/flow/flow_test.go` | Modify | Update registration calls |

## Exit Criteria

- Flow actions register through `op.Announce()`/`op.InitAll()` — same path as resource providers
- `flow.Register()` is deleted — no separate registration function
- #188 is fixed: `flow.choose` is in the `ActionRegistry` in production
- All tests pass
