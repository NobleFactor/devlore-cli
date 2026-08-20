# Resource Management: URI-Based Resource Tracking

> **Status:** rewritten 2026-07-22 (phase-8 step 51, slice 3) onto the landed `pkg/op` model; **revised
> 2026-08-20 onto the resource-construction rulings** ([plan](../plans/resource-construction.md), feature
> [#581](https://github.com/NobleFactor/devlore-cli/issues/581)): the catalog is input intent and travels with
> the graph (§5), plan-space paths follow the git model, identity is root-relative, and **the
> declared-output-spec proposal is rejected** — products are runtime facts, so the former Appendix A is
> removed rather than preserved (§9 item 8). Implementation is staged in
> [#582–#587](https://github.com/NobleFactor/devlore-cli/issues/581); until those land, the tree carries the
> pre-ruling behavior. Companion: [`4-resource-management.status.md`](4-resource-management.status.md).

This document describes resource management in `pkg/op`: how providers track external state through typed resource
handles, how the catalog resolves URI-based identity across the execution graph, and how recovery unifies under
receipts and the recovery site.

See also:

- [Resource Management Plan](../plans/resource-management.md) — the historical implementation plan
- [4.1-resource-identity.md](4.1-resource-identity.md) — URI schemes and the addressing contract
- [4.4-root-path-triad.md](4.4-root-path-triad.md) — `fsroot.Dir`, `Path`, and `op.RecoverySite`
- [2.2-phase-execution.md](2.2-phase-execution.md) — receipts and compensation

## 1. The Lineage Problem

Two nodes in an execution graph can target the same filesystem path with no dependency edge between them. The graph
cannot detect this when paths are opaque strings — the system treats `"/etc/foo"` as a value, not an identity.

```python
plan.file.write_text(destination_path="/etc/foo", content="v2", chmod=0o644)
result = plan.file.read_text(resource="/etc/foo")   # must read v2, not the original
```

Without identity, the write and the read are unordered — a silent race decided by scheduling. Identity is
tracked with typed resources and a catalog that maps each URI to its current version — but **ordering comes
from promises, not from URI matching** (ruled 2026-08-20): the read that must follow the write consumes the
write's invocation output, which is the edge. Two operations that merely spell the same string share one
catalog identity and gain no edge from it; the plan's judgment scenario 1 (delete-then-copy,
[resource-construction.md](../plans/resource-construction.md)) is the deliberate counterexample, where the
ordering-dependent contradiction is exactly why plan time cannot adjudicate it. What identity buys is intent
(§5), deduplication, observation (the trace), and runtime versioning (§4) — not edge inference.

## 2. Architectural Summary

The architecture separates **intent** (planning) from **reality** (execution). A graph is planned once and can be
executed on many machines:

- **Plan time** — pure, no I/O: a resource-typed parameter's string value mints a **pending** entry (no
  existence check); a promise value records the promise and nothing else; a **product is a runtime fact**
  with no plan-time presence (§5). String-typed parameters stay plain values.
- **Execution time** — the executor's pre-flight resolve pass verifies every pending rel under the run's
  root and applies state transitions; dispatch results — products included — become catalog facts on the
  per-run clone, recorded by the trace.

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

## 4. Shadowing — Runtime Versioning

**Revised 2026-08-20.** Shadowing is a **runtime** mechanism: when execution produces a resource at an
occupied URI, the run-clone's ledger appends a new generation and repoints the namespace — the prior
generation survives as history. It is how the trace records "this file was version N, and the run made it
version N+1."

What shadowing is **not**, under the intent ruling: a plan-time mechanism. Products have no plan-time
presence (§5), so planning never records who produces what, never claims an output URI, and never derives an
ordering edge from URI coincidence — ordering is the promise's job (§1). The former model — output claims at
plan time with implicit same-URI edges and plan-time producer conflicts — is superseded; its residue is
exactly what judgment scenario 1 pins as a runtime story, not a plan error.

Two units producing the same URI therefore surface at **run time**, as generations in the ledger — legal
versioning when the plan ordered them, and an ordering-dependent race when it did not. For gather, uniqueness
of items remains the plan author's contract: same-path modification across concurrent iterations is a race by
design; fix the plan ([2.3](2.3-orchestration-primitives.md)). The cross-kind rule stays at claim time for
**inputs**: the same URI claimed as two different kinds errors at the earliest claim
([3.5.4](3.5.4-file-provider.md)).

### 4.1 Freshness — the two-path reconciler

`Resolve` on a cache hit runs a two-path reconciler with a shared change-detection front end (landed; the
step-48 capture feeds its recorded side). Every resource exposes the pair: **`Etag`** — cheap, suggestive
(file: a stat tuple; content-addressed types: the URI itself) — and **`Digest`** — the honest content hash,
computed only when the Etag mismatch demands it:

| URI match? | Etag match? | Digest match? | Action |
|---|---|---|---|
| no | — | — | first sighting: intern, return new |
| yes | yes | *(skipped)* | cache hit: return existing — the cheapest path |
| yes | no | yes | **touch drift**: refresh the Etag, return existing, no shadow |
| yes | no | no | real content change — branch on `Addressing()` |

- **`AddressingLocation`** (file, git, appnet, pkg, service): identity is the location, bytes are mutable —
  a real change **shadows** (append the generation, repoint).
- **`AddressingContent`** (mem, function, json, yaml): identity *is* the digest, so URI-match +
  digest-mismatch is impossible by construction — treat it as **corruption** and error; content types never
  accumulate a shadow chain.
- **`AddressingUnknown`** is a tripwire, not a default: no announced resource may return it, and the
  catalog's branch panics if one does — no implicit "location is the default" bias.

Not a push model: divergence is detected when consumers ask, never asynchronously. Touch drift is the
classification that makes relocation safe — judgment scenario 2 (copy the tree, reconcile at the new root)
resolves every entry through exactly this row.

## 5. The Catalog Travels with the Graph

Ruled 2026-08-20 ([plan](../plans/resource-construction.md), feature
[#581](https://github.com/NobleFactor/devlore-cli/issues/581)): **the graph's resource catalog represents
input intent — it is, in effect, what must exist when the graph runs.**

### 5.1 Plan-time claiming — inputs only, pending only

- A **resource-typed parameter with a string value** mints a **pending** entry through the catalog. No
  existence check, no I/O — pending, never resolved; the executor's pre-flight owns transitions (§3).
- A **resource-typed parameter with a promise value** records the promise binding. **No catalog entry** —
  identity arrives when the producer runs.
- A **product** — a method's returned resource — is a **runtime fact**. No plan-time entry, no output
  declaration of any kind (the declared-output-spec proposal is rejected — §9 item 8).
- **String-typed parameters** (`destination_path`, `mode`, `user`, …) stay plain values.

### 5.2 Plan-space paths — the git model

Plan-authored paths are a small portable language: a **leading slash anchors at the fsroot** (as in
`.gitignore`), so `/foo/bar` ≡ `foo/bar`, both naming rel `foo/bar`. Machine-absoluteness is
**inexpressible in a plan** — it arises only from the run's root choice: a home-scope graph binds to the
account running it (`/Users/a`, `/home/b`, `C:\Users\c` — same graph, different accounts); a system-scope
graph binds to the host (`%SystemDrive%\` on Windows, `/` on unix). Volume and drive-letter spellings, and
rels that escape (`../`), are plan-time refusals — the latter is intent confinement can never satisfy.

### 5.3 Identity is rel; the fsroot binds at run

Resource identity is the **slash-canonical root-relative path** — the `fsroot.Path` serialization doctrine
(`rel` is the half that serializes) applied to the catalog. The fsroot is a **run parameter**, unknown until
execution. Activation (pre-flight's Pending → Active) is a *state and binding* event, never an identity
event: afterward the identity is the same rel it was as pending, and the `SourcePath` is the fully bound
triad — `Rel()` the identity verbatim, `Root()` the run's fsroot, `Abs()` derived, OS-native, carrying all
I/O and never serialized. Identity lives in the rel; location lives in the Path; activation joins them
without letting them trade places.

### 5.4 The serialized section — mandatory, even when empty

Every graph document carries a `resources` section: one row per current-generation entry, as intent —
`{id, uri, state: pending}` (all pending by construction: no producers, no Etag/Digest — those are trace
material). Content-addressed entries additionally carry their packed bytes (the content transport, step 25).
The section is present even when empty. **A document without it does not load, and a graph without a catalog
fails pre-flight hard** (`ReasonPreflightFailed`, before any dispatch). `schema_version` stays 1: pre-ruling
documents simply fail to load and are rewritten by re-planning.

### 5.5 Graph = intent; trace = observation

The graph document never records observation. The trace's ledger snapshot (step 48) records what the run
saw: activations with captured content identity, products, transitions, compensations. Pre-flight in one
sentence: **every pending rel must exist under the run's root.** The judgment scenarios that pin this split
live in the plan's "Judgment scenarios" section.

## 6. Recovery — Receipts and the Recovery Site

The pre-`op` `Tombstone` family is gone (phase-8 steps 40/42): the undo record is the **receipt**
([2.2](2.2-phase-execution.md)), and file mutation flows through the unified receipt with `MutationKind`,
boundary, and the recovery-digest tamper check ([3.5.4](3.5.4-file-provider.md)).

`op.RecoverySite` ([5.3](5.3-recovery-site.md), [4.4](4.4-root-path-triad.md)) is the shared archival service —
`ArchiveFile` (zero-copy rename into `.devlore/recovery/` under the root authority, same partition guaranteed),
`ArchiveData` / `ArchiveStream` (byte serialization), `RestoreFile` / `RestoreData`. A receipt's `TransactionID` is
the recovery key. Providers archive before destructive mutation; compensation restores — and the recorded
`recoveryDigest` detects tampering of the recovery store between forward action and compensation.

## 7. The Platform Provider — Data, Not Actions

The platform provider (`pkg/op/provider/platform/`) is the Starlark surface for the runtime environment's
`op.Platform` — the serializable capability + Composite package-manager router of
[3.4](3.4-platform-package-managers.md). Access is `both`: immediate reads answer from the local machine; planned
reads resolve at execution time against the target's platform — the mechanism by which one graph targets many
machines. The graph carries only serialized target identity; managers re-attach at run time, never serialize.

## 8. Provider Lifecycle

Every provider embeds `op.ProviderBase` and is constructed against the `*op.RuntimeEnvironment` by the constructor
its announcement registers (`op.AnnounceProvider(type, role, constructor, metadata)` — see
[3-operation-namespaces.md](3-operation-namespaces.md) and [2.1](2.1-typed-slots.md) §Action dispatch). A provider
follows the lifetime of its runtime environment — per run in a graph, per session in a script. Environment access
is uniform: `p.RuntimeEnvironment()` for the root, catalog, platform, recovery site, narrator.

## 9. Resolved Decisions

1. **Sealed interface** — `Resource` sealed via the unexported base accessor; catalog stores interface values.
2. **URI canonicalization** — applied at construction, pure.
3. **Immediate mode** — immediate receivers pass raw values; cataloged resources are a planning concern.
4. **Catalog scope** — per graph; cloned per run; ownership transfers at assembly.
5. **Recovery ownership** — receipts name the undo; providers own the mechanism; the recovery site is shared.
6. **Coercion vs. resolution** — plan time converts and claims (pure); execution time verifies and transitions
   (the pre-flight pass) — required because a graph is planned once and executed on many machines.
7. **Observations** — results, never catalog members (§3).
8. **Products are runtime facts** (ruled 2026-08-20) — no plan-time entry, no output declaration; the
   declared-output-spec proposal (the former Appendix A) is **rejected**, and plan-time output shadowing
   with it. Ordering is the promise's job.
9. **The catalog travels with the graph** (ruled 2026-08-20) — mandatory serialized section, present even
   empty; absence is a hard load error and a hard pre-flight failure; `schema_version` stays 1 (§5.4).
10. **Identity is rel; the fsroot binds at run** (ruled 2026-08-20) — git-model plan-space paths; activation
    never changes identity (§5.2–5.3).
11. **Graph = intent; trace = observation** (ruled 2026-08-20) — the document records pending intent only;
    the step-48 snapshot records what the run saw (§5.5).

## 10. Open Questions

1. **Remote execution transport** — the graph is portable, and `op.Platform` carries target identity, but the
   pre-flight pass needs a filesystem abstraction for remote targets ([1-system-model.md](1-system-model.md) §6.2
   vision).
2. **Per-type existence rollout** — the staged step-22 rollout: which resource type's `Resolve`/`Exists` lands
   next.

