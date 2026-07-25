# The Emergent System Model

> **Status:** vision document, restated 2026-07-22 (phase-8 step 51, slice 2) onto the landed `pkg/op` model. The
> forward-looking content — the dependency taxonomy, distributed orchestration, the emergent receipt graph, the
> package planner — remains vision; what changed is that every claim about the *current* system now describes the
> landed one (units + saga receipts, the sealed `Binding` set, the trace) instead of the retired runtime-phase /
> `Proxy` / `Context.Data` / Tombstone vocabulary. Companion: [`1-system-model.status.md`](1-system-model.status.md).

This document formalizes the architecture of devlore's execution model as it scales from local workstation
management to distributed system orchestration. The core thesis: **the execution graph is the system model.** There
is no separate state database, inventory file, or desired-state declaration. The model emerges from verified
execution records.

See also:

- [Execution Graph](2-execution-graph.md) — the sealed graph and the graph-vs-trace document split
- [Phase Execution](2.2-phase-execution.md) — the saga machine, receipts, compensation
- [Typed Slots](2.1-typed-slots.md) — the binding model and conversion
- [Orchestration Primitives](2.3-orchestration-primitives.md) — subgraphs and the flow combinators
- [Receipt Integrity](5-receipt-integrity.md) — checksum and signature

---

## 1. Design Thesis

Traditional configuration management tools maintain a **state file** that represents what the system should look
like. The state file is an indirect claim about reality — it says "Terraform applied these resources" or "Ansible
ran these tasks." If reality diverges from the state file (manual changes, partial failures, clock skew), the file
becomes a liability.

This architecture inverts the model. Instead of "desired state → apply → hope," it implements "execute logic →
capture the record → verify truth."

| Aspect | State-File Model (Terraform, Ansible) | Emergent Model (devlore) |
|---|---|---|
| Source of truth | Static file, can drift | The verified trace (execution record) |
| Logic | DSL/YAML (HCL, Jinja) | Code (Go providers, Starlark plans) |
| Inventory | Pre-defined, static | Emerges at execution time |
| Drift detection | External tool, expensive refresh | Re-verify against the recorded content identity |
| Concurrency | Limited, side-effect prone | Safe — DAG by construction |
| History | Scattered (git logs, CI, SSH) | Embedded — receipts in the trace, the run index |
| Recovery | Full wipe-and-reinstall | Receipt-guided surgical compensation |

The trace is not a log. It is a structured, checksummed, signable record of the exact plan, dependency resolution,
and per-dispatch outcomes that produced the current state of a machine ([2](2-execution-graph.md): the graph is the
plan, the trace is the record). The collection of traces across all machines in a fleet IS the system model.

---

## 2. Dependency Taxonomy

The system's intelligence is rooted in three distinct dependency types. Each type creates a different kind of edge
in the execution graph, carries different lifecycle semantics, and requires different failure handling.

### 2.1 Structural Dependencies — Composition

**Definition.** A relationship where one subsystem contains or manages another. Established by nesting — a subgraph
containing subgraphs ([2.3](2.3-orchestration-primitives.md)).

**Graph semantics.** A compositional edge. The child's record nests within the parent's — the trace's recovery tree
is exactly this recursive structure (a stamped substack per completed subgraph).

**Lifecycle impact.** If a child fails, the parent is structurally incomplete. Decommissioning a parent
decommissions its children first (depth-first). Re-verification propagates downward.

### 2.2 Functional Dependencies — Environment

**Definition.** A prerequisite that must exist in the host environment for a subsystem to function. Established by
`plan.pkg.install(...)` and kin.

**Graph semantics.** A prerequisite edge. Multiple subsystems may share a functional dependency; it is only safe to
remove when the last dependent is decommissioned.

**Lifecycle impact.** Subject to version intersection (Section 5, vision), reference counting, and batch
optimization. A vulnerability in a functional dependency traces to every subsystem depending on it.

### 2.3 Procedural Dependencies — Temporal Ordering

**Definition.** A temporal requirement where one action cannot begin until another has produced a specific result.
Created by **promise bindings** ([2.1](2.1-typed-slots.md)): a unit's slot references an upstream unit's result, and
the dependency edge derives from that reference at graph seal.

**Graph semantics.** A blocking edge — toposort guarantees the producer dispatches first, and the promise resolves
against its recorded result.

**Lifecycle impact.** Controls ordering. If a procedural dependency fails, the failure adjudicates through retry,
the error action, and the transition policy ([2.2](2.2-phase-execution.md)); the failed dispatch's receipt is
retained in the trace for diagnostics.

### Summary

| Type | Focus | Mechanism | Failure Effect |
|---|---|---|---|
| Structural | Hierarchy | Nested subgraphs | Parent incomplete |
| Functional | Environment | `plan.pkg.install(...)` | Subsystem cannot run |
| Procedural | Sequence | Promise bindings | Branch blocked |

---

## 3. Deployment Lifecycle — Phases at Plan Time, the Truth Gate at Run Time

A deployment conventionally progresses prepare → install → provision → verify. In the landed model this pipeline is
a **plan-time construction concern**: lifecycle scripts contribute per-phase subgraphs that assemble into one sealed
graph ([2.5](2.5-lifecycle-pipeline-construction.md)). At run time there are no phase walks — only units, subgraph
boundaries, and the saga machine ([2.2](2.2-phase-execution.md)); "phase" at run time means the run-lifecycle
dimension of `RunStatus`.

What survives — and matters — from the four-phase picture:

1. **Verify is the truth gate.** A verification step's recorded result (e.g. "port 80 responsive") is what
   distinguishes this model from traditional configuration management: the record captures verified facts, not just
   "success." Observations — point-in-time resource facts (`file.observe`, `git.observe`) — ride the trace as
   results ([4](4-resource-management.md) §6.1).
2. **Every completed step can be undone.** The classical (A, C, S) tuple maps onto the landed compensable-action
   contract: forward method, `Compensate<Name>` companion, and the **receipt** as S — captured per dispatch, pushed
   on the recovery stack, unwound LIFO on failure ([2.2](2.2-phase-execution.md) owns the contract in full).
3. **Failure leaves evidence.** A failed run's trace persists — receipts, the transition journal, the terminal
   `RunStatus` — so the record of what happened is permanent, whether or not compensation ran
   (`completed × degraded`, `stopped × execution_failed`, and the rest of the terminal grid).

---

## 4. Data Flow — the Binding Model

Every unit input is a named slot filled by a binding from the sealed set ([2.1](2.1-typed-slots.md) owns this in
full):

1. **Immediate** — a value known at plan time.
2. **Promise** — an upstream unit's result, resolved at dispatch; the source of dependency edges.
3. **Variable** — a named runtime value resolved against the run's variable surface (declared parameters resolved
   from the application's sources, layered frames, gather's per-iteration `item`, field projection).

The pre-`op` `Proxy` slot variant and the `Context.Data` property bag are gone — gather iteration flows through
variable frames, and runtime configuration arrives by variable resolution, so a serialized graph carries bindings
(the complete plan) and never runtime state.

---

## 5. Package Planning — The Functional Dependency Solver *(vision)*

On flat host environments where containers or VMs are impractical, the system employs a reference-counting package
planner operating at plan time, before dispatch. **Status: not built.** The landed foundation is the platform
contract and the Composite package-manager router ([3.4](3.4-platform-package-managers.md)); the solver below
remains design.

### 5.1 Reference Counting

Multiple subsystems may require the same package. The planner tracks active references to prevent premature
uninstalls: `Acquire` increments (two requesters, one INSTALL action); `Release` decrements; uninstall only at zero
AND ephemeral.

### 5.2 Ephemeral vs. Persistent Dependencies

| Type | Meaning | Uninstall policy |
|---|---|---|
| **Persistent** | Required for the subsystem's lifetime | Never auto-uninstalled |
| **Ephemeral** | Required only during a step (e.g., curl for download) | Uninstalled when ref count = 0 |

**Promotion rule:** if any subsystem requires persistence, persistent wins.

### 5.3 SemVer Intersection

When multiple subsystems constrain the same package: gather requirements recursively, intersect ranges
(`[max(minimums), min(maximums)]`), fail the plan on an empty intersection naming the incompatible subsystems, else
select the highest version in range — all during construction, before any dispatch.

### 5.4 Upgrade Safety

| Scenario | Action |
|---|---|
| New minimum within old maximum | Upgrade, then re-verify all dependents |
| New minimum exceeds old maximum | Reject — isolation boundary required |
| Ephemeral promoted to persistent | Lock — package marked non-removable |

An upgrade triggers a recursive re-verify of every dependent; failures route through the retry / transition
policies.

### 5.5 Batch Optimization

Install→uninstall pairs batch; install→upgrade skips the initial install; re-required packages are not uninstalled
within the batch window. **Record integrity rule:** an optimized-away install still records that the requirement was
satisfied — drift detection must not be confused by actions that were legitimately skipped.

---

## 6. Orchestration Flow

### 6.1 Local Execution (Single Machine) — *landed*

1. **Graph construction.** Starlark plans (or Go callers) register detached invocations; assembly seals the graph —
   edges from promises, toposort, plan-time type check, orphan scan ([3.5.3](3.5.3-plan-provider.md),
   [2](2-execution-graph.md)).
2. **Persist the plan.** The graph document is written (and signable) before execution.
3. **Execute.** `op.GraphExecutor` dispatches the unit tree — each subgraph under its own child executor —
   resolving promises against recorded results, honoring retry / error-action / transition policies.
4. **Compensate on failure.** The recovery tree unwinds LIFO, receipt by receipt.
5. **Record.** The trace — receipts, transition journal, catalog snapshot with content identity, terminal
   `RunStatus` — persists win or lose, indexed by the run index.

### 6.2 Distributed Orchestration (Multi-Node) — *vision*

The distributed model extends local execution with a coordinator-node handshake:

1. **Graph construction.** The coordinator builds the global graph; each global node is a machine's subgraph.
2. **Subgraph dispatch.** The coordinator sends subgraphs to target machines (targets may be discovered
   dynamically).
3. **Execution.** Each machine runs its subgraph through the landed local pipeline; failures are recorded in that
   machine's trace.
4. **Record collection.** The coordinator gathers traces and merges them into the global emergent model.

The same primitives operate at both scales:

| Local (single machine) | Global (distributed) |
|---|---|
| Unit = action dispatch | Node = machine subgraph |
| Edge = data dependency | Edge = cross-machine dependency |
| Subgraph = saga boundary | Subgraph = deployment wave |
| Gather = parallel items | Gather = parallel machine provisioning |
| Choose = planner-built branch | Choose = platform-specific provisioning |
| Result = dispatch output | Result = machine trace |
| Promise = upstream ref | Promise = cross-machine data ref |

### 6.3 Interface Nodes — *vision*

Subgraph boundaries use **Interface Nodes**: an **Input Node** blocks its children until the coordinator injects a
value (resolved from an upstream machine's trace); an **Output Node** exports a verified value to the coordinator
for propagation to downstream machines holding a promise on it.

### 6.4 Cross-Node Data Flow — *vision*

Machine 1 verifies and records a result → the coordinator extracts it from the trace → injects it into machine 2's
Input Node → machine 2 unblocks. Wait-state strategy: reactive push — the coordinator scans for waiting downstream
machines as each trace arrives.

---

## 7. The Emergent Model — The Record Graph

### 7.1 What the Model Is

The emergent model is the collection of all traces: a directed graph whose vertices are execution records and whose
edges are the three dependency types. There is no separate database — to know the state of the system, traverse the
records.

### 7.2 What a Record Holds — *landed locally*

The trace ([2](2-execution-graph.md), [5.2](5.2-recovery-serialization.md)) records per run:

- **Per-dispatch receipts** — the recovery tree: what each action did, sufficient to undo it; where execution
  stopped on failure, enabling surgical remediation instead of wipe-and-reinstall.
- **The transition journal + terminal `RunStatus`** — when and why the run's condition flipped.
- **The catalog snapshot with content identity** — Etag + Digest per active resource (phase-8 step 48): the
  as-deployed record that drift attribution reads.
- **Checksum and signature** — the graph document's checksum covers canonical content; signing is
  `pkg/signing` + `writ verify` ([5](5-receipt-integrity.md)).

The run index (`internal/cli`) folds records over time — `writ status` reads it today; the fleet-level record graph
above is the same idea at distributed scale.

### 7.3 Drift Detection

Re-verification against the recorded content identity provides drift detection: matching identity means the record
is still accurate; divergence means drift — attributable as source-changed vs. target-modified from the recorded
Etag/Digest (steps 47/48 landed exactly this for `writ status` / `upgrade`). The same code that deployed the system
re-verifies it — a native capability, not an external agent.

### 7.4 Traceability

Every component traces back to: the plan that declared the intent (the graph document, with its origin), the
constraints that selected versions (vision, §5), the per-dispatch outcomes (receipts), the upstream dependencies
that had to succeed first (the recovery tree + promises), and the checksummed, signable record of it all.

### 7.5 Permanent Failure Records

A failed dispatch's receipt and the run's journal are retained in the trace — the record of failure is permanent,
and every trace persists win or lose (the run index keeps them all). Retries extend the record rather than erasing
it: each attempt's history rides the receipt (`Attempts`), so the record shows the complete retry history, not just
the last attempt.

---

## 8. Safety Guarantees

### 8.1 DAG by Construction — *landed*

A unit must exist in the plan before its promise can be referenced, so cycles cannot be expressed; guarded-edge
validation additionally rejects malformed decision trees and ordering-edge cycles at both seal and load
([2.3](2.3-orchestration-primitives.md)). Passing a non-promise fails at plan time; a promise consumer dispatches
only after its producer; every dependent has a literal pointer to the producer it depends on. When a true circular
data dependency exists, the solution is an intermediary that decouples the write from the read.

### 8.2 Version Conflict Detection at Plan Time — *vision*

The SemVer solver (§5.3) fails the plan before any dispatch — no machine is left half-deployed by a version
conflict discovered at runtime.

### 8.3 Recorded Retries — *landed*

Retries are policy-driven ([2.6](2.6-execution-policies.md): tri-state, structural saga boundaries, full jitter)
and **recorded**: per-attempt history rides the dispatch's receipt, so the trace shows every attempt with its
failure context.

---

## 9. Failure and Recovery

### 9.1 Partial Results — the Gather Postures *(the local machinery is landed; the coordinator behaviors are vision)*

- **Strict consensus** (database cluster): without N/N successes the parent cannot resolve; the coordinator may
  decommission the successes to prevent split-brain.
- **Elastic / degraded** (web tier): 95 of 100 succeed — the run lands `completed × degraded`
  ([2.2](2.2-phase-execution.md)): successes kept, failures recorded, drift detection later reconciles the missing.
- **Heuristic substitution** (spot instances): a zone-capacity failure triggers re-discovery and deployment
  elsewhere — re-planning, in the landed vocabulary.

### 9.2 Hard vs. Soft Dependencies

A hard dependency parks or aborts the downstream branch on upstream failure. A soft dependency proceeds with a
cached value or default and verifies with degraded expectations — the error-action + `flow.Degraded` opt-in is the
landed form of this choice ([2.2](2.2-phase-execution.md)).

### 9.3 The Stale Fact Problem

When an external intermediary (e.g., a key vault) carries a dependency value, out-of-band changes create a blind
spot: the record still shows health from the last execution. **Fix:** verification must verify the *linkage* — not
just "is my service running?" but "do I still have access to the resource the upstream provided?" — so a re-verify
walk catches drift even without a new deployment.

---

## 10. Lifecycle Operations

- **Deploy** — forward: build, persist the plan, execute, record. *(landed: the writ deploy family, steps 47–49)*
- **Status / reconcile** — the re-verify walk over the records: `writ status` classifies fresh / stale / modified
  from the recorded content identity. *(landed locally)*
- **Upgrade** — constrained re-deploy with drift attribution (source-changed vs. target-modified) from the
  as-deployed record. *(landed locally; the §5 solver remains vision)*
- **Decommission** — reverse traversal: children before parents; release functional dependencies. *(landed
  locally)*

---

## 11. Open Questions

1. **Graph granularity in the global model.** Is each machine one node in the global graph, or does the global
   graph see per-machine structure?
2. **Global model storage.** Single document, per-machine records with a global index (the run index generalized),
   or a graph database?
3. **Reconciliation trigger.** Periodic, event-driven, or on-demand?
4. **Partial gather remediation.** Can the coordinator auto-generate a remediation subgraph for the missing 2 of
   10, or is gather strictly wait-and-report?
5. **Dynamic re-planning.** Can the coordinator re-run the original plan logic after partial failure, or does
   re-planning require a human?

---

## 12. Implementation Status

| Component | Status | Location |
|---|---|---|
| Sealed execution graph (units, bindings, edges) | Implemented | `pkg/op/` |
| Saga execution (receipts, recovery tree, run-state machine) | Implemented | `pkg/op/` |
| Binding model (immediate, promise, variable + projection) | Implemented | `pkg/op/` |
| Orchestration primitives (Subgraph, Choose, Gather, WaitUntil) | Implemented | `pkg/op/provider/flow/` |
| Providers (18 in the catalog) | Implemented | `pkg/op/provider/` ([3.5](3.5-provider-catalog.md)) |
| Plan bindings (the Starlark planning API) | Implemented | `pkg/op/provider/plan/`, `pkg/op/starlarkbridge/` |
| Record integrity (checksum, ssh-ed25519 signing, `writ verify`) | Implemented | `pkg/signing/` (step 46) |
| Trace store + run index + drift attribution | Implemented | `internal/cli/` (steps 47–48) |
| Package planner (ref counting, SemVer intersection) | Not started | — (§5 vision) |
| Distributed coordinator / interface nodes / cross-node promises | Not started | — (§6 vision) |
| Global record graph | Not started | — (§7 vision) |
