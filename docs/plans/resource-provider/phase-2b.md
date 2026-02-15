# Phase 2B: Template Provider, Manifest Provider, Graph Builder Cleanup

## Context

Phase 2A (PR pending) moved services into provider subpackages and created new
providers for previously unregistered pseudo-operations. All 28 actions are
registered and use flat names. The `Operation` enum in `writ/tree/` survives as
an indirection layer between filename parsing and graph construction.

Phase 2B eliminates that indirection, splits render into its own provider,
introduces the manifest provider for lore package planning, removes passthrough
transforms from `execution/plan.go`, and generates plan methods from providers.

**Worktree**: `devlore-cli.resource-provider`
**Branch**: `feat/provider-extraction`
**Base**: Phase 2A (same branch)

## Goals

1. **template.Provider** — Render is not a file operation. It takes input
   content and produces output content through Go template expansion. Split it
   into `provider/template/` with `template.Render`.
2. **manifest.Provider** — When the graph builder encounters
   `packages-manifest.yaml`, it creates a `manifest-resolve` node. A planning
   step resolves the manifest into a lore package lifecycle pipeline (phases,
   forward/compensate nodes) before execution begins.
3. **Delete Operation enum** — `writ/tree/operation.go` defines `OpLink`,
   `OpRender`, `OpCopy`, `OpDecrypt`, `OpPackages`. These are replaced by
   action name strings from the providers.
4. **Generated plan methods** — Plan methods in `execution/plan.go` and
   Starlark plan bindings are generated from provider action signatures. Each
   provider owns its plan methods.
5. **Remove passthrough transforms** — `Plan.Copy(source, path, "decrypt",
   "render")` is replaced by explicit node chains: `encryption.Decrypt →
   template.Render → file.Copy`.

## Provider Changes

### New: provider/template

```
provider/template/
  provider.go        — template.Provider with Render method
  actions_gen.go     — template.Render action + Register()
```

**provider.go**:
```go
type Provider struct{}

func (p *Provider) Render(templateData map[string]any, source, project string, content []byte) ([]byte, error)
```

**actions_gen.go**:
- `Render` action: reads content from `ctx.ContentFor(node)` or source file,
  expands Go templates with `ctx.Data`, stores result via `ctx.StoreContent()`.
  Name: `"render"`. Same logic currently in `file/actions_gen.go Render.Do()`.
- `Register(reg)` — registers Render.

### New: provider/manifest

```
provider/manifest/
  provider.go        — manifest.Provider with Resolve method
  actions_gen.go     — manifest.Resolve action + Register()
```

**provider.go**:
```go
type Provider struct{}

// Resolve reads a packages-manifest.yaml and returns the parsed package list.
func (p *Provider) Resolve(path string) ([]string, error)
```

The `manifest-resolve` action is a graph node. The action's `Do` method:
1. Reads the manifest file (path from "source" slot)
2. Parses package names from YAML/JSON
3. Invokes the lore package planner to build lifecycle phases
4. Stores the resulting subgraph specification for expansion

**Graph lifecycle**: build → **plan** → execute. The planning step walks the
graph, finds `manifest-resolve` nodes, and expands each into a lore package
lifecycle pipeline. The executor runs the expanded graph.

**actions_gen.go**:
- `Resolve` action: Name: `"manifest-resolve"`. `Do()` reads manifest,
  produces pipeline specification. `Undo()` is a no-op.
- `Register(reg)` — registers Resolve.

### Modified: provider/file

Remove `Render` from file provider. File provider drops from 10 to 9 actions:
link, copy, backup, unlink, remove, write, move, mkdir, source.

## Tree Package Changes

### Delete operation.go

Delete `internal/writ/tree/operation.go`. The `Operation` type, constants
(`OpLink`, `OpRender`, `OpCopy`, `OpDecrypt`, `OpPackages`), `Operations`
slice type, `HasCopy()`, `HasPackages()`, and `String()` are all removed.

### Update node.go — ProcessingPipeline

`ProcessingPipeline` returns `[]string` (action names) instead of `Operations`:

```go
func ProcessingPipeline(filename string) (targetName string, actions []string) {
    name := filename
    baseName := filepath.Base(name)

    // packages-manifest → manifest-resolve
    for _, pf := range PackagesManifestFiles {
        if baseName == pf {
            return name, []string{"manifest-resolve"}
        }
    }

    var pipeline []string

    if strings.HasSuffix(name, ".age") || strings.HasSuffix(name, ".sops") {
        name = strings.TrimSuffix(name, ".age")
        name = strings.TrimSuffix(name, ".sops")
        pipeline = append(pipeline, "decrypt")
    }

    if strings.HasSuffix(name, ".template") {
        name = strings.TrimSuffix(name, ".template")
        pipeline = append(pipeline, "render")
    }

    if len(pipeline) > 0 {
        pipeline = append(pipeline, "copy")
    } else {
        pipeline = []string{"link"}
    }

    return name, pipeline
}
```

### Update builder.go

Replace `Operations` references with `[]string`. Replace `HasCopy()` and
`HasPackages()` with direct string checks:

```go
func hasAction(actions []string, name string) bool {
    for _, a := range actions {
        if a == name { return true }
    }
    return false
}
```

### Update tree_test.go

All `Operations{OpLink}` → `[]string{"link"}`, etc.

## Graph Builder Changes

The graph builder (`writ/graph_builder.go` `BuildTree`) already constructs
nodes from the action name strings returned by the tree builder. The only
change is that `packages-manifest.yaml` files now produce a `manifest-resolve`
node instead of a `packages` node:

```go
node := &execution.Node{
    ID:     f.ID,
    Action: actions[0],  // "manifest-resolve"
    // ...
}
node.SetSlotImmediate("source", f.Source)
```

No structural change to the graph builder — it already reads action names
from the tree result.

## Plan Method Changes

### execution/plan.go — Remove passthrough transforms

Delete the `transforms ...string` variadic from `Plan.Copy()` and
`Plan.CopyWithMode()`. The chain-building logic moves to callers who
explicitly create decrypt/render nodes and wire edges.

Before:
```go
plan.Copy(source, path, "decrypt", "render")
```

After:
```go
d := plan.Decrypt(source)
r := plan.Render("")  // reads from upstream
c := plan.Copy("", path)
plan.DependsOn(d, r)
plan.DependsOn(r, c)
```

### execution/plan.go — Add missing methods

Add `Plan.Render()` and `Plan.Decrypt()` methods that create single nodes.
These replace the implicit transform chain.

Delete `Plan.Validate()` — already done in Phase 2A.

### Generated plan methods (future)

The plan methods follow the same pattern as actions: they read slots and create
nodes. The `plan_receiver` template from `star gen.receiver` can generate both
the Go plan methods and the Starlark plan bindings.

In this phase, the plan methods remain hand-written but follow the exact
patterns the generator produces. The `_gen.go` files will be nuke-safe after
Phase 3 updates the templates.

## Starlark Plan Changes

### plan_root.go — Add template and encryption namespaces

```go
case "template":
    return p.templatePlan, nil
case "encryption":
    return p.encryptionPlan, nil
```

The `plan.file.configure(source, path)` method (which creates render→copy
chains) moves to use explicit template and file actions:

Before: `plan.file.configure(source, path)` → render node + copy node + edge
After: `plan.template.render(source)` → `plan.file.copy(rendered, path)`

### plan_file.go — Remove configure method

The `configure` method created an implicit render→copy chain. Callers use
`plan.template.render()` and `plan.file.copy()` explicitly.

### New: Starlark template plan

`TemplatePlan` struct with `render` method. Same pattern as `FilePlan`.

### New: Starlark encryption plan

`EncryptionPlan` struct with `decrypt` method. Currently decrypt is not
exposed as a Starlark binding — it's wired implicitly by the graph builder.
Making it explicit allows plan authors to chain it.

## RegisterAll Update

`provider/register_gen.go` adds template and manifest:

```go
func RegisterAll(reg *execution.ActionRegistry) {
    file.Register(reg)
    encryption.Register(reg)
    template.Register(reg)
    pkg.Register(reg)
    shell.Register(reg)
    service.Register(reg)
    content.Register(reg)
    net.Register(reg)
    archive.Register(reg)
    git.Register(reg)
    manifest.Register(reg)
}
```

Action count: 28 → 28 (render moves from file to template; manifest-resolve
adds 1; file loses render = net zero for file, +2 total = 30... let me count).

**Revised count**: file(9) + encryption(1) + template(1) + pkg(4) + shell(2)
+ service(5) + content(1) + net(1) + archive(1) + git(3) + manifest(1) = **29**.

## ComputeSummary and Preflight

Update `ComputeSummary` switch cases:
- `"render"` → counts as template (was counting as template already via the
  render name)
- `"manifest-resolve"` → counts as package specification

Update `Preflight`: manifest-resolve nodes have no filesystem conflict (they
produce sub-nodes, not files).

## Steps

### 1. Create provider/template (move render from file)

- Create `provider/template/provider.go` — `Provider` with `Render()` method
- Create `provider/template/actions_gen.go` — `Render` action + `Register()`
- Remove `Render` from `provider/file/provider.go` and
  `provider/file/actions_gen.go`
- Update file `Register()` to no longer register Render

### 2. Create provider/manifest

- Create `provider/manifest/provider.go` — `Provider` with `Resolve()` method
- Create `provider/manifest/actions_gen.go` — `Resolve` action + `Register()`
- The `Do()` implementation reads the manifest and stores the package list.
  Full pipeline expansion is wired in the planning step (Step 6).

### 3. Delete writ/tree/operation.go, update tree package

- `git rm internal/writ/tree/operation.go`
- Update `node.go` — `ProcessingPipeline` returns `(string, []string)`
- Update `builder.go` — replace `Operations` with `[]string`, replace
  `HasCopy()`/`HasPackages()` with `hasAction()`
- Update `output.go` — `Operations` references
- Update `tree_test.go` — all operation constant references

### 4. Update execution/plan.go

- Remove `transforms ...string` from `Copy()` and `CopyWithMode()`
- Add `Plan.Render(source)` and `Plan.Decrypt(source)` methods
- These create single nodes; chaining is explicit via `Plan.DependsOn()`

### 5. Update Starlark plan bindings

- Add `TemplatePlan` struct with `render` method
- Add `EncryptionPlan` struct with `decrypt` method
- Add `template` and `encryption` to `PlanRoot.Attr()`
- Remove `configure` from `FilePlan`
- Update `planBindings.Configure()` → delete (callers use explicit chains)

### 6. Wire manifest planning step

- Add `ResolvManifests(graph)` function that walks the graph, finds
  `manifest-resolve` nodes, and expands each into a lore package lifecycle
  pipeline using the existing `lore.Build()` infrastructure
- Call from graph builder after `BuildTree()` and before execution

### 7. Update RegisterAll

- Add `template.Register(reg)` and `manifest.Register(reg)`
- Update test count: 28 → 29

### 8. Update ComputeSummary and Preflight

- Add `"manifest-resolve"` case to `ComputeSummary`
- Add `"manifest-resolve"` to `Preflight` skip list

### 9. Update tests

- `execution_test.go` — update action count, add template/manifest tests
- `writ/tree/tree_test.go` — replace `Operations{}` with `[]string{}`
- `writ/graph_test.go` — update action names if changed
- `starlark/receiver_test.go` — add template/encryption plan tests

### 10. Build and test

```bash
go build ./...
go vet ./...
go test ./internal/execution/... -count=1
go test ./internal/starlark/ -count=1
go test ./internal/writ/... -count=1
go test ./internal/lore/... -count=1
```

## Order of Operations

1. **Steps 1-2**: Create new providers (template, manifest)
2. **Step 3**: Delete Operation enum, update tree package
3. **Steps 4-5**: Update plan methods and Starlark bindings
4. **Step 6**: Wire manifest planning step
5. **Steps 7-8**: Update RegisterAll, ComputeSummary, Preflight
6. **Steps 9-10**: Tests, verify

## What This PR Does NOT Touch

- No dotted names — action names remain flat
- No flow package (Choose/Gather/Elevate) — separate PR
- No engine/build subpackage restructuring
- No code generation — `_gen.go` files are hand-written (Phase 3 makes them
  regenerable)
- Platform plan bindings stay hand-written (darwin.go, linux.go, windows.go)

## Critical Files

| File | Role |
|---|---|
| `provider/file/actions_gen.go` | Remove Render action |
| `provider/template/actions_gen.go` | New — template.Render |
| `provider/manifest/actions_gen.go` | New — manifest.Resolve |
| `provider/register_gen.go` | Add template + manifest |
| `writ/tree/operation.go` | Delete |
| `writ/tree/node.go` | ProcessingPipeline returns []string |
| `writ/tree/builder.go` | Replace Operations with []string |
| `execution/plan.go` | Remove transforms, add Render/Decrypt |
| `starlark/plan_file.go` | Remove configure method |
| `starlark/plan_root.go` | Add template + encryption namespaces |
| `execution/graph.go` | ComputeSummary cases |
| `execution/preflight.go` | manifest-resolve skip |
