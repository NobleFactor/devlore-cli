# Phase 4: Architecture Documentation

```yaml
title: "Phase 4: Architecture Documentation"
issue: TBD
status: complete
created: 2026-02-16
updated: 2026-02-16
```

## Context

Phases 1–3 renamed Operation → Action, restructured Services into Provider
subpackages, and updated code generation templates. The architecture docs
still referenced the old model (Operation interface, `Execute(ctx, node)`,
`node.GetSlot()`, `ContentFor`/`StoreContent`, flat package layout). Phase 4
brings the docs current.

## Scope

| Category | Files | Severity |
|---|---|---|
| Architecture docs | 5 docs | 3 CRITICAL, 2 LOW |
| Cross-references | `index.md`, `devlore-receipt-integrity.md`, `resource-provider.md` | link text |
| Code doc comment | `internal/execution/graph.go:21-22` | stale ContentFor ref |
| Plan docs | `resource-provider.md` (update), `phase-4.md` (create) | plan tracking |

## Changes

### `devlore-typed-slots.md` (CRITICAL)

- "Operation Interface" → "Action Interface"
- Old interface (`Execute(ctx, node) error`) → current Action contract (Do/Undo, three-value return)
- `node.GetSlot("key")` → `slots["key"].(type)`
- `FileLinkOp` example → `file.Link` with `Impl *Provider`, Do/Undo
- `FileDecryptOp` example → `encryption.Decrypt` with slots pattern (no ContentFor/StoreContent)
- `Ops(impl) []Operation` → `Register(reg *execution.ActionRegistry)`
- Terminology: `FileService` → `file.Provider`, `PackageService` → `pkg.Provider`
- Generated code: flat `generated/fileops/` → `provider/file/actions_gen.go`

### `devlore-phase-execution.md` (CRITICAL)

- `Operation`/`CompensableOperation` → unified Action (Do + Undo)
- `FileCopyOp.Execute()` → `file.Copy.Do()` with slots, three-value return
- `Compensate()` → `Undo(ctx, slots, state)`
- Eliminated CompensableOperation section — Action has Undo built in
- Files table updated to current layout
- Cross-ref: "Operation Namespaces" → "Action Namespaces"

### `devlore-operation-namespaces.md` (MEDIUM)

- Retitled to "Action Namespaces"
- Current namespaces: 11 providers, 31 actions, dotted names
- Docker example: `DockerPullOp` → `docker.Pull` with Do/Undo, Register
- `AllOps()` → `RegisterAll(reg)` in `provider/register_gen.go`
- Updated naming conventions, checklist

### `devlore-execution-graph.md` (LOW)

- Node struct: `Operations []string` → `Action Action` field
- YAML examples: `operations: [link]` → `action: file.link`

### `devlore-graph-convergence-operations.md` (LOW)

- NodeDefault comment: "standard operation node" → "standard action node"
- YAML: `operations: [shell]` → `action: shell.exec`
- Node struct: added Action field, removed Rollback string

### Cross-references

- `index.md`: "Operation Namespaces" → "Action Namespaces"
- `devlore-receipt-integrity.md`: same
- `resource-provider.md`: same

### Code doc comment

- `graph.go:21-22`: `Do(ctx, node)` → `Do(ctx, slots)`, ContentFor/StoreContent → Result + promise slots

### Plan docs

- `resource-provider.md`: Phase 4 moved to Completed, Phase 5 (Testing) added
- `phase-4.md`: this file

## Files

- `docs/architecture/devlore-typed-slots.md`
- `docs/architecture/devlore-phase-execution.md`
- `docs/architecture/devlore-operation-namespaces.md`
- `docs/architecture/devlore-execution-graph.md`
- `docs/architecture/devlore-graph-convergence-operations.md`
- `docs/architecture/index.md`
- `docs/architecture/devlore-receipt-integrity.md`
- `docs/plans/resource-provider.md`
- `docs/plans/resource-provider/phase-4.md` (new)
- `internal/execution/graph.go`

## Verification

```bash
# No stale Operation type references
grep -n 'type Operation interface\|CompensableOperation\|\.Execute(ctx \*Context, node' docs/architecture/*.md
# No stale content pipeline
grep -n 'ContentFor\|StoreContent' docs/architecture/*.md
# No stale slot access
grep -n 'node\.GetSlot' docs/architecture/*.md
# No old struct names
grep -n 'FileLinkOp\|FileDecryptOp\|FileCopyOp\|DockerPullOp' docs/architecture/*.md
# No old registration pattern
grep -n 'AllOps\|func.*Ops.*\[\]' docs/architecture/*.md
# Build still passes
go build ./...
```
