---
title: "Phase 4: Reconciliation engine"
parent: ../reconciliation.md
status: draft
---

# Phase 4: Reconciliation engine

## Summary

Build the `ReconciliationStore` that captures resource state snapshots from
execution events, and wire it into `writ reconcile`. The existing
filesystem-based drift detection in `internal/writ/reconcile` is preserved
as a fallback for scenarios where no execution receipt exists.

## Rationale

Two complementary drift detection approaches serve different scenarios:

| Scenario | Approach | Data source |
| --- | --- | --- |
| Receipt exists | Action-based reconciliation | Stored `ReconciliationData` + action's `Reconcile` method |
| No receipt | Filesystem-based reconciliation | `internal/writ/reconcile.ScanTarget` — reads symlinks and files directly |

The "no receipt" scenario is real: a user might delete their receipts
directory, or run reconciliation on a machine that was configured manually.
The filesystem scanner detects symlink/copy state without needing prior
execution history.

## ReconciliationStore

```go
// internal/execution/reconcile.go

type ReconciliationStore struct {
    entries map[string]StoreEntry // keyed by resource URI
}

type StoreEntry struct {
    NodeID             string
    ActionName         string
    ReconciliationData any
    Timestamp          time.Time
}
```

The store implements `EventSink`. During deploy/upgrade, it captures
`ReconciliationPayload` from each `ExecutionEvent`. It is persisted
alongside the graph receipt.

## Reconciliation flow

```
writ reconcile
    |
    +-- receipt exists?
    |       |
    |       yes --> load ReconciliationStore from receipt
    |       |       for each entry:
    |       |           action.Reconcile(ctx, entry.ReconciliationData)
    |       |               --> (drifted, err)
    |       |       report drift
    |       |
    |       no --> fall back to filesystem scan
    |               reconcile.ScanTarget(sourceDir, targetDir)
    |               report state (linked, missing, orphan, etc.)
    |
    +-- --fix requested?
            |
            yes --> re-execute drifted resources
```

## Coexistence with internal/writ/reconcile

The filesystem-based reconciler (`internal/writ/reconcile`) is not replaced.
It continues to serve as:

1. **Fallback** when no receipt exists
2. **Quick scan** for symlink health (faster than loading a full receipt)
3. **Discovery** — finds what was deployed without needing the original graph

The action-based reconciler is the **primary** path when a receipt exists,
because it verifies resource state through the action's own verification
logic (content hashes, package versions, service state) rather than
filesystem heuristics.

## Tasks

- [ ] Implement `ReconciliationStore` with `EventSink` interface
- [ ] Implement store persistence (serialize/deserialize alongside receipt)
- [ ] Wire store as an `EventSink` during deploy/upgrade
- [ ] Implement action-based reconciliation path in `writ reconcile`
- [ ] Preserve filesystem-based fallback path
- [ ] Add `--fix` flag to re-execute drifted resources
- [ ] Add drift report output (table of resource URI, expected state, actual state)

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `internal/execution/reconcile.go` | Create | `ReconciliationStore` with `EventSink` |
| `internal/writ/reconcile/reconcile.go` | Modify | Add action-based path alongside filesystem path |
| `internal/writ/commands.go` | Modify | Wire reconciliation into deploy/reconcile/upgrade |
