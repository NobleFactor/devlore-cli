# Action Namespaces

> **Status:** rewritten 2026-07-22 (phase-8 step 51, slice 5). The pre-`op` how-to — `internal/execution` paths,
> hand-written `actions_gen.go` wrappers with `Do`/`Undo`, `op.Announce` descriptors with `Register`/`NewPlanned`
> callbacks, `SetSlotImmediate` — is replaced by the landed authoring workflow: a hand-written provider +
> `make generate` (announcement, action-name constants, inventory). The provider inventory itself is owned by
> [3.5-provider-catalog.md](3.5-provider-catalog.md) and is no longer duplicated here. Companion:
> [`3-operation-namespaces.status.md`](3-operation-namespaces.status.md).

This document describes how to add a new action namespace to the execution engine.

See also: [2-execution-graph.md](2-execution-graph.md) · [2.1-typed-slots.md](2.1-typed-slots.md) ·
[2.2-phase-execution.md](2.2-phase-execution.md) · [3.5-provider-catalog.md](3.5-provider-catalog.md) (the
namespace inventory) · [4.3-resource-registration.md](4.3-resource-registration.md) (the resource half)

## Architecture Overview

The engine is `pkg/op`: planners assemble a sealed `op.Graph`; `op.GraphExecutor` runs it; receipts and the trace
record it. A **namespace** is one provider package under `pkg/op/provider/<namespace>/` — the hand-written Go
struct whose methods are the business logic — plus the generated announcement that projects it onto the engine and
the Starlark surface. One provider per namespace; the namespace is the provider's identity, appearing in action
names (`file.link`), method paths (`plan.file.link(...)`), and error messages.

The current inventory — 18 action providers with roles, access zones, and per-provider design-doc links — lives in
the [provider catalog](3.5-provider-catalog.md). Star's own tool providers (`staranalysis`, `starcode`, …) live
under `cmd/star/provider/` with their own inventory, announced the same way.

## Adding a New Namespace

### Step 1 — write the provider

Create `pkg/op/provider/<namespace>/provider.go` (the style guide's provider layout, Appendix A, governs regions
and ordering):

```go
// Package docker provides container actions for the operation graph.
package docker

// Provider provides container actions.
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a docker provider bound to the given runtime environment.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider { ... }

// Pull pulls a container image.
func (p *Provider) Pull(activationRecord *op.ActivationRecord, image string) (*Resource, *Receipt, error) { ... }

// CompensatePull removes the pulled image recorded by the receipt.
func (p *Provider) CompensatePull(activationRecord *op.ActivationRecord, receipt *Receipt) error { ... }
```

The rules the framework enforces:

1. **`op.ProviderBase` embedded**; construction takes the `*op.RuntimeEnvironment` (environment access is
   `p.RuntimeEnvironment()` — root, catalog, platform, recovery site, narrator).
2. **Activation-first**: every provider method's first parameter is the `*op.ActivationRecord` (the step-27 floor;
   compensating actions included).
3. **Namespace placement** derives from each method's classification ([3.6](3.6-method-classification.md)) —
   planned methods contribute graph nodes; immediate methods execute at script evaluation.
4. **Compensable methods return their receipt** and pair with `Compensate<Name>` — signatures validated at
   registration; the receipt names its own compensating action ([2.2](2.2-phase-execution.md)). Predicates,
   queries, and pure transforms are fallible or pure — no pair.
5. Resource types, if the namespace has them, follow [4.3](4.3-resource-registration.md) (embed
   `op.ResourceBase`, write the environment-aware constructor).

### Step 2 — `make generate`

Codegen (the `star` binary, pinned to a Last-Known-Good snapshot so a broken tree cannot strand the gen files)
reads the provider and emits, per package:

| Artifact | Content |
|---|---|
| `gen/provider.gen.go` | the `init()` announcement: `op.AnnounceProvider(reflect.TypeFor[Provider](), role, constructor, methodMetadata)` — roles (`RoleAction` / `RoleModule` / both), the constructor, and per-method Starlark parameter names with optionality and defaults (`chmod?={{ umask 0o666 }}`) |
| `action_names.gen.go` | the typed `op.ActionName` constants (`docker.Pull op.ActionName = "docker.pull"`) — compiler-checked action names (phase-8 step 32) |
| `gen/<resource>.gen.go` | one announcement per resource type ([4.3](4.3-resource-registration.md)) |

`make generate` also runs `New-OpInventory`, which regenerates `pkg/op/inventory/inventory.gen.go` — the
blank-import roster that links every announced provider into the binaries (and `cmd/star/inventory/` for star's
own). Adding the package is automatic; there is no hand-maintained registration list.

### Step 3 — test

1. **Provider unit tests** in the package — direct method calls with a test environment; compensation round-trips
   (`TestX_...`, `TestCompensateX_...`).
2. **A devlore-test fixture** (`cmd/devlore-test/devloretest/data/test_<namespace>*.star`) driving the announced
   surface end to end through `plan.assemble_definition` / `plan.run`.
3. For engine-level tests that need a bespoke action, announce a test provider directly —
   `op.AnnounceProvider(...)` in the test file, then `op.ReceiverRegistry().BuildAction("gate.wait")` +
   `WithAction` (the pattern `pkg/op/server`'s integration test uses).

### Step 4 — document

A catalog row in [3.5](3.5-provider-catalog.md) and a `3.5.x` design doc + status companion on the established
pattern (thesis, API surface, both-doors usage, the greped per-method test matrix — the step-39 bar).

## Provider Method Contracts

| Contract | Signature (after the activation) | Expectation |
|----------|----------------------------------|-------------|
| **Compensable** | `(...) (T, *Receipt, error)` | a `Compensate<Name>` companion taking the receipt |
| **Fallible** | `(...) (T, error)` | predicates, queries, reads |
| **Pure** | `(...) T` | path algebra and the like |

### Return value: the object of the action

The first return (T) is the **object** of the action — the thing acted upon, answering "to whom or what?":

| Method | Returns | Object |
|--------|---------|--------|
| `file.Link` | the `*SymbolicLink` | the symlink created |
| `file.WriteText` | the `*Regular` | the file written |
| `pkg.Install` | the package resource | the packages installed |
| `service.Start` | the service resource | the service started |
| `git.Clone` | the repository resource | the directory cloned into |
| `file.Exists` | `bool` | whether the path exists |

Return the resource acted upon, not derived or summary values (checksums live on resources and receipts, not in
return positions).

## The Starlark Surface

Resource operations project as sub-namespaces of the plan provider's three-tier surface
([3.5.3](3.5.3-plan-provider.md)): `plan.<namespace>.<method>(...)` for planned access, `<namespace>.<method>(...)`
for immediate. A **root-placed** provider (`+devlore:root=true` — flow is the one today) has its methods promoted
flat: `plan.subgraph(...)`, `plan.gather(...)`. Combinator-shaped actions supply an `op.Planner` so plan-time
construction (bodies, cases, branch topology) is theirs ([3.5.2](3.5.2-flow-provider.md)).

## Naming Conventions

| Layer | Convention | Example |
|-------|------------|---------|
| Action name | `<namespace>.<snake_method>` | `docker.pull`, `file.write_text` |
| Typed constant | `<namespace>.<GoName>` in `action_names.gen.go` | `docker.Pull`, `file.WriteText` |
| Provider method | `<GoName>` | `Provider.Pull` |
| Compensating action | `Compensate<GoName>` → `<namespace>.compensate_<snake>` | `file.compensate_file_mutation` |
| Starlark call | `plan.<namespace>.<snake_method>(...)` | `plan.docker.pull(...)` |

snake_case on every serialized/Starlark surface; camelCase in Go.

## Checklist

- [ ] `pkg/op/provider/<namespace>/provider.go` — ProviderBase, access directive, activation-first methods,
      `Compensate<Name>` pairs with receipts
- [ ] resource types per [4.3](4.3-resource-registration.md), if any
- [ ] `make generate` — announcement + action-name constants + inventory (verify `gen/provider.gen.go` metadata)
- [ ] unit tests + a devlore-test fixture
- [ ] catalog row + `3.5.x` design doc with the greped test matrix
