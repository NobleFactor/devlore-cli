# Recovery Stack Serialization & Restart

Design topic for a future plan. Not yet approved for implementation.

## Problem

When a phase fails and retries are exhausted, the executor unwinds the recovery
stack and the process exits. The operator has no option to inspect the failure,
fix the root cause, and resume execution from the point of failure. The graph
and its accumulated state are lost.

## Restart Scenario

1. Phase N fails after exhausting retries
2. Executor offers: abort (unwind) or pause (save state)
3. Operator chooses pause — executor serializes execution state to disk
4. Operator troubleshoots (fixes config, restores connectivity, etc.)
5. Operator invokes restart — executor loads saved state and resumes from Phase N

## Design Surface

### RetryPolicy Extension

RetryPolicy gains a `pause` (or `manual`) backoff strategy. When a phase fails
and the policy specifies `pause`, the executor saves state and yields to the
operator instead of unwinding. This fits within the existing retry contract —
it's a backoff strategy that waits indefinitely for human intervention.

```go
const (
    BackoffNone        BackoffStrategy = "none"
    BackoffLinear      BackoffStrategy = "linear"
    BackoffExponential BackoffStrategy = "exponential"
    BackoffPause       BackoffStrategy = "pause"  // yield to operator
)
```

### Recovery Stack Serialization

For restart to work across process boundaries, the recovery stack must
round-trip through YAML/JSON:

- RecoveryEntry references a Node (by ID, not pointer) and stores UndoState
- UndoState is `any` — providers must ensure their state marshals correctly
  (this constraint already exists for receipts)
- GatherUndoState includes per-iteration proxy contexts and recovery entries

### Results Map Serialization

The results map (`map[string]any`) holds node Results needed for promise slot
resolution. For restart, downstream nodes need access to results from
already-completed nodes. Options:

- Serialize the full results map alongside the recovery stack
- Re-derive results from the receipt (if receipts capture enough)
- Hybrid: serialize only results referenced by pending promise slots

### Execution Checkpoint

A checkpoint captures everything needed to resume:

- Graph definition (already serializable)
- Phase progress (which phases completed, which failed)
- Recovery stack (for undo if restart also fails)
- Results map (for promise resolution of remaining nodes)
- Context.Data keys that are serializable (not functions)

### Open Questions

- How does restart interact with GatherUndoState? If a gather is mid-iteration,
  can it resume from the failed iteration or must it restart the whole gather?
- Should checkpoint be automatic (every phase boundary) or explicit (only on
  pause)?
- How do function-valued slots (decryptor, validators) get re-populated on
  restart? They come from Context.Data which is built from CLI flags and config.
- Does the receipt format need to change to support restart metadata?
