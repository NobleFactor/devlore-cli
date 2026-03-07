# The Emergent System Model

This document formalizes the architecture of devlore's execution model as it
scales from local workstation management to distributed system orchestration.
The core thesis: **the execution graph is the system model.** There is no
separate state database, inventory file, or desired-state declaration. The
model emerges from verified execution receipts.

See also:

- [Execution Graph](2-execution-graph.md) — Core graph architecture
- [Phase Execution](2.2-phase-execution.md) — Saga pattern, compensation
- [Typed Slots](2.1-typed-slots.md) — Slot model and resolution chain
- [Orchestration Primitives](2.3-orchestration-primitives.md) — Gather,
  Choose, WaitUntil, Sidecar
- [Receipt Integrity](5-receipt-integrity.md) — Checksum and signature

---

## 1. Design Thesis

Traditional configuration management tools maintain a **state file** that
represents what the system should look like. The state file is an indirect
claim about reality — it says "Terraform applied these resources" or "Ansible
ran these tasks." If reality diverges from the state file (manual changes,
partial failures, clock skew), the file becomes a liability.

This architecture inverts the model. Instead of "desired state → apply →
hope," it implements "execute logic → capture receipt → verify truth."

| Aspect | State-File Model (Terraform, Ansible) | Emergent Model (devlore) |
|---|---|---|
| Source of truth | Static file, can drift | Verified execution receipt |
| Logic | DSL/YAML (HCL, Jinja) | Code (Go providers, Starlark scripts) |
| Inventory | Pre-defined, static | Emerges at execution time (Proxies) |
| Drift detection | External tool, expensive refresh | Re-run Verify phase on receipt |
| Concurrency | Limited, side-effect prone | Safe — DAG by construction |
| Provenance | Scattered (git logs, CI, SSH) | Embedded in graph (receipts) |
| Recovery | Full wipe-and-reinstall | Phase-aware surgical remediation |

The receipt is not a log. It is a structured, checksummed, optionally signed
record of the exact code path, dependency resolution, and phase outcomes that
produced the current state of a machine. The collection of receipts across
all machines in a fleet IS the system model.

---

## 2. Dependency Taxonomy

The system's intelligence is rooted in three distinct dependency types. Each
type creates a different kind of edge in the execution graph, carries different
lifecycle semantics, and requires different failure handling.

### 2.1 Structural Dependencies — Composition

**Definition.** A relationship where one subsystem contains or manages another
subsystem. Established by nesting `plan.deploy()` calls.

**Graph semantics.** Creates a compositional edge. The child's receipt is
embedded within the parent's receipt, forming a recursive structure.

**Lifecycle impact.** If a child fails, the parent is structurally incomplete.
Decommissioning a parent decommissions its children first (depth-first
traversal). Reconciliation propagates downward — re-verifying a parent
triggers re-verification of all structural children.

**Example.** A "Production Application" subsystem contains a "Database
Subsystem" and a "Cache Subsystem." The database receipt is nested within the
application receipt.

### 2.2 Functional Dependencies — Environment

**Definition.** A prerequisite that must exist in the host environment for a
subsystem to function. Established by `plan.package.install()` and related
package operations.

**Graph semantics.** Creates a prerequisite edge. Multiple subsystems may
share a functional dependency. The dependency is only safe to remove when the
last structural dependent is decommissioned.

**Lifecycle impact.** Subject to version intersection (Section 5), reference
counting, and batch optimization. A security vulnerability in a functional
dependency can be traced to every subsystem that depends on it, regardless
of nesting depth.

**Example.** Subsystem S declares `plan.package.install("openssl")`. This
records that S requires OpenSSL. The receipt captures the installed version,
the constraint that was satisfied, and the reference count.

### 2.3 Procedural Dependencies — Temporal Ordering

**Definition.** A temporal requirement where one action cannot begin until
another has produced a specific result. Created by the Slot → Promise → Proxy
chain.

**Graph semantics.** Creates a blocking edge. The downstream node's execution
is physically prevented until the upstream node's Verify phase succeeds and
the Promise resolves.

**Lifecycle impact.** Controls deployment ordering. If a procedural
dependency fails, the downstream branch is parked or aborted. The failure
metadata is preserved as a Tombstone for diagnostics.

**Example.** `promise_db = plan.deploy(database, server_a)` followed by
`plan.deploy(web_app, server_b, promise_db)`. The web app's Prepare phase
cannot begin until the database's Verify phase succeeds.

### Summary

| Type | Focus | Mechanism | Failure Effect |
|---|---|---|---|
| Structural | Hierarchy | Nested `plan.deploy()` | Parent incomplete |
| Functional | Environment | `plan.package.install()` | Subsystem cannot run |
| Procedural | Sequence | Promises & Slots | Branch blocked |

---

## 3. Node Lifecycle — The Four Phases

Every deployment follows a four-phase progression. A node is considered
fulfilled in the graph only when all phases succeed.

### 3.1 Prepare

Resolve proxies, fetch artifacts, validate the host environment.

The Prepare phase records the initial state and the specific versions of
inputs consumed. If a Slot holds a Proxy, this phase triggers the discovery
subroutine (e.g., a cloud API call to enumerate servers). Proxies return
Promises that are resolved before execution continues.

### 3.2 Install

Place binaries, install native packages, set up the filesystem.

The Install phase triggers the Package Planner (Section 5) for batched
package operations. It records that the required artifacts are physically
present on the host. This is where functional dependencies are satisfied.

### 3.3 Provision

Configure installed components, start services, apply runtime state.

The Provision phase runs the logic that transforms installed packages into
running services. It records configuration parameters, generated identifiers
(PIDs, UUIDs), and runtime state.

### 3.4 Verify

Execute health checks. This is the **truth gate.**

The Verify phase is what distinguishes this model from traditional
configuration management. Success here resolves the Promise back to the
coordinator. The receipt records the verified fact (e.g., "port 80
responsive," "process running with PID 4501").

If Verify fails, the system has two options depending on configuration:

1. **Tombstone.** The state is preserved as-is for diagnostic permanence.
   The Provision phase's side effects are not rolled back — the evidence of
   failure is retained in the emergent model.
2. **Compensate.** The saga coordinator unwinds completed phases in LIFO
   order using the recovery stack (see Phase Execution architecture doc).

### Phase as Barrier

Because the system uses synchronous dependency contracts, a downstream node
waiting on a promise is waiting for the upstream node to complete its Verify
phase — not just for it to start. The phase sequence is a barrier:

```
          Prepare ──→ Install ──→ Provision ──→ Verify
             │            │            │           │
          cleanup     uninstall   unprovision    (truth)
        (compensate) (compensate)  (compensate)
```

Each phase is a scoped transaction defined by the tuple **(A, C, S)**:

| Component | Role |
|---|---|
| **A** (Action) | Forward operation |
| **C** (Compensate) | Reverse operation |
| **S** (State) | Metadata captured during A that C needs to undo |

A is obligated to populate S during forward execution. S is the receipt of A
and the input to C. The recovery stack stores these tuples in LIFO order.
On failure, the saga coordinator walks the stack in reverse, invoking each
C with its corresponding S.

---

## 4. The Slot Model — Data Flow Primitives

The slot model provides the data flow mechanism that connects nodes in the
execution graph. Every node input is a named Slot that holds one of three
value types.

### 4.1 Immediate Values

Known at plan time. Filled by the Starlark script or graph builder.

```
node.SetSlotImmediate("packages", "nginx,curl")
```

### 4.2 Promises

References to upstream node results. Resolved at execution time when the
upstream node completes its Verify phase. Creates an edge in the graph.

```python
# Starlark
db = plan.deploy(database, server_a)
plan.deploy(web_app, server_b, db_endpoint=db.endpoint)
```

The executor resolves the promise by reading the upstream node's result map
and injecting the value into the downstream slot before calling Do().

### 4.3 Proxies

A specialization of promises for subroutine behavior. Used by Gather
operations to bind iteration variables. The Proxy doesn't reference a
specific upstream node — it references a field within the current iteration
of a parallel comprehension.

### Resolution Chain

When a slot is resolved at execution time:

1. Check for caller-provided value (Starlark script or graph builder)
2. Fall back to Context.Data (engine-injected values like SOPS config)
3. If neither exists and the slot is required, execution fails

This chain is codified in the generated Action wrappers. Each action reads
its slots without type-switching — the resolution is self-contained.

---

## 5. Package Planning — The Functional Dependency Solver

On flat host environments where containers or VMs are impractical, the system
employs a reference-counting package planner that operates during the Plan
phase, before any subgraph is dispatched for execution.

### 5.1 Reference Counting

Multiple subsystems may require the same package. The planner tracks active
references to prevent premature uninstalls.

**Acquire:** During the Prepare phase, the coordinator invokes `Acquire` for
each required package. If two subsystems request the same package, the
reference count increments but only one INSTALL action is generated.

**Release:** When a subsystem completes its use of a package, it calls
`Release`. The package is uninstalled only when the reference count reaches
zero AND the dependency type is ephemeral.

### 5.2 Ephemeral vs. Persistent Dependencies

The planner distinguishes between two dependency lifetimes:

| Type | Meaning | Uninstall policy |
|---|---|---|
| **Persistent** | Required for the subsystem's lifetime | Never auto-uninstalled |
| **Ephemeral** | Required only during a phase (e.g., curl for download) | Uninstalled when ref count = 0 |

**Promotion rule:** If any subsystem requires a package to be persistent, the
planner promotes it. Persistent wins over ephemeral.

### 5.3 SemVer Intersection

When multiple subsystems declare version constraints on the same package, the
planner computes the overlapping range:

1. **Gather requirements.** As `plan.deploy()` is called recursively, the
   planner collects all functional dependencies with their version constraints.
2. **Intersect ranges.** For each package, compute the overlap of all
   declared ranges. The agreed range is `[max(all minimums), min(all maximums)]`.
3. **Detect conflicts.** If the intersection is empty, the plan fails with a
   conflict error identifying exactly which subsystems are incompatible.
4. **Select version.** Pick the highest version within the agreed range.

This happens during graph construction (dry-run). No subgraph is dispatched
to a node until all version constraints are resolved.

### 5.4 Upgrade Safety

When a new subsystem requires a higher version than what is currently deployed
on a running host, the planner evaluates risk against the emergent model's
existing receipts:

| Scenario | Action |
|---|---|
| New minimum is within old maximum | Upgrade, then re-verify all dependents |
| New minimum exceeds old maximum | Reject — isolation boundary required |
| Ephemeral promoted to persistent | Lock — package marked non-removable |

An upgrade triggers a **recursive re-verify**: the coordinator identifies
every node with a functional dependency on the upgraded package and re-runs
their Verify phases. If any re-verify fails, the system triggers the retry
or rollback policy.

### 5.5 Batch Optimization

The planner optimizes the install-use-uninstall pattern common in build
workflows (e.g., install curl, download artifacts, uninstall curl):

| Pattern | Optimization |
|---|---|
| Install → Uninstall (same batch) | Keep if another subsystem needs it; batch uninstall at end |
| Install → Upgrade (same package) | Skip initial install, install target version directly |
| Install → Uninstall → Install | Maintain — do not uninstall if re-required within same batch window |

**Receipt integrity rule:** Even if an install is optimized away (because the
package already exists), the receipt records that the requirement was
satisfied. Drift detection must not be confused by "missing" actions that
were optimized away.

---

## 6. Orchestration Flow

### 6.1 Local Execution (Single Machine)

The current execution model operates on a single machine:

1. **Graph construction.** Starlark scripts call plan bindings (e.g.,
   `plan.package.install()`, `plan.file.link()`). Each call creates a Node
   with an Action from the registry, fills Slots, and returns an Output
   (promise).
2. **Dry-run validation.** The graph is serialized before execution,
   producing a plan that shows what will happen. Circular dependencies are
   structurally impossible (Section 8.1).
3. **Phase execution.** The executor walks phases in order (prepare →
   install → provision → verify). Within each phase, nodes are executed
   via topological sort with concurrency limits.
4. **Compensation.** On failure, the recovery stack unwinds completed phases
   in LIFO order.
5. **Receipt generation.** The completed graph is serialized as a receipt
   with checksum and optional signature.

### 6.2 Distributed Orchestration (Multi-Node)

The distributed model extends local execution with a coordinator-node
handshake:

1. **Graph construction.** The coordinator runs the plan logic to build the
   global execution graph. Each "node" in the global graph represents a
   machine's subgraph.
2. **Subgraph dispatch.** The coordinator sends subgraphs to target machines.
   Targets can be gathered dynamically at runtime via Proxies.
3. **Execution.** Each machine executes its local subgraph through the
   four-phase lifecycle. Failures are recorded as Tombstone nodes.
4. **Receipt collection.** The coordinator downloads receipts from all
   machines. These receipts are merged into the global emergent model.

The same primitives operate at both scales:

| Local (single machine) | Global (distributed) |
|---|---|
| Node = action | Node = machine subgraph |
| Edge = data dependency | Edge = cross-machine dependency |
| Phase = lifecycle boundary | Phase = deployment wave |
| Gather = parallel items | Gather = parallel machine provisioning |
| Choose = conditional branch | Choose = platform-specific provisioning |
| Result = action output | Result = machine receipt |
| Promise slot = upstream ref | Promise slot = cross-machine data ref |

### 6.3 Interface Nodes

To keep node-level execution clean, subgraphs use **Interface Nodes** at
their boundaries:

- **Input Node (Requirement).** Blocks execution of its children until the
  coordinator injects a value (e.g., `db_endpoint`). The coordinator resolves
  this by reading the upstream machine's receipt.
- **Output Node (Provision).** Upon successful Verify phase, exports a value
  to the coordinator (e.g., `api_key`). The coordinator propagates this value
  to downstream machines that hold a promise on it.

### 6.4 Cross-Node Data Flow

When Node A on machine 1 produces a value that Node B on machine 2 needs:

1. Machine 1 executes its subgraph. The Verify phase succeeds and produces
   a result (e.g., an API key).
2. The coordinator receives machine 1's receipt and extracts the result.
3. The coordinator injects the result into machine 2's Input Node slot.
4. Machine 2's execution unblocks and continues.

**Wait-state strategy:** Reactive (push). The coordinator maintains the
global state. When a machine's receipt arrives, the coordinator scans for
downstream machines waiting on that data and dispatches immediately.

---

## 7. The Emergent Model — The Receipt Graph

### 7.1 What the Model Is

The emergent model is the collection of all receipts. It is a directed graph
where every vertex is a receipt and edges represent the three dependency
types (structural, functional, procedural).

There is no separate database. If you want to know the state of the system,
you traverse the receipt graph.

### 7.2 Receipt Structure

A receipt is a serialized execution graph annotated with phase outcomes:

```yaml
node_id: "uuid-123"
structural_parent: "uuid-parent"
functional_deps:
  - name: openssl
    version: "3.0.2"
    type: persistent
  - name: curl
    version: "8.5.0"
    type: ephemeral
procedural_inputs:
  db_ip: "10.0.0.5"
phases:
  prepare:
    status: success
    duration: "100ms"
  install:
    status: optimized
    details: "already present"
  provision:
    status: success
    outputs:
      pid: 4501
  verify:
    status: success
    fact: "port 80 responsive"
timestamp: "2026-02-18T11:30:00Z"
checksum: "sha256:a7b9c3d4..."
```

The receipt records:

- **Phase granularity.** Where execution stopped on failure, enabling
  surgical remediation instead of full wipe-and-reinstall.
- **Dependency provenance.** Why each package version was chosen (the
  intersection of which subsystems' constraints).
- **Verified facts.** The actual health check result, not just "success."

### 7.3 Drift Detection

The Verify phase is built into the graph code. Re-running just the Verify
leaf of an existing receipt provides drift detection:

- **Verify passes:** The receipt is still accurate. The model is current.
- **Verify fails:** The system has drifted. The receipt is no longer a fact.
  This triggers the reconciliation policy.

This is a native capability, not an external agent. The same code that
deployed the system can re-verify it.

### 7.4 Provenance

Every component in the system can be traced back to:

- The specific line of Starlark code that declared the intent
- The version constraints that selected the package version
- The phase outcomes at each step of deployment
- The upstream dependencies that had to succeed first
- The timestamp and checksummed execution record

### 7.5 Tombstone Records

When a retry policy runs to completion and still fails, the graph keeps
Tombstone Nodes for the failed deployments. The graph is a complete picture
of what happened. Tombstones enable queries like: "Show me all nodes where
the Provision phase has historically failed more than 20% of the time."

Tombstones are permanent. They are not cleaned up on retry — the retry is a
new node in the graph that depends on the failure metadata of the original.

---

## 8. Safety Guarantees

### 8.1 DAG by Construction

The programming model makes circular dependencies structurally impossible:

```python
promise_1 = plan.deploy(service_a, server_1)
promise_2 = plan.deploy(service_b, server_2, promise_1)
```

A node must exist in the plan before its promise can be passed as an
argument. You cannot reference a future node that hasn't been instantiated.
This builds a DAG by construction — you cannot express a cycle in the
Starlark API.

**Safety guarantees:**

1. **Immediate validation.** Passing a variable that isn't a registered
   promise fails at plan time.
2. **Implicit barriers.** `promise_2` waits for the Verify phase of
   `promise_1` to succeed. Built-in stop-on-error.
3. **Traceable provenance.** Every dependent instance has a literal pointer
   in the graph to the specific upstream instance it depends on.

When a true circular data dependency exists (A needs B's IP, B needs A's
security group), the solution is an intermediary — a shared data store or
key vault that decouples the write from the read.

### 8.2 Version Conflict Detection at Plan Time

The SemVer intersection solver (Section 5.3) runs during graph construction.
If two subsystems require incompatible versions of the same package, the plan
fails before any subgraph is dispatched. No machine is left in a partially
deployed state due to a version conflict discovered at runtime.

### 8.3 Deterministic Retries

Retries are not loops. They are graph extensions. If a node fails, the retry
is a new node in the graph that depends on the failure metadata of the
original. This means:

- Every retry attempt is recorded in the receipt
- The retry has full access to the original's failure context
- The receipt graph shows the complete retry history, not just the last attempt

---

## 9. Failure and Recovery

### 9.1 Retry as Graph Mutation

When a Gather operation produces partial results, the coordinator treats the
failure as a graph mutation opportunity, not a simple retry loop. The retry
policy determines the strategy:

#### Strict Consensus (e.g., Database Cluster)

Deploying a 3-node cluster. One node fails. Without N/N successes, the
parent Slot cannot resolve. The coordinator may trigger a Decommission
subgraph on the successful nodes to prevent split-brain.

#### Elastic/Degraded (e.g., Web Tier)

Deploying 100 servers. 5 fail due to transient issues. The coordinator marks
the Slot as "Resolved (Degraded)." The emergent model shows 95 active
edges — the system model reflects reality at 95% capacity. Drift detection
will eventually identify the missing 5 and attempt reconciliation.

#### Heuristic Substitution (e.g., Spot Instances)

A subgraph fails because a zone is out of capacity. The retry policy triggers
a Proxy re-evaluation — the discovery subroutine finds alternative targets,
generates new subgraphs, and attempts deployment elsewhere.

### 9.2 Hard vs. Soft Dependencies

The dependency type in a Slot determines failure behavior:

- **Hard dependency.** The downstream node cannot function without the
  upstream value. If the upstream fails, the downstream branch must Park
  (suspend) or Abort.
- **Soft dependency.** The downstream node can use a cached value or a
  default. It can proceed with a warning and Verify with degraded
  expectations.

### 9.3 The Stale Fact Problem

When an external intermediary (e.g., Azure Key Vault) provides a dependency
value, out-of-band changes create a blind spot:

- Node B pushes an API key to the vault during Provision.
- Node A pulls the key during Prepare. The receipt records the linkage.
- A human manually rotates the key. The emergent model still shows health
  based on the last execution.

**Fix:** The Verify phase must verify the linkage — not just "is my service
running?" but "do I still have access to the resource provided by the
upstream Slot?" If Verify includes a check against the intermediary, a
re-verify walk catches drift even without a new deployment.

---

## 10. Lifecycle Operations

The four-phase model applies across all lifecycle operations, each with
different traversal semantics:

### Deploy

Forward traversal. Build graph, execute phases, collect receipts.

### Reconcile

Re-verify walk. Traverse the receipt graph and re-run Verify phases.
Failures trigger re-provisioning of the affected subtree. Because structural
dependencies nest, reconciling a top-level subsystem recursively reconciles
all children.

### Upgrade

Constrained re-deploy. The Package Planner (Section 5) computes the new
version intersection, checks upgrade safety, and generates an upgrade
subgraph. After the upgrade, all dependents are re-verified.

### Decommission

Reverse traversal. Walk the structural dependency tree depth-first.
Decommission children before parents. Release functional dependencies.
Decrement reference counts. Uninstall packages whose reference count
reaches zero (if ephemeral).

---

## 11. Open Questions

These questions represent design decisions that will be resolved through
implementation and scenario testing.

1. **Graph granularity in the global model.** Is each machine a single node
   in the global graph, or does the global graph have visibility into
   per-machine phases?

2. **Global graph serialization.** The global graph plus all machine receipts
   equals the system model. Storage format: single document, directory of
   per-machine receipts with a global index, or a graph database?

3. **Reconciliation trigger.** Is drift detection periodic (cron),
   event-driven (webhook), or on-demand?

4. **Partial gather resolution.** When a gather returns 8/10 successes, can
   the coordinator automatically generate a remediation subgraph to find 2
   replacements before moving to the next synchronous step? Or is gather
   strictly "wait and report"?

5. **Dynamic re-planning.** Can the coordinator use the original Starlark
   logic to re-plan after a partial failure, or does re-planning require
   human intervention?

---

## 12. Implementation Status

| Component | Status | Location |
|---|---|---|
| Execution graph (nodes, edges, slots) | Implemented | `internal/execution/` |
| Phase execution (saga, recovery stack) | Implemented | `internal/execution/` |
| Typed slots (immediate, promise, proxy) | Implemented | `internal/execution/` |
| Orchestration primitives (Gather, Choose) | Designed | `docs/architecture/` |
| Providers (10 resource domains) | Implemented | `internal/execution/provider/` |
| Actions (Do/Undo wrappers) | Implemented | `**/actions_gen.go` |
| Plan bindings (Starlark graph API) | Implemented | `internal/starlark/plan_*.go` |
| Receipt integrity (checksum, signature) | Implemented | `internal/signing/` |
| Package planner (ref counting, SemVer) | Not started | — |
| Distributed coordinator | Not started | — |
| Interface nodes (Input/Output) | Not started | — |
| Tombstone records | Not started | — |
| Cross-node promise resolution | Not started | — |
| Global receipt graph | Not started | — |
