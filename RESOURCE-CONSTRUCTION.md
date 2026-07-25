# Plan-time vs run-time resource coercion

Two discrete sets of scenarios, sharply distinct rules.

## Plan time

**Inputs are strings; outputs are pending Resources. The catalog mediates everything.**

### String parameter → Resource (input)

`NodeBuilder.fillSlot`: parameter type is a registered Resource type, slot value is a string from `.star`.

1. Use the string as the URI.
2. `catalog.GetOrCreate(uri, factory)` — factory closure constructs a fresh Resource of the parameter's concrete type using `NodeBuilder.ctx`.
3. If catalog already has a shadowed entry for this URI (a producer node ran shadowing first), GetOrCreate returns the canonical entry with originID stamped — the producer→consumer edge is implicit.
4. If catalog has no entry, GetOrCreate interns a discovery entry (`state=unresolved`); pre-flight will resolve at run time.
5. Slot stores the canonical Resource.

No I/O. Pure type-tagging plus catalog identity assignment.

### Method return value → Resource (output)

`NodeBuilder.dispatch` after node creation, before invocation registration: provider method's declared return type is a Resource, the planner doesn't have the actual return value yet (execution is deferred), but it knows the *intended* output URI from the call's positional/kwarg slot pattern.

1. Per the plan doc's `shadowOutputParam`/`shadowPendingOutput` convention: the destination Resource argument by parameter-position convention identifies the output URI.
2. `catalog.Shadow(resource, node.ID())` — registers `state=pending` with origin = this node's ID.
3. Conflict detection fires here: two nodes shadowing the same URI is a plan-time error (catalog rejects different origins).

No I/O. Pure dependency-edge construction.

## Run time

**Resources arrive already-cataloged-and-typed; metadata is populated against the target machine.**

### Pre-flight (before any node runs)

Iterate the catalog ledger. For each `state=unresolved` entry: stat against the target machine (file.Resource → os.Stat; service.Resource → systemd query; etc.); fail-fast if a discovery URI doesn't exist. `state=pending` entries are skipped — they'll be populated by their producer node.

### Per-node dispatch

Slot values are typed Resources with `state=resolved` (inputs) or `state=pending` (outputs). The action's reflected method receives them directly via `Method.Invoke` — no string-to-Resource conversion ever happens at run time. `op.Convert`'s cascade may run for non-Resource type adjustments, but Resources flow through assignability (level 1).

### Post-dispatch

Provider returned a Resource. `executor.go:471, 474` calls `catalog.Shadow` again, but now with the actual returned instance. Catalog updates the pending entry to resolved with metadata; downstream consumers waiting on this URI's promise see the resolved state when they execute.

## What this means for `op.Convert`

`op.Convert` does **not** know about Resources. Resource construction is a catalog operation, not a type cascade. The slice/map/SourceConverter/TargetConverter cascade we built handles every Go→Go projection that doesn't touch external state.

For the immediate-mode codegen failure (`string → *file.Resource`), the codegen runtime is doing a plan-time-shaped operation (taking string args, building structures) but without a graph. The clean answer: **codegen scripts run in plan mode against an in-memory catalog scoped to that codegen run.** Same `catalog.GetOrCreate` path; the Runtime owns a Catalog (it already does — `runtime.ctx.Catalog`); fillSlot calls into it. No new mechanism, no `op.Convert` change.

## Work to do

- **Plan-mode change in `NodeBuilder.fillSlot`**: when slot type is a Resource type and source is a string, call `ctx.Catalog.GetOrCreate(string, factoryFor(slotType))`. Factory mapping is the registry-derived constructor for the Resource type. Fail with a clean error if no factory is registered.

- **No `op.Convert` change.** It stays as the generic Go→Go cascade for non-Resource conversions.

- **No run-time change.** Slots already hold typed Resources by the time `Method.Invoke` runs.