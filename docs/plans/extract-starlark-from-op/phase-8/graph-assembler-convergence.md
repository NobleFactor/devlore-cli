---
title: "Phase 8 · graph-assembler convergence — one private buildGraph; NewGraph + load both go through it"
parent: "docs/plans/extract-starlark-from-op/phase-8/unify-subgraph-execution.md"
issue: TBD
status: approved
created: 2026-06-06
updated: 2026-06-06
---

# Graph-assembler convergence

## Goal

One private `buildGraph` is the **only** place that hand-builds `&Graph{}`. `NewGraph` (construction) and the load path
each prepare a root + metadata their own way and call it. The root spec becomes a **local** in `NewGraphSpec`
(`NewRootSubgraphSpec` deleted), collapsing the root call-sites from three to one. The load code relocates beside its
constructors; `load.go` is deleted.

## Motivation

`NewGraph` (graph.go:129) and `buildGraphFromPayload` (load.go:84) both hand-build `&Graph{}`. Load **cannot** simply
route through `NewGraph` (option rejected): `NewGraph → populate` re-derives edges via `materializeEdges` (slot-producers
only — never subgraph→subgraph edges, never a non-producer edge), whereas load must **preserve** the document's edges
(set *and* order) or the round-trip checksum breaks (`CanonicalContent` serializes the edge slice; e.g.
`marshalers_test.go:76`'s hand edge would re-derive to nothing). The two also use different child-construction models
(load: flat build + placeholder `linkChildren`; `populate`: spec-children + `materializeEdges`).

So the shared builder sits **below both**: a private `buildGraph` that takes an already-prepared root (whose edges the
caller has set the way it needs) plus metadata. Each preparer keeps its own edge handling; the single `&Graph{}` build
is shared.

## Design

### 1. One private builder; two preparers

- **`buildGraph(root *Subgraph, metadata graphMetadata) *Graph`** — the only place that hand-builds `&Graph{}`. It sets
  the struct fields from `metadata`, derives `unitsByID` by walking the root's descendants, and recomputes the checksum
  from `CanonicalContent`. Private, in graph.go.
- **`NewGraph(spec *GraphSpec) (*Graph, error)`** — builds the root from the spec via `NewSubgraph(&spec.Root)`
  (`populate` derives edges from slot-producers and sorts children, unchanged), assembles **fresh** metadata
  (timestamp=now, current schemaVersion, signs the canonical via `spec.SopsClient`), and calls `buildGraph`.
- **the load path** — builds the root from the payload (units via `assembleNode`/`assembleSubgraph`, linked, with the
  **document's** edges set directly), takes the **document's** metadata, and calls `buildGraph`.

Neither hand-builds `&Graph{}`; each *prepares* its root (construction derives edges; load preserves them) and calls the
shared builder. That satisfies "one place hand-builds `&Graph{}`" without distorting `NewGraph`'s edge derivation or
load's edge preservation.

### 2. Metadata + edges (the round-trip)

`graphMetadata` (graph.go) carries `schemaVersion` (uint32), `timestamp` (time.Time), `signature` (*sops.Signature).

- **Construction** (`NewGraph`): fresh — timestamp=now, schemaVersion=`GraphSchemaVersion`, and it signs the canonical
  via `spec.SopsClient` (nil when unsigned).
- **Load**: the document's — schemaVersion/timestamp from the payload, and the document's signature is **preserved**
  (not re-signed).
- **Edges** live on `root.edges`: construction derives them in `populate`; load sets them from the document. `buildGraph`
  recomputes the checksum from `CanonicalContent` (root + edges + metadata), so the load path's preserved edges yield a
  canonical identical to the document's → recomputed checksum **equals** the document's (round-trip + an implicit
  integrity check).

### 3. Root spec as a local; delete the factory

Inline the canonical root in `NewGraphSpec`: `&GraphSpec{Root: *NewSubgraphSpec().WithID("root").WithActionNamed("flow.subgraph")}`.
Delete `NewRootSubgraphSpec`. (var rejected — init-time `WithActionNamed` panic + aliasing; func rejected — DRY moot at
one site.)

### 4. Naming + placement

- `graph.go` order: `NewGraph`, `NewGraphSpec`, **`buildGraph`** (private shared builder), `LoadGraph`, `assembleGraph`
  (directly below `LoadGraph`), and the `graphMetadata` type.
- `assembleGraph` (renamed `buildGraphFromPayload`) is the load deserializer; `assembleNode` (renamed
  `buildNodeFromPayload`) → `node.go` below `NewNode`; `assembleSubgraph` (renamed `buildSubgraphFromPayload`) +
  `(*Subgraph).linkChildren` → `subgraph.go` below `NewSubgraph`; `resolvePayloadAction` → `helpers.go`.
- `load.go` deleted.

## Sequence diagrams

Both paths converge on the private `buildGraph`.

### `NewGraph` — construction (in-memory)

```
caller → NewGraphSpec()                   ⟶ *GraphSpec{ Root: root spec, ActionNamed "flow.subgraph" }
caller → spec.WithUnits(...)               ⟶ children under spec.Root
caller → NewGraph(spec)
          ├─ NewSubgraph(&spec.Root)       ⟶ *Subgraph root; populate DERIVES edges (slot-producers) + sorts children
          ├─ metadata = fresh              : timestamp=now, schemaVersion=GraphSchemaVersion, sign(canonical) via Sops
          └─ buildGraph(root, metadata)    ◀── the single &Graph{} build (recompute checksum, derive unitsByID)
          ⟵ *Graph
```

### `LoadGraph` — deserialization (document)

```
caller → LoadGraph(env, data, format)
          ├─ decode(data, format)          ⟶ payload (graphData)
          └─ assembleGraph(env, payload)
               ├─ assembleNode / assembleSubgraph    ⟶ units (resolvePayloadAction → NewNode/NewSubgraph)
               ├─ linkChildren + set root.edges = payload.Edges   : PRESERVE the document's edges (set + order)
               ├─ metadata = document's     : schemaVersion, timestamp, signature (preserved, not re-signed)
               └─ buildGraph(root, metadata)   ◀── the SAME single &Graph{} build (checksum recomputes == document's)
               ⟵ *Graph
          ⟵ *Graph
```

### Invariants

- **One `&Graph{}` build** — only inside `buildGraph`; `NewGraph` and the load path both call it, neither hand-builds.
- **Edge handling stays where it belongs** — `NewGraph` derives, load preserves; `buildGraph` is agnostic (reads
  `root.edges`).
- **Round-trip** — load preserves edges + metadata, so `buildGraph`'s recomputed checksum equals the document's.

## Blast radius

- `graph.go`: + `graphMetadata` + `buildGraph`; `NewGraph` refactored to prepare root+metadata and call `buildGraph`;
  `NewGraphSpec` inlines the root; `LoadGraph` + `assembleGraph` moved in.
- `subgraph.go`: `NewRootSubgraphSpec` deleted; `assembleSubgraph` + `linkChildren` moved in.
- `node.go`: `assembleNode` moved in.
- `helpers.go`: `resolvePayloadAction` moved in.
- `load.go`: deleted.
- Root call-sites: 3 → 1 (`NewGraphSpec`).

## Verification

- `pkg/op` + `pkg/op/provider/flow` green (scoped; whole-tree red on #6).
- **Round-trip preserved**: a save→load(→save) test asserts the recomputed checksum equals the document's, with
  edges/timestamp/schemaVersion/signature intact.
- Grep gates: no `&Graph{}` outside `buildGraph`; `NewRootSubgraphSpec` gone; `load.go` gone.

## Sequencing

1. Add `graphMetadata` + `buildGraph(root, metadata)`; refactor `NewGraph` to prepare the root (via `NewSubgraph`) +
   fresh metadata and call `buildGraph`. Gate: construction tests unchanged (incl. marshal round-trip for constructed
   graphs).
2. Rename `buildGraphFromPayload` → `assembleGraph`; have it build the root from the payload (preserving the document's
   edges) + document metadata and call `buildGraph` — drop the hand-built `&Graph{}`. `LoadGraph` decodes → `assembleGraph`.
   Gate: load + round-trip (recomputed checksum == document's).
3. Inline the root spec in `NewGraphSpec`; delete `NewRootSubgraphSpec`. Gate: build + tests.
4. Rename + relocate `assembleNode` / `assembleSubgraph` / `linkChildren` / `resolvePayloadAction`; delete `load.go`.
   Gate: build + tests + grep.

## Status

- 2026-06-06 — approved. Step 2's edges gap resolved by **Option B** (shared private `buildGraph`); plan revised.
