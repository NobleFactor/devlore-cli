---
step: 27
title: "Caller id on the activation — Starlark call-site via Thread.CallFrame"
status: not-started — design settled 2026-06-18 (callerID, replacing the activation's unit reference)
proof_run: n/a (not started)
parent: ../../phase-8.md
---

# Step 27 — Caller id on the activation (`callerID`)

**Status:** `not-started`. Design settled (2026-06-18): the activation carries a `callerID string` that identifies the
caller of the dispatched provider method. In graph dispatch that's the dispatching unit's id; in Starlark dispatch it's
a call-site `file:line:col` synthesized from the thread's call stack.

## Framing — both ends are callers

A provider method (`file.Copy`, …) is the **callee**. What invokes it differs by mode, but both are **callers**:

- **Graph dispatch:** an `op.ExecutableUnit` (node) *binds* a provider method (an `Action`) plus its slots. Dispatching
  the unit executes the call it encodes — so the unit is a **graph-encoded call to a provider method**, the caller.
- **Starlark dispatch:** a `.star` line `file.copy(...)` is a **script-encoded call to a provider method**, the caller.

Both identify the caller; the difference is only representation (a unit id vs. a source position). The field's job is to
**identify** that caller, not to resolve it — resolution back to a unit object is a graph-only bonus. Hence the name
`callerID` (not `unitID`, which is true only for the graph; not `siteID`/`originID`, which collide with `RecoverySite` /
`op.Origin`).

## What this step delivers

1. **Rename the activation's unit reference to a caller id.** Change
   `NewActivationRecord(graph, unit ExecutableUnit, env)` → `NewActivationRecord(graph, callerID string, env)`; the
   activation stores `CallerID string` in place of `Unit ExecutableUnit`.
   - Graph dispatch: `CallerID` = the dispatching unit's `ID()`.
   - Starlark dispatch: `CallerID` = the call-site id (or `""` when no thread is in scope).
2. **Synthesize the Starlark call-site id.** Today immediate-mode dispatch has no caller id, so `.star`-produced
   resources are interned with an empty producer stamp. Derive a deterministic `file:line:col` from the Starlark thread
   and use it as `CallerID`.

## Deriving the Starlark call-site (cheap)

- The thread is already handed to the dispatcher and currently **discarded**:
  `func (g *goReceiver) dispatch(_ *starlark.Thread, …)` (`pkg/op/starlarkbridge/go_receiver.go:560-561`). Name it.
- Reuse the in-repo pattern: `thread.CallStack()[last].Pos.String()` → `file:line:col` (already used at
  `cmd/devlore-test/devloretest/trace.go:47-51`). Same source location every run → a deterministic caller id. Build the
  activation with it at `go_receiver.go:761`.

## Producer stamping (the payoff)

`resource.producerID := activation.callerID`. The caller that invoked a *producing* method is exactly that resource's
producer, so the existing `ResourceBase.producerID` is the caller id, viewed from the resource's side.

- `NewResource`/`GetOrCreate` take a `producerID string` (= `activation.CallerID`). They already only read `Unit.ID()`
  today (per the `producerID = activationRecord.Unit.ID()` doc comments across file/json/yaml/git/mem/function/service/
  appnet/pkg). Mechanical, ~12 sites.

**Debuggability (the motivation).** Because `ProducerID()` is a string and the Starlark caller id is `file:line:col`,
inspecting a `.star`-produced resource in a debugger shows its origin directly:

```
resource.ProducerID() → "mkfile.star:42:8"   // "created by the call at line 42"
```

A strict improvement over today's empty stamp, at no cost beyond this step. Optionally fold the caller id into
`file.Resource.String()` (`resource.go:372`) so the provenance shows without drilling into `ProducerID()`.

## Typed-unit consumers (the only obstacle — already solved)

Four sites need the *object*, not the id — flow `Gather`/`Subgraph` type-assert `activation.Unit.(*op.Subgraph)`
(`pkg/op/provider/flow/provider.go:204`, `:364`) and `pkg/op/method.go:545`/`:557` pass it to compensation. They resolve
it via `activation.Graph.ResolveExecutable(callerID)` — **the accessor already exists** (`pkg/op/graph.go:575`) — and
only run in graph dispatch (Graph non-nil), so the lookup is always available. The Graph/Unit "both nil or both set"
pairing invariant (`pkg/op/activation_record.go`) dissolves: `CallerID` is always a string; `Graph` stays optional.

## Caveats (semantic, not engineering)

1. **Call-site ≠ per-invocation.** `Pos` is a source location; a Starlark `for` loop calling `file.copy(...)` on one
   line yields the same caller id for every iteration (the graph gives each iteration a distinct unit). This is
   call-site lineage, not dynamic-invocation lineage. `CallStack()` exposes the full chain if richer trace metadata is
   wanted later (same API as `trace.go`).
2. **Starlark-only.** Exists only when a `*starlark.Thread` is in scope. CLI/test/Go dispatch has none → still empty; the
   eager-property path already calls `dispatch(nil, …)` (`go_receiver.go:233`) → nil thread, no caller id.
3. **Frame depth.** Select the innermost *script* frame (skip any builtin frames).

## Relationship to other steps

Independent of step 26 and step 24. It introduces a *second* representation of caller identity (source-position-based)
alongside the graph's unit-id representation — both serving the one role of identifying the caller.

## Cost

Low. The call-site string is a few lines (name the thread param, reuse the `trace.go` pattern). The rename is ~12
mechanical `producerID string` constructor edits + 4 resolve-via-`ResolveExecutable` rewrites, and it *removes* the
pairing invariant rather than adding to it.

## Exit

The activation carries `CallerID string`; `.star`-produced resources carry a deterministic call-site `producerID` visible
in a debugger; graph dispatch is unchanged; tests assert the stamp for both a single call and a loop (documenting the
call-site-vs-invocation semantics); full `make test` green.
