# Resource Management: URI-Based Resource Tracking

> **Status:** rewritten 2026-07-22 (phase-8 step 51, slice 3) onto the landed `pkg/op` model; **revised
> 2026-08-20 onto the resource-construction rulings** ([plan](../plans/resource-construction.md), feature
> [#581](https://github.com/NobleFactor/devlore-cli/issues/581)): the catalog is input intent and travels with
> the graph (§5), plan-space file paths follow the git model, file identity is root-relative, and **the
> declared-output-spec proposal is rejected** — products are runtime facts, so the former Appendix A is
> removed rather than preserved (§9 item 8). Implementation: phases 0–3
> ([#582](https://github.com/NobleFactor/devlore-cli/issues/582)–[#585](https://github.com/NobleFactor/devlore-cli/issues/585),
> delivered 2026-08-20..22) are in the tree — the serialized section is enforced, file identity is the rel
> with run-bound activation, and plan-time claiming, scoped verification, `MissingResourcePolicy`, and the
> consumed-Gone guard are live; [#586](https://github.com/NobleFactor/devlore-cli/issues/586) (run time
> consumes the catalog at dispatch) and [#587](https://github.com/NobleFactor/devlore-cli/issues/587)
> (closure) remain. Companion: [`4-resource-management.status.md`](4-resource-management.status.md).

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
- **Execution time** — the executor's pre-flight resolve pass binds every pending resource to the run's
  environment (rel-first for root-relative schemes), and each subgraph executor verifies the claims its own
  units consume when its scope starts, by the scheme's existence predicate (for file resources: the rel
  under the run's root) — §3; dispatch results — products included — become catalog facts on the per-run
  clone, recorded by the trace.
- **Dispatch resolves by identity** — at run time a string may be a **key, never a constructor**: a
  resource-typed slot value resolves against the run catalog, retrieving the claimed entry or refusing;
  run-computed paths enter through the explicit discovery/resolution actions (§5.6–§5.7).

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
Shadow(r, producerID) → id         ← new generation at an occupied URI; namespace repointed (§4)
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
- **Gone** — the resource is no longer there: an existence check failed (reactive), or a mutating consumer
  destroyed the resource and reported it (ruled 2026-08-20 — the remove is the authority; its receipt carries the
  undo). Terminal for the entry; reviving a URI appends a fresh entry.

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
          └────────┘  (a later failed check, or a mutating consumer's destruction → Gone)
```

#### Catalog behavior matrix

Ruled 2026-07-14 (step 22); **re-stated 2026-08-20 (Q1 ruling)** to agree with §5.1 and the code: the
2026-07-14 rendering folded pre-flight's existence outcomes into the `Discover` rows, which read as
discover-time I/O — `Discover` performs none, and existence belongs to pre-flight alone.

**Claiming (plan time — no I/O anywhere in this table):**

| Op | Cache state | Content-addressable | Location-based |
|---|---|---|---|
| `Discover` | miss | append `Pending`; no `producerID` | same |
| `Discover` | hit, `Pending` | return existing; discard input | same |
| `Discover` | hit, `Active` | return existing | same |
| `Discover` | hit, `Gone` | return error | same |
| `Discover` | (any) | **never shadows** | **never shadows** |

**Verification (scoped pre-flight — `VerifyExistence`, the only place `Exists()` runs). Ruled 2026-08-22:
verification is per subgraph executor.** A graph is an object holding a root subgraph — there is only
subgraph execution — so each executor verifies, when its scope starts, the claims its own units consume.
The root executor verifies unconditional claims at the run's starting line; a choose case (itself a
subgraph) verifies its claims only when it is hit, by which point mid-run production may already have
satisfied them. Unreached scopes never judge their claims.

**`Exists` is kind-honest** (ruled 2026-08-22, phase 4 PR 3/#611): each file taxonomy kind's predicate is
lstat plus a kind test — a `*Regular` claim over a directory or a symbolic link answers false, so a
wrong-kind claim fails verification at the starting line ("claims are true when made") instead of
activating kind-blind. The mutator family is kind-honest with it: `file.move_directory` moves a
directory under a `*Directory` claim (retiring the #585 C2 kind-looseness).

| Entry state | `Exists()` | Result |
|---|---|---|
| `Pending` | true | `Pending` → `Active` |
| `Pending` | false | **the transition fails**: `Pending` → `Gone`, with a warning always produced. Under `MissingResourcePolicyStop` (the default) the consuming scope fails — a pending resource that fails its scheme's existence predicate (for **file resources**: the rel does not exist relative to the run's fsroot) is unmet intent (§5.5). Under `Ignore` the scope proceeds and the consumer applies its policy at dispatch. |
| `Active` | false (a later re-check) | `Active` → `Gone` |

**The claims taxonomy (ruled 2026-08-22; policy model ruled the same day).** Every literal claim is
**required** by default — all resources named as literals are expected to exist when their consuming scope
starts. Per-consumption tolerance is expressed by **`MissingResourcePolicy`**, an enumeration with explicit
values (never iota) and a fail-safe zero:

- `MissingResourcePolicyStop` (**0 — the zero value and the default**): a missing resource fails the
  consuming scope — unmet intent.
- `MissingResourcePolicyIgnore`: the call is **made anyway** — the provider sees the absence and handles it
  (a remove no-ops; the receipt records "target was already absent" so compensation of the no-op is a
  no-op).

A **Skip** variant ("do not dispatch") was considered and **DROPPED (ruled 2026-08-22)**: its undo story is
trivially clean — nothing ran, nothing to undo — but its forward side (nil-valued promises to downstream
consumers; a trace that cannot tell "skipped" from "ran and produced nothing") buys machinery Ignore never
needs, and the structural way to author optional steps already exists (a choose case, whose claims are
judged only when hit). Re-adding it later is purely additive.

**A warning is produced whenever a missing resource is detected, under every policy.** The parameter's TYPE
is the declaration — no directive: at announcement, a method with a `MissingResourcePolicy`-typed parameter
and exactly one consumed (resource-typed) parameter links the two; more than one consumed parameter beside
one policy is ambiguous and refuses at announcement. The claim still enters the catalog under every policy
(identity, compensation, and drift all want the entry). Aggregation across consumers of one entry: **Stop
wins** — any Stop consumer in the verifying scope fails it; otherwise each Ignore consumer applies its
policy at dispatch.

**Conditionality is structural, never declared**: a claim consumed only inside a conditional subgraph is
verified by that subgraph's executor when the branch is hit. The one deliberately strict case: an
*unconditional* consumer of a file that an earlier unit creates with no promise edge stays a pre-flight
failure — ordering-by-coincidence is what "ordering comes from promises" forbids; consume the producer's
promise and no pending entry exists to check at all.

**Production (dispatch time — products are runtime facts, §5.1):**

| Op | Cache state | Content-addressable | Location-based |
|---|---|---|---|
| `NewResource` | miss | append `Pending` → `Active`; stamp `producerID` | same |
| `NewResource` | hit, `Pending`/`Active` | return existing (singleton) | **shadow**: new entry, stamp `producerID`; old entry stays as history |
| `NewResource` | hit, `Gone` | **shadow** (revives the URI) | same |

**Consumption (dispatch time — ruled 2026-08-20, judgment scenario 1):**

| Consumed entry state | Result |
|---|---|
| `Active` | dispatch proceeds; a mutating consumer that destroys the resource transitions the entry `Active` → `Gone`, and its receipt carries the undo |
| `Gone` | **a `Stop` consumer fails on the catalog's NARRATED verdict** — "consumes X, destroyed by unit N before it could run" (the destroyer stamp) — it sees the state; it does not rediscover the loss through its own I/O. An `Ignore` consumer makes the call: its provider handles the absence. A warning is produced in every case. |
| `Pending` | unreachable — pre-flight has already activated the entry or failed the run |

Where the guard lives (ruled 2026-08-20): **the action dispatch seam** — `Method.Invoke` converts slot values
to Go arguments via `Convert`, and the check runs **after conversion, before the forward method is called**.
After conversion is the earliest point the complete consumed set exists: promise-fed and defaulted slots
resolve only at slot-fill, so a purely static executor-side guard would miss promise-fed consumption. The
seam is also shared by graph dispatch and starlark immediate dispatch, so the verdict holds on both surfaces.
The guard closes a live asymmetry: `Discover` errors on a hit-`Gone` entry (the claiming table), but
`Convert`'s catalog hit (`Current`/`Lookup`) screens no state — today it hands a consumer the `Gone`
generation silently. The guard's error is typed; the executor classifies it into the run-status reason
(the failure is structural — the forward method never ran, so there is nothing to compensate for this unit).
The catalog-resolve verdict remains the in-flight backstop, and provider I/O errors still catch losses the
catalog cannot know (out-of-band deletion mid-run). The `Gone` transition stamps the destroying unit —
symmetric with `producerID` — so the verdict names both units: *consumed by unit 1 before unit 2 could run*.

Delivered (#585 PRs C, D, and C2 — #604/#605/#606, 2026-08-22): the remove consumes its resource-typed
target and transitions the entry with the destroyer stamp; the guard at the dispatch seam gives the second
consumer the narrated verdict; and C2 completed the family — every file-scheme mutator (`Remove`, `Unlink`,
`Move`'s source, `RemoveAll`) is a resource-typed consumer gated by `on_missing`, a successful move marking
its source Gone with the stamp. Judgment scenario 1 pins the full ruled shape end to end.

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

### 5.2 Plan-space paths — the git model (file resources)

Plan-authored **file paths** are a small portable language: a **leading slash anchors at the fsroot** (as in
`.gitignore`), so `/foo/bar` ≡ `foo/bar`, both naming rel `foo/bar`. Machine-absoluteness is
**inexpressible in a plan** — it arises only from the run's root choice: a home-scope graph binds to the
account running it (`/Users/a`, `/home/b`, `C:\Users\c` — same graph, different accounts); a system-scope
graph binds to the host (`%SystemDrive%\` on Windows, `/` on unix). Volume and drive-letter spellings, and
rels that escape (`../`), are plan-time refusals — the latter is intent confinement can never satisfy.

### 5.3 File identity is rel; the fsroot binds at run

**File-resource** identity is the **slash-canonical root-relative path** — the `fsroot.Path` serialization doctrine
(`rel` is the half that serializes) applied to the catalog. The fsroot is a **run parameter**, unknown until
execution. Activation (pre-flight's Pending → Active) is a *state and binding* event, never an identity
event: afterward the identity is the same rel it was as pending, and the `SourcePath` is the fully bound
triad — `Rel()` the identity verbatim, `Root()` the run's fsroot, `Abs()` derived, OS-native, carrying all
I/O and never serialized. Identity lives in the rel; location lives in the Path; activation joins them
without letting them trade places.

### 5.4 The serialized section — mandatory, even when empty

Every graph document carries a `resources` section: one row per current-generation entry, as intent —
**`{id, uri}` and nothing else** (ruled 2026-08-21; delivered 2026-08-22, #585 PR A/#602). Intent needs no state
field: presence in the section IS the pending claim — pending is definitional, not recorded — and
producers, Etag/Digest, and state are trace vocabulary (§5.5), so the intent row is its own type rather
than a borrowed trace row. Content-addressed entries additionally carry their packed bytes (the content
transport, step 25). The section is present even when empty. **A document without it does not load, and a
graph without a catalog fails pre-flight hard** (`ReasonPreflightFailed`, before any dispatch).
`schema_version` stays 1: pre-ruling documents simply fail to load and are rewritten by re-planning. Known
boundary, accepted: today's decoders are lenient, so a stray field (e.g. a hand-edited `state:`) is ignored
rather than refused — strict decoding is a codec-wide property, not this section's.

### 5.5 Graph = intent; trace = observation

The graph document never records observation. The trace's ledger snapshot (step 48) records what the run
saw: activations with captured content identity, products, transitions, compensations — and **the binding**:
the snapshot names the root the run bound (its `root` field, stamped by the executor at capture), because
file identities are rels and a recorded rel is interpretable only against the root that ran it. Consumers
derive an entry's native path by joining the recorded root with the URI's rel payload — identity is never
parsed for a native form (the #547 rule). **The binding is enforced at the executor's pre-flight resolve
pass** (implemented 2026-08-21, #584 PR 3): every pending entry re-bases onto the run's environment, and a
root-relative scheme re-binds its path rel-first through the `op.RootBinder` seam (`file` implements it) —
so existence, Etag, Digest, and I/O all read the run's world, never the environment that constructed or
rehydrated the object. Pre-flight in one sentence (scoped per the 2026-08-22 ruling): **every required pending resource must
satisfy its scheme's existence predicate when its consuming scope starts — for file resources, the rel must
exist under the run's root.** The judgment scenarios that pin this split live in the plan's "Judgment
scenarios" section.

### 5.6 Run time consumes the catalog — a string is a key, never a constructor

Ruled 2026-08-22 (the sketch's rule made precise; implementation
[#609](https://github.com/NobleFactor/devlore-cli/issues/609)/[#610](https://github.com/NobleFactor/devlore-cli/issues/610)
under [#586](https://github.com/NobleFactor/devlore-cli/issues/586)): **no string-to-resource conversion
ever happens at run time — at graph dispatch a string may be a key, never a constructor.** A resource-typed slot value —
captured object or rehydrated URI string alike — resolves against the run catalog, and resolution
retrieves the entry the plan already claimed or refuses with a typed error naming the URI; it never mints.
The refusal is enforceable because the catalog is complete by construction (§5.1): a dispatch miss can
only mean a doctored document or a defect. Construction from strings survives in exactly two places:
load-time rehydration — a provider decoding its own emitted identity, the inverse of its serialization (an
identity decode, not a conversion) — and immediate mode (§9 item 3). Production and discovery are the
sanctioned channels for resources that come into being at run time; the dispatch seam only ever looks up.

### 5.7 Explicit discovery and resolution — run-computed paths

A run-computed path — a regex over tool output, an opaque command's side-effect file — cannot be claimed
(its value does not exist at plan time) and must not convert at dispatch (§5.6). The sanctioned channel is
a pair of explicit file actions (ruled 2026-08-22; implementation
[#611](https://github.com/NobleFactor/devlore-cli/issues/611) under
[#586](https://github.com/NobleFactor/devlore-cli/issues/586)):

- **`file.discover(path, kind?="entry")`** — lstat: interns the entry at the path itself, no follow.
- **`file.resolve(path, kind?="entry")`** — stat: interns what the chain designates, which is never a
  link; confinement-judged.

The rules, each ruled 2026-08-22:

1. **Stop-only.** A missing target, kind mismatch, dangling chain, or confinement escape is the action's
   own error at its own node. No `on_missing`: an Ignore would return nothing and put a nil promise in
   every downstream slot — the cost that had Skip dropped from `MissingResourcePolicy`. Tolerance stays
   structural (probe + choose) or at the consumer.
2. **`kind` is opt-in strictness.** A named enum (`entry`, `regular`, `directory`, `symbolic_link`);
   `entry` is the default — the short spelling is permissive, and asserting a kind sharpens the verdict
   at the action's own node. Results intern as discoveries: observed facts, no production claim.
3. **The runtime path grammar is plan-space plus an under-root rebase.** Rels and anchored spellings
   normalize as authored; escapes and `@name` refuse as authored; a machine-absolute input rebases to its
   rel when it falls under the bound run root and refuses as a confinement violation otherwise. Run time
   may speak absolutes because the root is bound — machine-absoluteness arises only from the run's root
   choice (§5.2), and these actions sit on the far side of that choice.
4. **The literal-path discriminator.** A file that must exist when the run starts is claimed (§5.1 —
   pre-flight's verdict); a file that comes into being mid-run — an opaque command's side effect at a
   known path — is discovered. Guidance, not enforcement: no plan-time test can tell the cases apart.
5. **The follow doctrine.** Kinds are lstat-strict at consumption (a symlink to a regular file is kind
   symbolic-link, never regular), and the parameter type is the follow-policy declaration:
   `*Regular`/`*Directory` demand that kind, no follow; `*SymbolicLink` is the link itself; `Entry`
   accepts any kind, the method assuming kind-switch, confinement judgment, and interning duties for any
   follow it performs. Implicit follow at the dispatch seam never happens: a silent follow aliases one
   disk entity under two catalog identities — mediation cannot see the join — and a symlink is the disk's
   `../`, escaping the confinement the grammar enforces. The kernel resolves names implicitly at open;
   this model resolves designation explicitly at a unit.
6. **An authored string into an `Entry`-typed slot refuses at plan time.** A claim asserts a kind and
   `Entry` asserts none; the author states the kind or feeds a discovery.
7. **The fail-fast boundary.** Pre-flight's verdict covers claims — unmet intent fails before any
   dispatch. A discovery verifies at its own node, the earliest moment the fact exists: discover/resolve
   failures are mid-run by nature. And nothing stops an out-of-band actor deleting a file under a running
   graph, short of a lockdown on the targeted fsroot directory — the observation layer and reconciliation
   are the designed response, not prevention.
8. **The pure ordering edge** (decided at PR 3/#611): `after`, typed `op.OrderingEdge`, consumes an
   upstream invocation's promise solely for sequencing — the edge orders, and the delivered value is
   discarded BY TYPE (every source converts to the empty edge). The dedicated type exists because `any`
   cannot carry the contract: an invocation bound to an any-typed parameter captures the
   flow-combinator convention — the unit itself — instead of its promise.
9. **The runtime dialect's leading-slash sharpening** (decided at PR 3/#611): on unix a leading slash is
   ambiguous between the anchored spelling and machine-absoluteness, and at run time the machine reading
   wins — tools emit machine absolutes; the two readings agree under the root, and an out-of-root
   absolute refuses rather than silently confining. Authors of literals write bare rels.

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
12. **The claims taxonomy** (ruled 2026-08-22) — every literal claim required by default; per-consumption
    tolerance via `MissingResourcePolicy` (`Stop` the fail-safe zero and default, `Ignore`; **Skip
    dropped**); the parameter's type is the declaration; a warning on every detection; Stop wins
    aggregation (§3).
13. **Verification is scoped** (ruled 2026-08-22) — each subgraph executor verifies the claims its own
    units consume when its scope starts; conditionality is structural, never declared (§3).
14. **Mutation targets are resource-typed consumers** (ruled 2026-08-20; delivered 2026-08-22, #585) — the
    mutator claims its target, destruction stamps the destroyer, and the consumed-Gone guard at the
    dispatch seam narrates the verdict (§3).
15. **A string is a key, never a constructor** (ruled 2026-08-22) — dispatch resolves resource-typed slot
    values by identity against the run catalog; a miss refuses; conversion survives only in load-time
    rehydration (an identity decode) and immediate mode (§5.6).
16. **Conversion is explicit** (ruled 2026-08-22) — run-computed paths enter through
    `file.discover`/`file.resolve`: Stop-only, kind enum with `entry` default, runtime grammar with the
    under-root rebase, the starting-line/mid-run discriminator (§5.7).
17. **The follow doctrine** (ruled 2026-08-22) — kinds are lstat-strict at consumption; the parameter type
    declares follow policy; implicit follow never happens; `Entry`-typed slots refuse authored strings at
    plan time (§5.7).
18. **Claims are true when made** (ruled 2026-08-22) — falseness is a mediation failure with four doors;
    kind-honest activation (per-kind `Exists`; the activation capture kind-mismatch becomes a verdict) is
    chartered in the plan's phase-4 docket.

## 10. Open Questions

1. **Remote execution transport** — the graph is portable, and `op.Platform` carries target identity, but the
   pre-flight pass needs a filesystem abstraction for remote targets ([1-system-model.md](1-system-model.md) §6.2
   vision).
2. **Per-type existence rollout** — the staged step-22 rollout: which resource type's `Resolve`/`Exists` lands
   next.

