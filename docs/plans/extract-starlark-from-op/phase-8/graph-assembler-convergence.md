---
title: "Phase 8 · graph-assembler convergence — NewGraph is the only *Graph builder; load goes through it"
parent: "docs/plans/extract-starlark-from-op/phase-8/unify-subgraph-execution.md"
issue: TBD
status: approved
created: 2026-06-06
updated: 2026-06-06
---

# Graph-assembler convergence

## Goal

`NewGraph` is the single `*Graph` constructor and the only place that hand-builds `&Graph{}`. `LoadGraph` decodes;
`assembleGraph` (the renamed `buildGraphFromPayload`) assembles the units along the way and goes **through `NewGraph`**.
The root spec falls out as a **local** in `NewGraphSpec` (`NewRootSubgraphSpec` deleted), collapsing the root
call-sites from three to one. The graph-load code moves beside the constructors it mirrors; `load.go` is deleted.

## Motivation

`NewGraph` (graph.go:129) and `buildGraphFromPayload` (load.go:84) both hand-build `&Graph{}` — two assemblers, which
violates "hand-building is off-limits outside the one constructor (and outside tests)." Remove the second hand-builder,
collapse the root call-sites, and reunite the construct / serialize / deserialize family.

## Design

### 1. `NewGraph` is the sole `*Graph` builder; load goes through it

- `NewGraph(spec *GraphSpec) (*Graph, error)` stays the **only** place that builds `&Graph{}`. (There is no separate
  shared assembler — an earlier draft invented one; that was the inconsistency. `assembleGraph` is *not* a new builder;
  it is the renamed load deserializer below.)
- `LoadGraph(env, data, format)` decodes the bytes to a payload and calls `assembleGraph`.
- `assembleGraph(env, payload)` (renamed `buildGraphFromPayload`) assembles each node / subgraph **along the way** via
  `assembleNode` / `assembleSubgraph`, links them under the root, builds a `GraphSpec`, and calls `NewGraph`. It no
  longer hand-builds `&Graph{}`.

So `assembleNode` / `assembleSubgraph` / `assembleGraph` are the renamed `build*FromPayload` family; `NewGraph` is the
one constructor they funnel into.

### 2. Metadata — the round-trip wrinkle, resolved

`NewGraph` does not *take* checksum/signature: it **computes** the checksum from `CanonicalContent` (graph.go:154),
**signs** via the spec's SopsClient, and sets `timestamp = now` + `schemaVersion = GraphSchemaVersion`. Because
`CanonicalContent` **includes the timestamp** (graph.go:299/322), a fresh `now` would change the canonical content and
thus the recomputed checksum — breaking the round-trip.

So load must feed `NewGraph` the document's provenance. Add `graphMetadata` (in graph.go) carrying the document's
`schemaVersion`, `timestamp`, and `signature`. `GraphSpec` carries it optionally, alongside `Origin` (already spec
provenance):

- **Construction**: metadata nil → `NewGraph` generates fresh (timestamp=now, current schemaVersion, signs via
  SopsClient).
- **Load**: `assembleGraph` fills it from the payload → `NewGraph` uses the document's timestamp / schemaVersion /
  signature and recomputes the checksum, which then **matches** the document's (identical content) — giving load an
  implicit integrity check (recomputed vs the document's stored checksum).

### 3. Root spec as a local; delete the factory

With load funnelling through `NewGraph`, the root spec is created in exactly one place — `NewGraphSpec`. Inline it and
delete the factory:

```
func NewGraphSpec() *GraphSpec {
    return &GraphSpec{Root: *NewSubgraphSpec().WithID("root").WithActionNamed("flow.subgraph")}
}
```

`NewRootSubgraphSpec` is deleted. **var** rejected (its `WithActionNamed` initializer panics at `pkg/op` load, before
flow is announced; shared-mutation aliasing). **func** rejected (DRY moot at one site).

### 4. Naming + placement

- `graph.go` order: `NewGraph`, `NewGraphSpec`, `LoadGraph`, `assembleGraph` (directly below `LoadGraph`), plus the
  `graphMetadata` type.
- `assembleNode` (renamed `buildNodeFromPayload`) → `node.go` below `NewNode`.
- `assembleSubgraph` (renamed `buildSubgraphFromPayload`) + `(*Subgraph).linkChildren` → `subgraph.go` below
  `NewSubgraph`.
- `resolvePayloadAction` → `helpers.go`.
- `load.go` deleted (emptied).

## Sequence diagrams

Both paths converge on `NewGraph` as the single `&Graph{}` builder.

### `NewGraph` — construction (in-memory)

```
caller → NewGraphSpec()                         ⟶ *GraphSpec{ Root: root spec, ActionNamed "flow.subgraph" }
caller → spec.WithUnits(...)                     ⟶ children added under spec.Root
caller → NewGraph(spec)
          ├─ NewSubgraph(&spec.Root)             ⟶ *Subgraph (root; "flow.subgraph" by name, resolved later at dispatch)
          ├─ metadata == nil → generate fresh    : timestamp = now, schemaVersion = GraphSchemaVersion
          ├─ CanonicalContent → GitStyleChecksum : checksum
          ├─ spec.SopsClient.Sign(canonical)     : signature (nil when unsigned)
          └─ &Graph{…}                           ◀── THE single &Graph{} build
          ⟵ *Graph
```

### `LoadGraph` — deserialization (document)

```
caller → LoadGraph(env, data, format)
          ├─ decode(data, format)                ⟶ payload (graphData)   ← LoadGraph's only job: decode
          └─ assembleGraph(env, payload)
               ├─ assembleNode(env, n)       [per node]      ⟶ *Node      (resolvePayloadAction → NewNode)
               ├─ assembleSubgraph(env, sg)  [per subgraph]  ⟶ *Subgraph  (resolvePayloadAction → NewSubgraph)
               ├─ linkChildren(...)                          : wire units under the root (topological)
               ├─ build GraphSpec{ Root+children, Edges, graphMetadata ← payload (schemaVersion, timestamp, signature) }
               ├─ NewGraph(spec)
               │    ├─ NewSubgraph(&spec.Root)               ⟶ *Subgraph (root)
               │    ├─ metadata set → use document's         : timestamp, schemaVersion, signature
               │    ├─ CanonicalContent → GitStyleChecksum   : checksum (== document's, same content)
               │    └─ &Graph{…}                             ◀── THE single &Graph{} build (shared with construction)
               │    ⟵ *Graph
               └─ verify recomputed checksum == payload.checksum   (integrity)
               ⟵ *Graph
          ⟵ *Graph
```

### Invariants the diagrams encode

- **One `&Graph{}` build** — only inside `NewGraph`; both paths route through it.
- **`LoadGraph` only decodes**, then delegates to `assembleGraph`.
- **`assembleGraph` assembles units along the way** (`assembleNode` / `assembleSubgraph`), wires them, then funnels
  through `NewGraph`.
- **Metadata branch in `NewGraph`**: nil ⇒ fresh (construction); set ⇒ the document's (load), so the recomputed
  checksum matches and the round-trip holds.

## Blast radius

- `graph.go`: + `graphMetadata`; `GraphSpec` gains an optional metadata field; `NewGraph` honors it (fresh when nil);
  `NewGraphSpec` inlines the root; `LoadGraph` + `assembleGraph` moved in below `NewGraph`.
- `subgraph.go`: `NewRootSubgraphSpec` deleted; `assembleSubgraph` + `linkChildren` moved in.
- `node.go`: `assembleNode` moved in.
- `helpers.go`: `resolvePayloadAction` moved in.
- `load.go`: deleted.
- Root call-sites: 3 → 1 (`NewGraphSpec`).

## Verification

- `pkg/op` + `pkg/op/provider/flow` green (scoped; whole-tree still red on #6).
- **Round-trip preserved**: load through `NewGraph` with the document's metadata recomputes a checksum equal to the
  document's, and preserves timestamp / schemaVersion / signature — a round-trip test asserts equality.
- Grep gates: no `&Graph{}` outside `NewGraph`; `NewRootSubgraphSpec` gone; `load.go` gone.

## Sequencing

1. Add `graphMetadata` + the optional `GraphSpec` metadata field; make `NewGraph` honor it (fresh when nil). Verify
   construction tests + that a constructed graph is unchanged (metadata nil path).
2. Rename `buildGraphFromPayload` → `assembleGraph`; rebuild it to assemble units + a `GraphSpec` (with the document's
   metadata) and call `NewGraph` instead of hand-building `&Graph{}`. `LoadGraph` decodes → `assembleGraph`. Verify
   load + round-trip tests (recomputed checksum == document's).
3. Inline the root spec in `NewGraphSpec`; delete `NewRootSubgraphSpec`. Verify.
4. Rename + relocate `assembleNode` / `assembleSubgraph` / `linkChildren` / `resolvePayloadAction`; delete `load.go`.
   Verify build + tests + the grep gates.

## Status

- 2026-06-06 — **approved** (sequence diagrams added); ready to implement per the Sequencing section.
