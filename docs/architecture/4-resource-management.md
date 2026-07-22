# Resource Management: URI-Based Resource Tracking

> **Status:** rewritten 2026-07-22 (phase-8 step 51, slice 3) onto the landed `pkg/op` model. The migration-era
> bookkeeping (per-method signature-migration tables, the string-parameter "today" snapshots, `Tombstone` recovery,
> the constructor-registry coercion chain) is replaced; the landed catalog model — states, the behavior matrix
> (ruled 2026-07-14), shadowing, observations — is the body; the **declared-output-spec proposal is preserved as an
> explicitly unimplemented appendix** (verified 2026-07-22: no `OutputSpec`, no `*Planned` companions in the tree).
> Companion: [`4-resource-management.status.md`](4-resource-management.status.md).

This document describes resource management in `pkg/op`: how providers track external state through typed resource
handles, how the catalog resolves URI-based identity across the execution graph, and how recovery unifies under
receipts and the recovery site.

See also:

- [Resource Management Plan](../plans/resource-management.md) — the historical implementation plan
- [4.1-resource-identity.md](4.1-resource-identity.md) — URI schemes and the addressing contract
- [4.4-root-path-triad.md](4.4-root-path-triad.md) — `fsroot.Root`, `Path`, and `op.RecoverySite`
- [2.2-phase-execution.md](2.2-phase-execution.md) — receipts and compensation

## 1. The Lineage Problem

Two nodes in an execution graph can target the same filesystem path with no dependency edge between them. The graph
cannot detect this when paths are opaque strings — the system treats `"/etc/foo"` as a value, not an identity.

```python
plan.file.write_text(destination_path="/etc/foo", content="v2", chmod=0o644)
result = plan.file.read_text(resource="/etc/foo")   # must read v2, not the original
```

Without identity, the write and the read are unordered — a silent race decided by scheduling. The fix is to track
**identity**: typed resources with URIs, a catalog that maps each URI to its current version, and **shadowing** that
turns same-URI production into implicit dependency edges (or plan-time conflicts).

## 2. Architectural Summary

The architecture separates **intent** (planning) from **reality** (execution). A graph is planned once and can be
executed on many machines:

- **Plan time** — pure, no I/O: values convert to typed resources ([2.1](2.1-typed-slots.md)'s conversion cascade),
  URIs are claimed in the catalog, shadowing records who produces what.
- **Execution time** — the executor's pre-flight resolve pass verifies existence against the target and the catalog
  applies state transitions; dispatch results transition claims to reality.

**`Resource`** (`pkg/op/resource.go`) — an interface sealed by an unexported method; only types embedding
`ResourceBase` implement it. The base carries identity (`uri`, catalog `id`, `producerID` — empty for discovered,
pre-existing things) and the catalog-owned `ResourceState`. Provider resource types embed the base and add domain
fields ([3.5.x](3.5-provider-catalog.md) per provider; [4.1](4.1-resource-identity.md) for scheme and addressing).

**`ResourceCatalog`** (`pkg/op/resource_catalog.go`) — one per graph: the append-only ledger plus the URI→id
namespace. Its surface (tree-verified 2026-07-22):

```
Discover(uri, factory)             ← observation: read-or-introduce, no production claim
GetOrCreate(producerID, uri, factory) ← production: claim the URI for a producing unit
Resolve(r) → (canonical, id)       ← return the canonical entry for a caller-built resource
Shadow(r, producerID) → (id, err)  ← new version at an occupied URI; namespace repointed
Current(uri) → id                  ← the namespace's current version
Lookup(id) / Len / Link            ← ledger access; Link interns an entry
State(id) / MarkGone(r) / VerifyExistence(r) ← the state machine (below)
Clone()                            ← the per-run copy Run clones onto the environment
Snapshot() / ContentResources()    ← the trace's ledger snapshot; content transport (step 25)
```

**Catalog ownership transfers at assembly**: the planning catalog is captured by `AssembleDefinition` and sealed
into the graph; `Run` clones it onto the per-run environment ([2](2-execution-graph.md),
[3.5.3](3.5.3-plan-provider.md)).

## 3. Resource States and the Behavior Matrix

Three states (`pkg/op/resource_state.go` — `ResourceState`, renamed from `op.State` by phase-8 step 41):

- **Pending** — the URI is claimed but not yet observed or produced. Plan-time entries are born here.
- **Active** — observed-as-existing (discovery) or freshly created (production). Metadata is trustworthy.
- **Gone** — reactive: an existence check failed. Terminal for the entry; reviving a URI appends a fresh entry.

**The catalog owns transitions.** The existence verdict is the resource's `Exists()` predicate; the catalog applies
the transition through `VerifyExistence`, driven from the **executor's pre-flight resolve pass** over the per-run
clone — never from plan-time `DiscoverResource`, which only introduces `Pending` entries. Rollout is staged
(phase-8 step 22): `file` is implemented and tested; other resource types' `Resolve`/`Exists` remain unimplemented
stubs and stay `Pending` until their per-type step.

```
        ┌────────────┐
        │  Pending   │  ◀── initial state on insert
        └────────────┘
          │        │
 Exists() │        │ Exists() false
          ▼        ▼
   ┌──────────┐ ┌────────┐
   │  Active  │ │  Gone  │ ◀── terminal; revival = a fresh shadowing entry
   └──────────┘ └────────┘
          │        ▲
          └────────┘  (a later failed check on an Active entry → Gone)
```

#### Catalog behavior matrix (ruled 2026-07-14; step 22)

| Op | Cache state | `Exists()` | Content-addressable | Location-based |
|---|---|---|---|---|
| `DiscoverResource` | miss | success | append `Pending` → `Active`; no `producerID` | same |
| `DiscoverResource` | miss | failure | append `Pending` → `Gone`; return error | same |
| `DiscoverResource` | hit, `Pending` | success | in-place `Pending` → `Active`; discard input | same |
| `DiscoverResource` | hit, `Pending` | failure | in-place `Pending` → `Gone`; return error | same |
| `DiscoverResource` | hit, `Active` | (not called) | return existing | same |
| `DiscoverResource` | hit, `Gone` | (not called) | return error | same |
| `DiscoverResource` | (any) | (any) | **never shadows** | **never shadows** |
| `NewResource` | miss | (not called) | append `Pending` → `Active`; stamp `producerID` | same |
| `NewResource` | hit, `Pending`/`Active` | (not called) | return existing (singleton) | **shadow**: new entry, stamp `producerID`; old entry stays as history |
| `NewResource` | hit, `Gone` | (not called) | **shadow** (revives the URI) | same |

**Why the asymmetry:** a content-addressable URI encodes its identity, so re-producing it is provably the same
resource (the CAS types: mem, function, json, yaml — [4.2](4.2-mem-resource.md),
[3.5.5](3.5.5-json-provider.md)/[3.5.6](3.5.6-yaml-provider.md), [3.5.14](3.5.14-function-provider.md)). A
location-based URI does not encode content, so a new production at the same URI is genuinely a new version
downstream consumers must see. `Gone` is recorded rather than suppressed because "we expected this to exist and it
doesn't" is input to compensation and reconciliation.

**Observations are not catalog members (ruled 2026-07-14).** An observation is a point-in-time **metadata
snapshot** — a fact *about* a thing, not a thing whose existence is in question. The membership test: the catalog is
the identity ledger of things that can be asked "do you exist?"; an observation cannot meaningfully answer (its own
existence is trivially true), so it is not a `Resource` and never enters the catalog. It rides the **execution
record** instead: an observe action's observation is that node's result, carried on its receipt and serialized in
the trace; resume re-observes rather than reconstructs. Identity comes from the observed resource by back-link
(`op.ObservationBase`); an observation mints no URI and hashes no content.

## 4. Shadowing — Implicit Edges and Plan-Time Conflicts

When planning produces a resource at a URI, the catalog records the claim; when a later unit references the same
URI, `Resolve` returns the canonical (possibly shadowed) entry and the producer link makes the ordering explicit:

```
Step 1: plan.file.write_text(destination_path="/etc/foo", ...)
  └─ the write's target resource is cataloged with producerID = the write node
Step 2: plan.file.read_text(resource="/etc/foo")
  └─ the string converts to a typed resource; Resolve returns the cataloged entry;
     the read depends on the write — order guaranteed, no explicit promise needed
```

Two units *producing* the same URI is a **plan conflict** surfaced at claim time (this is also the cross-kind rule:
the same URI claimed as two different kinds errors at the earliest claim — [3.5.4](3.5.4-file-provider.md)). For
gather, uniqueness of items is the plan author's contract: same-resource production across iterations is a plan
conflict, same-path modification is a race — both by design; fix the plan
([2.3](2.3-orchestration-primitives.md)).

## 5. Recovery — Receipts and the Recovery Site

The pre-`op` `Tombstone` family is gone (phase-8 steps 40/42): the undo record is the **receipt**
([2.2](2.2-phase-execution.md)), and file mutation flows through the unified receipt with `MutationKind`,
boundary, and the recovery-digest tamper check ([3.5.4](3.5.4-file-provider.md)).

`op.RecoverySite` ([5.3](5.3-recovery-site.md), [4.4](4.4-root-path-triad.md)) is the shared archival service —
`ArchiveFile` (zero-copy rename into `.devlore/recovery/` under the root authority, same partition guaranteed),
`ArchiveData` / `ArchiveStream` (byte serialization), `RestoreFile` / `RestoreData`. A receipt's `TransactionID` is
the recovery key. Providers archive before destructive mutation; compensation restores — and the recorded
`recoveryDigest` detects tampering of the recovery store between forward action and compensation.

## 6. The Platform Provider — Data, Not Actions

The platform provider (`pkg/op/provider/platform/`) is the Starlark surface for the runtime environment's
`op.Platform` — the serializable capability + Composite package-manager router of
[3.4](3.4-platform-package-managers.md). Access is `both`: immediate reads answer from the local machine; planned
reads resolve at execution time against the target's platform — the mechanism by which one graph targets many
machines. The graph carries only serialized target identity; managers re-attach at run time, never serialize.

## 7. Provider Lifecycle

Every provider embeds `op.ProviderBase` and is constructed against the `*op.RuntimeEnvironment` by the constructor
its announcement registers (`op.AnnounceProvider(type, role, constructor, metadata)` — see
[3-operation-namespaces.md](3-operation-namespaces.md) and [2.1](2.1-typed-slots.md) §Action dispatch). A provider
follows the lifetime of its runtime environment — per run in a graph, per session in a script. Environment access
is uniform: `p.RuntimeEnvironment()` for the root, catalog, platform, recovery site, narrator.

## 8. Resolved Decisions

1. **Sealed interface** — `Resource` sealed via the unexported base accessor; catalog stores interface values.
2. **URI canonicalization** — applied at construction, pure.
3. **Immediate mode** — immediate receivers pass raw values; cataloged resources are a planning concern.
4. **Catalog scope** — per graph; cloned per run; ownership transfers at assembly.
5. **Recovery ownership** — receipts name the undo; providers own the mechanism; the recovery site is shared.
6. **Coercion vs. resolution** — plan time converts and claims (pure); execution time verifies and transitions
   (the pre-flight pass) — required because a graph is planned once and executed on many machines.
7. **Observations** — results, never catalog members (§3).

## 9. Open Questions

1. **Remote execution transport** — the graph is portable, and `op.Platform` carries target identity, but the
   pre-flight pass needs a filesystem abstraction for remote targets ([1-system-model.md](1-system-model.md) §6.2
   vision).
2. **Per-type existence rollout** — the staged step-22 rollout: which resource type's `Resolve`/`Exists` lands
   next.

---

## Appendix A — Proposed: Declared Output Specs *(design only — not implemented)*

> **Verified 2026-07-22:** no `OutputSpec` type, no `KnownAtExecution` sentinel, and no `*Planned` companion
> methods exist in the tree. The landed alternative is the planner + conversion path: plan-time target resources
> are built by the conversion cascade (`TargetConverter.CanConvertFrom` — [2.1](2.1-typed-slots.md)) and cataloged
> at claim time; monadic outputs (e.g. `pkg.install`'s manager-dependent purl) shadow post-dispatch. This appendix
> preserves the proposal and its prior-art grounding for a future charter.

**The proposal.** Every resource-producing method declares a pure sibling — `X` / `XPlanned` — computing the output
resource's identity from the input slot values; the framework calls it at plan time, and the forward method calls
the same function at execution time (one source of identity truth). Outputs whose identity depends on runtime
values return a `KnownAtExecution` sentinel (Terraform's "known after apply", temporally framed): the planner skips
plan-time shadowing and the executor shadows the real result post-dispatch. A companion triplet — forward, planned,
compensate — placed adjacently in source, wired by codegen from the naming convention with six static checks (no
annotations, no struct tags).

**Claimed benefits:** one identity-construction function shared by planner and method; trivially testable pure
specs; dry-run by calling the spec instead of the method; rehydration of pending entries from recorded slots;
plan-time conflict detection for every applicative output; implicit edges via URI matching; speculative
skip-if-unchanged.

**Prior art:** Bazel's analysis-phase `ctx.actions.declare_file` (declared outputs; hard phase separation; explicit
input/output lists rather than URI-matched implicit edges); Terraform's `PlanResourceChange` and the
`(known after apply)` marker; Nix's deterministic output paths; Mokhov, Mitchell, Peyton Jones, *Build Systems à la
Carte* (ICFP 2018) — the applicative-vs-monadic task split (our default applicative; the sentinel is the monadic
exit). Where the proposal deliberately diverges from Bazel: the spec is reusable by the forward method (Bazel's
`declare_file` is analysis-only); edges are discovered from shared URIs rather than declared; purity is by
convention (Go) rather than interpreter-enforced (Starlark).
