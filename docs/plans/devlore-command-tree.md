---
title: "Devlore Command Tree Restructuring"
status: draft
created: 2026-02-13
updated: 2026-02-13
---

# Plan: Devlore Command Tree Restructuring

## Summary

Restructure the `devlore.*` command tree from a single monolithic Knowledge extension
into four extensions organized by target: knowledge, package, ops, and model. Move the
GenReceiver extension from noblefactor-ops to devlore-cli and rename it to follow the
`com.noblefactor.devlore.*` convention with `devlore.<noun>.<verb>` command naming.

## Goals

1. **Four-node command tree.** `devlore.knowledge`, `devlore.package`, `devlore.ops`,
   `devlore.model` — each classified by what the commands target.
2. **Correct extension placement.** All devlore extensions live in
   `devlore-cli/star/extensions/`, not noblefactor-ops.
3. **Noun/verb naming.** Every command follows `devlore.<noun>.<verb>` — nouns are
   the target domain, verbs are the action.
4. **No functional changes.** The Starlark implementations (`.star` files) are moved
   and renamed but not rewritten. Flags and behavior stay identical.

## Current State

### com.noblefactor.devlore.Knowledge (devlore-cli)

One extension containing 8 commands that span four concerns:

| Command | Target | Proposed Home |
|---|---|---|
| `devlore.knowledge.extract` | Knowledge artifacts | knowledge |
| `devlore.knowledge.build.modelfile` | Ollama config | model |
| `devlore.knowledge.index.domains` | Knowledge artifacts | knowledge |
| `devlore.knowledge.index.packages` | Lore packages | package |
| `devlore.knowledge.sign.index` | Knowledge artifacts | knowledge |
| `devlore.knowledge.sign.package` | Lore packages | package |
| `devlore.knowledge.validate` | Knowledge + packages | Split: knowledge + package |
| `devlore.knowledge.api` | Ops API surface | Split: ops.validate + ops.generate docs |

### com.noblefactor.star.GenReceiver (noblefactor-ops)

One extension in the wrong repo with the wrong naming convention:

| Command | Target | Proposed Home |
|---|---|---|
| `gen.receiver` | Graph ops + planned receivers | ops |

## Target Command Tree

```
devlore
├── knowledge
│   ├── extract          — extract knowledge artifacts from source (--domain)
│   ├── index            — generate domain indexes
│   ├── sign             — sign knowledge index
│   └── validate         — validate knowledge artifacts
├── package
│   ├── index            — generate package indexes + cross-references
│   ├── sign             — sign a lore package
│   └── validate         — validate lore packages
├── ops
│   ├── generate         — generate graph operations + planned receivers + docs
│   └── validate         — validate API contract (Attr vs StringDict, etc.)
└── model
    └── build            — generate Ollama Modelfile from knowledge domain
```

## Command Mapping

| Current | New | Notes |
|---|---|---|
| `devlore.knowledge.extract` | `devlore.knowledge.extract` | Add `--domain ops` for mapping artifact |
| `devlore.knowledge.index.domains` | `devlore.knowledge.index` | Shortened — only one index verb for knowledge |
| `devlore.knowledge.index.packages` | `devlore.package.index` | Moved to package node |
| `devlore.knowledge.sign.index` | `devlore.knowledge.sign` | Shortened — only one sign verb for knowledge |
| `devlore.knowledge.sign.package` | `devlore.package.sign` | Moved to package node |
| `devlore.knowledge.validate` | `devlore.knowledge.validate` + `devlore.package.validate` | Split by target |
| `devlore.knowledge.api` (contract check) | `devlore.ops.validate` | Contract validation is ops concern |
| `devlore.knowledge.api` (doc generation) | `devlore.ops.generate` | Docs generated alongside code |
| `devlore.knowledge.build.modelfile` | `devlore.model.build` | Moved to model node |
| `gen.receiver` (noblefactor-ops) | `devlore.ops.generate` | Moved to devlore-cli, renamed |

## Implementation Phases

### Phase 0: Delete Dispatch Interfaces

Delete `Direct`, `Writer`, `Transform` from `internal/execution/operation.go`.
Delete `Executable`. Replace with a single `Operation` interface:

```go
type Operation interface {
    Name() string
    Execute(ctx *Context, node *Node) error
}
```

Update the executor to call `op.Execute(ctx, node)` uniformly — no type-switch.
Update all 31 existing ops to implement the single interface. Each op handles
its own content sourcing internally.

- [ ] Delete `Direct`, `Writer`, `Transform`, `Executable` interfaces from `operation.go`
- [ ] Add `Execute(ctx *Context, node *Node) error` to `Operation` interface
- [ ] Update executor dispatch — remove type-switch, call `op.Execute()` uniformly
- [ ] Update all ops in `ops.go`, `ops_package.go`, `ops_service.go` to implement single interface
- [ ] Update all tests

**Files**:

| File | Repo | Action |
|---|---|---|
| `internal/execution/operation.go` | devlore-cli | Delete Direct/Writer/Transform/Executable, single Operation |
| `internal/execution/executor.go` | devlore-cli | Remove type-switch dispatch |
| `internal/execution/ops.go` | devlore-cli | All ops implement single Execute |
| `internal/execution/ops_package.go` | devlore-cli | All ops implement single Execute |
| `internal/execution/ops_service.go` | devlore-cli | All ops implement single Execute |
| `internal/execution/graph.go` | devlore-cli | Remove Executable interface |
| `internal/execution/*_test.go` | devlore-cli | Update tests |

### Phase 1: Create Four Extensions

Create the extension directories and `extension.yaml` files. Move `.star`
implementations from the monolithic Knowledge extension into the correct
extension directories.

- [ ] Create `com.noblefactor.devlore.Knowledge` (slimmed — 4 commands)
- [ ] Create `com.noblefactor.devlore.Package` (3 commands)
- [ ] Create `com.noblefactor.devlore.Actions` (2 commands)
- [ ] Create `com.noblefactor.devlore.Model` (1 command)
- [ ] Move `gen-receiver.star` from noblefactor-ops to devlore-cli
- [ ] Split `validate.star` into knowledge and package variants
- [ ] Split `api.star` into ops.validate (contract) and merge doc generation into ops.generate

**Files**:

| File | Repo | Action |
|---|---|---|
| `star/extensions/com.noblefactor.devlore.Knowledge/extension.yaml` | devlore-cli | Modify (remove moved commands) |
| `star/extensions/com.noblefactor.devlore.Knowledge/commands/build-modelfile.star` | devlore-cli | Delete (moves to Model) |
| `star/extensions/com.noblefactor.devlore.Knowledge/commands/index-packages.star` | devlore-cli | Delete (moves to Package) |
| `star/extensions/com.noblefactor.devlore.Knowledge/commands/sign-package.star` | devlore-cli | Delete (moves to Package) |
| `star/extensions/com.noblefactor.devlore.Knowledge/commands/api.star` | devlore-cli | Delete (splits to Ops) |
| `star/extensions/com.noblefactor.devlore.Package/extension.yaml` | devlore-cli | Create |
| `star/extensions/com.noblefactor.devlore.Package/commands/index.star` | devlore-cli | Create (from index-packages.star) |
| `star/extensions/com.noblefactor.devlore.Package/commands/sign.star` | devlore-cli | Create (from sign-package.star) |
| `star/extensions/com.noblefactor.devlore.Package/commands/validate.star` | devlore-cli | Create (package validation from validate.star) |
| `star/extensions/com.noblefactor.devlore.Actions/extension.yaml` | devlore-cli | Create |
| `star/extensions/com.noblefactor.devlore.Actions/commands/generate.star` | devlore-cli | Create (from gen-receiver.star + doc generation from api.star) |
| `star/extensions/com.noblefactor.devlore.Actions/commands/validate.star` | devlore-cli | Create (contract validation from api.star) |
| `star/extensions/com.noblefactor.devlore.Model/extension.yaml` | devlore-cli | Create |
| `star/extensions/com.noblefactor.devlore.Model/commands/build.star` | devlore-cli | Create (from build-modelfile.star) |

### Phase 2: Remove GenReceiver from noblefactor-ops

Delete the extension from noblefactor-ops after it has been moved to devlore-cli.

- [ ] Delete `star/extensions/com.noblefactor.star.GenReceiver/` directory

**Files**:

| File | Repo | Action |
|---|---|---|
| `star/extensions/com.noblefactor.star.GenReceiver/extension.yaml` | noblefactor-ops | Delete |
| `star/extensions/com.noblefactor.star.GenReceiver/commands/gen-receiver.star` | noblefactor-ops | Delete |

### Phase 3: Update ops.generate for Domain Extraction

Fold the `--domain ops` mapping artifact generation into `devlore.knowledge.extract`.
This extends the existing extract command rather than adding a new command.

- [ ] Add `ops` domain to extract.star
- [ ] Wire `go.mapping()` call for ops domain
- [ ] Write mapping artifacts to knowledge root

**Files**:

| File | Repo | Action |
|---|---|---|
| `star/extensions/com.noblefactor.devlore.Knowledge/commands/extract.star` | devlore-cli | Modify (add ops domain) |

## Migration Path

1. All commands get new names. Old names stop working immediately — there are no
   legacy users.
2. GenReceiver moves repos. The `go` and `file` receivers it depends on are builtins
   in the star runtime (noblefactor-ops), so they remain available regardless of
   which repo hosts the extension.
3. The noblefactor-ops Phase 6 verification commands (`star gen.receiver ...`) become
   `star devlore ops generate ...` with equivalent flags.

## Generation Model

The generator reads hand-written **services** (`FileService`, `PackageService`,
`ServiceManagerService`) and produces all infrastructure code. Services are the
only hand-written code; everything else is generated and nuke-safe.

### What We Write

| Artifact | File | Example |
|---|---|---|
| Service | `internal/execution/file_service.go` | `FileService` |

### What the Generator Produces

| Artifact | Location | Example |
|---|---|---|
| Ops interface | `internal/execution/generated/fileops/` | `fileOps` (unexported) |
| Graph operations | `internal/execution/generated/fileops/` | `FileLinkOp`, `FileCopyOp` |
| Planned receiver | `internal/execution/generated/fileops/` | Starlark `plan.file.*` bindings |
| Execute receiver | `internal/execution/generated/fileops/` | Starlark `file.*` bindings |
| Starlark type mappings | `internal/execution/generated/fileops/` | Slot assertions, UnpackArgs |
| Registration | `internal/execution/generated/fileops/` | `Ops(impl fileOps) []Operation` |

### Single Operation Interface

```go
type Operation interface {
    Name() string
    Execute(ctx *Context, node *Node) error
}
```

No `Direct`, `Writer`, `Transform` interfaces. The content model (no content,
consumer, transformer) is baked into each generated op's `Execute()` method by
the generator, based on the service method's return signature. The executor
calls `op.Execute(ctx, node)` uniformly for every node.

### Command Flags

`devlore.ops.generate` has one optional flag:

- `--implementation <name>` — generate only for this service (default: discover
  all `*Service` structs and generate everything)

Namespace is derived from the service name (`FileService` → `file`,
`ServiceManagerService` → `service_manager`). Path is known
(`internal/execution/`). All methods are included by default.

## Resolved Questions

- **`--category` flag**: Removed. Namespace derived from service name.
- **7 flags reduced to 1.** `--implementation` is optional. Without it, the
  generator discovers all `*Service` structs and generates everything.
- **`validate.star` split**: Two standalone implementations, no `--type` flag.
  `devlore.knowledge.validate` validates knowledge artifacts (domains, indexes).
  `devlore.package.validate` validates lore packages (manifests, lifecycles).
  Each knows its own schema set.
- **No dispatch interfaces.** `Direct`, `Writer`, `Transform` are deleted.
  One `Operation` interface with `Name()` and `Execute()`. Each generated op
  is self-contained — the generator bakes the content model into `Execute()`.
- **Service naming.** Hand-written structs named `*Service` (`FileService`,
  `PackageService`, `ServiceManagerService`). Methods are activities — currently
  forward-only, will expand to forward + backward (compensation).

## Related Documents

- [phase-6.md](star-gen-receiver/phase-6.md) — Typed slots and full generation
- [star-gen-receiver.md](star-gen-receiver.md) — Overall generator plan
- GitHub issue #71 — `star tree` builtin command
