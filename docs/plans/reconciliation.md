---
title: "Audit, Reconciliation, and Recovery — Implementation Plan"
issue: https://github.com/NobleFactor/devlore-cli/issues/156
status: draft
created: 2026-03-05
updated: 2026-03-05
---

# Plan: Audit, Reconciliation, and Recovery

## Summary

Complete the audit, reconciliation, and recovery framework described in the
[architecture document](../architecture/5.1-reconciliation.md).
The `RecoveryStack` and executor migration are done. This plan covers the
remaining work: the 4-value `Action.Do` return, the `ReconcilableAction`
interface, provider reconcile methods, the `ExecutionEvent` envelope, the
audit ledger, and the reconciliation engine — while preserving the existing
filesystem-based drift detection in `internal/writ/reconcile`.

## Goals

1. **Action lifecycle triangle**: Every compensable action gains a `ReconcileX`
   method that verifies resource state without re-executing `Do`.
2. **Structured audit**: Replace ad-hoc node status tracking with a durable
   `ExecutionEvent` stream that captures timing, resource identity, and outcome.
3. **Drift detection at two levels**: Action-based reconciliation for
   receipt-backed verification; filesystem-based reconciliation
   (`internal/writ/reconcile`) as a fallback when no receipt exists.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `RecoveryStack` | ✅ Done | `pkg/op/recovery.go` — `PushAction`, `Push`, `Do`, `Unwind`, `Discard`, `ErrDrifted` |
| Executor migration | ✅ Done | `executor.go`, `choose.go`, `gather.go` use `op.RecoveryStack.PushAction` |
| `Action.Do` signature | 3-value | Returns `(Result, UndoState, error)` — needs 4th `ReconciliationState` |
| `ReconcilableAction` | ❌ Missing | No interface for action-level drift detection |
| `ExecutionEvent` | ❌ Missing | No structured event envelope |
| Audit ledger | ❌ Missing | Node status baked into `Graph.Nodes[].Status` |
| Provider reconcile methods | ❌ Missing | Checksums live inside `CompensateX` — should move to `ReconcileX` |
| Provider error prefixes | ⚠️ Present | Service provider prefixes errors with `"compensate"` — belongs at boundary |
| `internal/writ/reconcile` | ✅ Working | Filesystem-based drift detection; independent of execution graph |
| Code generator | ✅ Working | Star templates generate action bindings; needs triangle enforcement |

## Implementation Phases

1. [**Action.Do 4-value return**](reconciliation/phase-1.md) — Extend the action
   interface to return `ReconciliationState` as a 4th value; update all
   implementations.

2. [**ReconcilableAction and provider reconcile methods**](reconciliation/phase-2.md) —
   Add the `ReconcilableAction` interface; implement `ReconcileX` methods on
   providers that manage persistent resources; strip checksums from `CompensateX`.

3. [**ExecutionEvent and audit ledger**](reconciliation/phase-3.md) — Introduce the
   structured event envelope; implement the audit ledger as an `EventSink`.

4. [**Reconciliation engine**](reconciliation/phase-4.md) — Build the
   reconciliation store; wire it into `writ reconcile` alongside the existing
   filesystem-based fallback.

5. [**Code generator triangle enforcement**](reconciliation/phase-5.md) — Update
   star templates to detect and enforce the `ActionX` / `CompensateActionX` /
   `ReconcileActionX` triangle.

6. [**Error prefix cleanup**](reconciliation/phase-6.md) — Remove boundary-layer
   context from provider error messages; move phase-aware wrapping to the executor.

## Files to Create/Modify

| File | Phase | Action | Purpose |
| --- | --- | --- | --- |
| `pkg/op/action.go` | 1, 2 | Modify | 4-value Do, `ReconciliationState`, `ReconcilableAction` |
| `pkg/op/recovery.go` | 2 | Modify | `PushAction` binds reconcile for `ReconcilableAction` |
| `pkg/op/event.go` | 3 | Create | `ExecutionEvent`, payloads, `EventSink` |
| `internal/execution/executor.go` | 1, 3, 6 | Modify | 4-value return, event emission, error wrapping |
| `internal/execution/audit.go` | 3 | Create | `AuditLedger` |
| `internal/execution/reconcile.go` | 4 | Create | `ReconciliationStore` |
| `internal/execution/flow/choose.go` | 1 | Modify | 4-value inner Do |
| `internal/execution/flow/gather.go` | 1 | Modify | 4-value inner Do |
| `pkg/op/action_reflect.go` | 1 | Modify | 4-value reflected return |
| `pkg/op/provider/file/provider.go` | 1, 2 | Modify | 4-value Do, `ReconcileX`, strip checksums from `CompensateX` |
| `pkg/op/provider/pkg/provider.go` | 1, 2 | Modify | 4-value Do, `ReconcileInstall` |
| `pkg/op/provider/service/provider.go` | 1, 2, 6 | Modify | 4-value Do, `ReconcileX`, strip error prefixes |
| `pkg/op/provider/git/provider.go` | 1, 2 | Modify | 4-value Do, `ReconcileClone` |
| `internal/writ/reconcile/reconcile.go` | 4 | Modify | Action-based path alongside filesystem path |
| `internal/writ/commands.go` | 4 | Modify | Wire reconciliation |
| `star/extensions/.../templates/graph_actions.go.template` | 5 | Modify | Emit `Reconcile`, triangle validation |

## Related Documents

- [Architecture: Reconciliation](../architecture/5.1-reconciliation.md)
- [Plan: Compensation](./compensation.md)
- [Plan: Resource Management](./resource-management.md)
- Issue #156 — Audit, Reconciliation, and Recovery in the Execution Graph

## Open Questions

- [ ] How should the reconciliation store be persisted? Options: embedded in
  the graph receipt, separate index file, or SQLite database.
- [ ] Should `ReconciliationData` be typed per action (e.g., `FileCopyRecon{Path, Hash}`)
  or remain opaque `any`? Typed gives compile-time safety; opaque gives
  flexibility and simpler generator output.
- [ ] What is the TTL for recovery data? Should `RecoveryPayload` carry an
  expiry hint so that old undo state can be garbage collected?
- [ ] Should the audit ledger replace the graph receipt's `Node.Status`
  approach, or should it be a separate artifact?
- [ ] Can the `EventSink` pattern replace `LifecycleHook` entirely, or do hooks
  serve a distinct purpose (external observation vs internal coordination)?
- [ ] Self-healing loop: should the Coordinator run periodic `Reconcile` sweeps
  automatically, or only on explicit `writ reconcile` invocation?
- [ ] Should `net.Download` and `shell.Exec` produce reconciliation data?
  Architecture says no (no persistent resource / arbitrary side effects),
  but specific use cases may warrant it.
