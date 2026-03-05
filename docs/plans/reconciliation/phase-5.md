---
title: "Phase 5: Code generator triangle enforcement"
parent: ../reconciliation.md
status: draft
---

# Phase 5: Code generator triangle enforcement

## Summary

Update the star code generator to detect and enforce the three-method
triangle: `ActionX`, `CompensateActionX`, `ReconcileActionX`. The
generator verifies type signature chains and emits `Reconcile` methods
on generated action structs.

## The triangle

For a provider method `Copy`:

```go
// Forward action — Do
func (p *Provider) Copy(source, dest string) (result string, undo Tombstone, reconcile CopyRecon, err error)

// Compensation — Undo
func (p *Provider) CompensateCopy(state Tombstone) error

// Reconciliation — Reconcile
func (p *Provider) ReconcileCopy(state CopyRecon) (bool, error)
```

The generator enforces:

1. **Type chain**: The 3rd return type of `Copy` (`CopyRecon`) must match
   the 1st parameter type of `ReconcileCopy` (`CopyRecon`).
2. **Completeness**: If `ReconcileCopy` exists but `Copy` doesn't return
   a 3rd non-error value, the build fails.
3. **Optionality**: `ReconcileCopy` is not required. If absent, the action
   is not reconcilable.

## Template changes

`graph_actions.go.template` currently generates action structs that
implement `Action` (via `Do`) and optionally `CompensableAction` (via
`Undo`). It gains a third arm:

```go
// If ReconcileX exists on the provider
func (a *copyAction) Reconcile(ctx *op.Context, state op.ReconciliationState) (bool, error) {
    typed, ok := state.(CopyRecon)
    if !ok {
        return false, fmt.Errorf("reconcile: expected CopyRecon, got %T", state)
    }
    return a.provider.ReconcileCopy(typed)
}
```

## Method discovery

The star extension's provider analysis phase already discovers `ActionX`
and `CompensateActionX` methods. It gains a third pass:

1. For each `ActionX`, check if `ReconcileActionX` exists
2. If yes, verify the type signature chain
3. Record the reconcile method for template emission

## Tasks

- [ ] Extend provider method discovery to detect `ReconcileX` methods
- [ ] Verify type signature chain (3rd return of `ActionX` = 1st param of `ReconcileX`)
- [ ] Update `graph_actions.go.template` to emit `Reconcile` method
- [ ] Generator refuses to build if reconciliation chain is broken
- [ ] Regenerate all `actions_gen.go` files

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `star/extensions/com.noblefactor.devlore.Actions/templates/graph_actions.go.template` | Modify | Emit `Reconcile` method |
| `star/extensions/com.noblefactor.devlore.Actions/` | Modify | Triangle validation in provider analysis |
| `pkg/op/provider/*/actions_gen.go` | Regenerate | Final generation with reconcile support |
