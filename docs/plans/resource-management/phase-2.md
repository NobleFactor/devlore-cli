# Phase 2: Graph Integration

## Context

Phase 2 wires the `ResourceManager` and `NamespaceMap` (from Phase 1) into the
`Graph` lifecycle. After this phase, planned receivers can call Resolve/Shadow
during planning, and `FillSlot` creates implicit edges from resource identity.

Phase 1 shipped the core types: `ResourceManager` (append-only ledger),
`NamespaceMap` (URI-to-ResourceID with Resolve/Shadow), URI helpers with
scheme constants.

**Repo**: devlore-cli
**Branch**: `feature/resource-management`

## Changes

### Graph Owns Manager and Namespace (`pkg/op/graph.go`)

Added `Resources *ResourceManager` and `Namespace *NamespaceMap` fields to
`Graph`. Both are initialized in `NewGraph` and tagged `json:"-" yaml:"-"`
(planning-only state, not serialized).

### FillSlot Detects Resource Identity (`pkg/op/output.go`)

Extended `FillSlot` to check whether an immediate value carries resource
identity. When the value embeds `op.Resource` with a non-empty
`OriginNodeID`, an implicit edge is created from the origin node to the
consumer. This enables automatic dependency ordering when a resource
produced by one node flows to another — without explicit `Output` passing.

### `extractResource` Reflection Helper (`pkg/op/resource.go`)

Added `extractResource(v any) (Resource, bool)` which extracts resource
identity from three value forms:

- Direct `op.Resource` value
- Go struct embedding `op.Resource` (e.g., `file.Resource`)
- `map[string]any` with `"uri"`/`"id"`/`"origin_node_id"` keys (produced
  by `Unmarshal` when a starlark struct is decoded to `*any`)

The map form also handles nested `"resource"` keys (from structs that
embed `op.Resource` — the embedding serializes as a nested map key).

### Remove Dead `backup` Annotation (`pkg/op/graph.go`)

Removed `Summary.BackedUp` field and the `Annotations["backup"]` check
from `ComputeSummary`. No production code ever sets this annotation — it
only appeared in test fixtures. The resource ledger (Decision #9) replaces
per-node annotations for tracking resource state.

### Tests

**`pkg/op/resource_test.go`** (new tests):
- `TestExtractResource_DirectResource` — plain op.Resource value
- `TestExtractResource_EmbeddedResource` — struct embedding op.Resource
- `TestExtractResource_PointerToEmbedded` — pointer to embedding struct
- `TestExtractResource_MapFromUnmarshal` — map[string]any with resource fields
- `TestExtractResource_MapWithoutResourceFields` — unrelated map returns false
- `TestExtractResource_NonResource` — nil, string, int, map, slice all return false
- `TestExtractResource_NilPointer` — nil pointer returns false

**`pkg/op/output_test.go`** (new tests):
- `TestFillSlotImplicitEdge_ResourceWithOrigin` — embedded resource creates edge
- `TestFillSlotImplicitEdge_ResourceWithoutOrigin` — discovered resource creates no edge
- `TestFillSlotImplicitEdge_PlainResource` — plain op.Resource with origin creates edge

**`pkg/op/graph_test.go`** (new tests):
- `TestNewGraph_InitializesResources` — Resources and Namespace non-nil
- `TestGraph_ResourcesNotSerialized` — JSON excludes resource fields

**`pkg/op/graph_test.go`** (updated):
- Removed `BackedUp` assertions from `TestGraph_ComputeSummary`
- Removed `backup` annotation from test fixtures
- Updated `TestSummary_String_Writ` to exclude `backed_up`

**`internal/writ/graph_test.go`** (updated):
- Removed `BackedUp` assertion from `TestComputeSummary`
- Removed "with backups" subtest from `TestSummaryString`
- Updated `TestNodeAnnotations` to use non-backup annotation
- Removed backup annotation from test node fixture

## Files

| File | Action | Purpose |
|------|--------|---------|
| `docs/plans/resource-management/phase-2.md` | Create | This document |
| `pkg/op/resource.go` | Modify | Add `extractResource` helper, add `reflect` import |
| `pkg/op/graph.go` | Modify | Add Resources/Namespace fields, init in NewGraph, remove BackedUp |
| `pkg/op/output.go` | Modify | Resource-aware implicit edges in FillSlot |
| `pkg/op/resource_test.go` | Modify | Add extractResource tests |
| `pkg/op/output_test.go` | Modify | Add implicit edge tests |
| `pkg/op/graph_test.go` | Modify | Add resource field tests, remove BackedUp tests |
| `internal/writ/graph_test.go` | Modify | Remove BackedUp references |

## What This Does NOT Touch

- `provider/file/resource.go` — continues using `fmt.Sprintf` for URIs
- `internal/execution/` — executor changes are Phase 4
- No generated code changes
- No codegen tool changes
