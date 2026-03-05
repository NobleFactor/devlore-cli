---
title: "Phase 2: ReconcilableAction and provider reconcile methods"
parent: ../reconciliation.md
status: draft
---

# Phase 2: ReconcilableAction and provider reconcile methods

## Summary

Add the `ReconcilableAction` interface and implement `ReconcileX` methods
on providers that manage persistent resources. Move checksum verification
from `CompensateX` methods to `ReconcileX` methods. Wire the reconcile
function into `PushAction` so that `Unwind` automatically performs the
reconcile→undo dance.

## Rationale

The architecture document (§1a) describes a three-method triangle:

```
ActionX          → forward action (Do)
CompensateActionX → mechanical reversal (Undo)
ReconcileActionX  → drift detection (Reconcile)
```

Today, the file provider's `CompensateBackup` and `CompensateMove` verify
checksums before restoring — reconciliation-like work embedded inside
compensation. This violates the principle that compensation is pure
mechanical reversal. The `ReconcileX` method is where drift detection
belongs.

## Interface

```go
// pkg/op/action.go
type ReconcilableAction interface {
    Action
    Reconcile(ctx *Context, state ReconciliationState) (drifted bool, err error)
}
```

An action that implements `CompensableAction` + `ReconcilableAction` has
the full lifecycle. The capability matrix:

| Combination | Meaning |
| --- | --- |
| `Action` only | Forward-only, no undo, no reconciliation |
| `Action` + `ReconcilableAction` | Forward-only but drift-detectable |
| `CompensableAction` only | Undoable but no drift detection |
| `CompensableAction` + `ReconcilableAction` | Full lifecycle |

## PushAction evolution

`PushAction` detects `ReconcilableAction` and binds the reconcile closure
alongside the compensate closure:

```go
func (s *RecoveryStack) PushAction(ctx *Context, action Action, undoState any, reconcileState any) {
    comp, ok := action.(CompensableAction)
    if !ok {
        return
    }
    var reconcile func(any) (bool, error)
    if rec, ok := action.(ReconcilableAction); ok {
        reconcile = func(state any) (bool, error) {
            return rec.Reconcile(ctx, state)
        }
    }
    s.Push(
        func(state any) error { ... },
        reconcile,
        undoState,
        reconcileState,
    )
}
```

Note: `PushAction` gains a 4th parameter `reconcileState any`. All call
sites updated (executor, choose, gather).

## Provider reconcile methods

### file provider

| Method | Reconciliation Data | Verification |
| --- | --- | --- |
| `ReconcileCopy` | `{Path, Checksum}` | Re-read file, compare SHA256 |
| `ReconcileLink` | `{LinkPath, TargetPath}` | `os.Readlink`, compare target |
| `ReconcileBackup` | `{RecoveryPath, Checksum}` | Verify backup file exists and hash matches |
| `ReconcileMove` | `{DestPath, Checksum}` | Re-read destination, compare SHA256 |

Checksum verification currently in `CompensateBackup` and `CompensateMove`
moves here. Those compensation methods become pure rename/restore operations.

### pkg provider

| Method | Reconciliation Data | Verification |
| --- | --- | --- |
| `ReconcileInstall` | `{PackageNames, Versions}` | Re-query package manager for installed versions |

### service provider

| Method | Reconciliation Data | Verification |
| --- | --- | --- |
| `ReconcileEnable` | `{Name, ExpectedState: "enabled"}` | Query `systemctl is-enabled` |
| `ReconcileDisable` | `{Name, ExpectedState: "disabled"}` | Query `systemctl is-enabled` |
| `ReconcileStart` | `{Name, ExpectedState: "active"}` | Query `systemctl is-active` |
| `ReconcileStop` | `{Name, ExpectedState: "inactive"}` | Query `systemctl is-active` |

### git provider

| Method | Reconciliation Data | Verification |
| --- | --- | --- |
| `ReconcileClone` | `{RepoPath, HEAD}` | Read `.git/HEAD` and compare |

### Not reconcilable

- `net.Download` — result flows to next node, no persistent resource
- `shell.Exec` — arbitrary side effects
- `template.Render`, `encryption.Decrypt` — pure transforms

## Tasks

- [ ] Add `ReconcilableAction` interface in `pkg/op/action.go`
- [ ] Update `PushAction` to accept `reconcileState` and bind reconcile closure
- [ ] Update all `PushAction` call sites (executor, choose, gather)
- [ ] Implement `ReconcileCopy` on file provider
- [ ] Implement `ReconcileLink` on file provider
- [ ] Implement `ReconcileBackup` on file provider
- [ ] Implement `ReconcileMove` on file provider
- [ ] Strip checksum verification from `CompensateCopy`, `CompensateBackup`, `CompensateMove`
- [ ] Implement `ReconcileInstall` on pkg provider
- [ ] Implement `ReconcileEnable`, `ReconcileDisable` on service provider
- [ ] Implement `ReconcileStart`, `ReconcileStop` on service provider
- [ ] Implement `ReconcileClone` on git provider
- [ ] Update `Do` methods to return reconciliation data as 3rd value
- [ ] Regenerate all `actions_gen.go` files

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/action.go` | Modify | Add `ReconcilableAction` |
| `pkg/op/recovery.go` | Modify | `PushAction` binds reconcile closure |
| `pkg/op/provider/file/provider.go` | Modify | `ReconcileX` methods, strip checksums from `CompensateX`, update `Do` returns |
| `pkg/op/provider/pkg/provider.go` | Modify | `ReconcileInstall`, update `Do` return |
| `pkg/op/provider/service/provider.go` | Modify | `ReconcileX` methods, update `Do` returns |
| `pkg/op/provider/git/provider.go` | Modify | `ReconcileClone`, update `Do` return |
| `internal/execution/executor.go` | Modify | Pass reconcile state to `PushAction` |
| `internal/execution/flow/choose.go` | Modify | Pass reconcile state to `PushAction` |
| `internal/execution/flow/gather.go` | Modify | Pass reconcile state to `PushAction` |
| `pkg/op/provider/*/actions_gen.go` | Regenerate | Wire reconcile method |
