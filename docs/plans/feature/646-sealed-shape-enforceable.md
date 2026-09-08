---
title: "The sealed-resource shape is enforced at announcement"
issue: https://github.com/NobleFactor/devlore-cli/issues/646
status: draft
created: 2026-09-08
updated: 2026-09-08
---

# Plan: The sealed-resource shape is enforced at announcement

## Summary

This plan adds contract enforcement to the announcement of a resource, and takes nothing away.

The contract already holds: every one of the twelve announced resource types is an exported interface, sealed by an
unexported method, over an unexported struct in the provider's own package. What is missing is enforcement. Today
`AnnounceResource` refuses only an interface with no registered implementation; after this plan it also refuses a
type that fails the shape, the inventory's boot-discipline suite walks every announced type with the same check, and
the registration document states the shape as the contract for adding a resource.

Only `AnnounceResource` runs the check. `AnnounceProvider` and `AnnounceType` -- providers, and plain data types such
as the providers' result structs -- are untouched, so a plain exported struct remains a data type exactly as before.
Nothing that works today stops working; the only thing removed is the ability to announce a non-conforming type as a
resource, which nothing in the tree does.

## Issue 646

Phase 9 of the sealed-provider-resources plan ([#625](https://github.com/NobleFactor/devlore-cli/issues/625),
[625-sealed-provider-resources.md](625-sealed-provider-resources.md)): the rule becomes structurally enforceable,
so it survives the next contributor without depending on review. Closed by this plan's pull request.

## Goals

- [ ] A resource announced as a struct, as an unsealed interface, or as an interface over an exported struct is
      refused at announcement, and the refusal names the type and the rule it broke.
- [ ] Every announced resource type in the tree passes the check, asserted by a test that walks the populated
      registry.
- [ ] `4.3-resource-registration.md` states the shape as the contract for adding a provider resource; its status
      file and the feature plan record the phase.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| Twelve announced resource types | ✅ all interfaces | `file.AnyKind`, `file.Regular`, `file.Directory`, `file.SymbolicLink`; `Resource` in appnet, function, git, json, mem, pkg, service, yaml |
| Twelve `RegisterResourceImplementation` calls | ✅ | each provider's own `init`, interface → unexported struct |
| The seal | ✅ | `sealedResource()` on every interface, implemented by the unexported struct |
| `AnnounceResource` | ⚠️ tolerant | refuses an interface with no registered implementation; accepts a struct as "its own implementation" (`resourceImplementationFor`'s pre-seal branch) |
| `ResourceReceiverType` | ⚠️ | keeps the implementation (`ProviderType()`, the struct) but not the announced interface |
| Boot-discipline suite (`pkg/op/inventory/discipline_test.go`) | ✅ two walks | Addressing override; product-type resolution. No walk asserts the shape |
| `4.3-resource-registration.md` §5 | ❌ stale | tells a developer to write "the resource struct embedding `op.ResourceBase`" |

## Requirements

### Requirement 1: One check states the shape

A single predicate, `CheckSealedShape(announced, implementation reflect.Type) error`, asserts in order:

1. `announced` is an interface;
2. it embeds `op.Resource` (`announced.Implements(Resource)`);
3. it carries at least one unexported method — the seal — so no type outside the package can satisfy it;
4. `implementation` is a struct type;
5. the struct's name is unexported;
6. the struct is declared in the interface's package;
7. `*implementation` satisfies `announced`.

Each failure is its own error, naming the type and the rule. The order is the order a reader diagnoses in: a
struct announcement fails at 1 and is told to announce the interface; an interface missing its seal fails at 3.

### Requirement 2: The announcement refuses a non-conforming shape

`AnnounceResource` runs the check after resolving the implementation and asserts it, as it asserts the missing
registration today. A provider that exports its struct fails at process start, in its own `init`, naming the type
-- before any test, review, or document is consulted. The pre-seal tolerance in `resourceImplementationFor` (a
non-interface is its own implementation) stays only so the check can name what was announced; nothing passes
through it any more.

### Requirement 3: The suite states the rule over the populated registry

`ResourceReceiverType` gains `AnnouncedType() reflect.Type`. The boot-discipline suite, which runs with every
provider's gen package imported, walks every announced resource and asserts `CheckSealedShape(AnnouncedType(),
ProviderType())` returns nil. The registration enforces the rule; this walk is where the rule is READ, beside the
two discipline walks that already exist, and it is what fails if the enforcement is ever loosened.

### Requirement 4: The judgment scenario -- the rule bites

A pkg/op test announces three deliberately non-conforming fixtures and asserts each refusal by message: an
exported struct with no interface; an interface without a seal; a sealed interface over an exported struct.
Nothing reaches the registry: the check runs before `registerReceiverType`. Rule 6 (same package) has no negative
fixture -- a second test package would be needed to break it -- and is pinned by the positive walk alone; stated
here rather than implied.

### Requirement 5: The document is the contract

`4.3-resource-registration.md` §5 replaces "the resource struct" with the shape: the exported interface embedding
`op.Resource` with its `sealedResource()` seal; the unexported struct embedding `op.ResourceBase`; the
`op.RegisterResourceImplementation` call in the provider's own `init`; constructors that return the interface. §1
says the announcement refuses any other shape, and §6's sequence gains the check. The status file records it.

## Design

**Where the check lives.** `pkg/op/resource_implementation.go`, beside `RegisterResourceImplementation`: the file
that already explains why the interface and the struct cross the package boundary separately. Exported, because
the inventory suite is a different package and the rule should be callable by anything that announces.

**Why the announcement, not only the test.** A test states a rule; the seam enforces it. A provider that exports
its struct today announces successfully and fails nothing until a reviewer notices. After this plan it fails to
start, with the rule in the message. The suite keeps the assertion too, so the two agree and a loosening of either
is visible.

**Why a star lint rule is not this task.** The user's ruling stands that every linter is embedded in star; a goast
rule for this shape belongs with the lint provider's work ([#837](https://github.com/NobleFactor/devlore-cli/issues/837)).
The announcement check is not a linter: it is the registry refusing a type, at the one place every resource passes.

**What #647 owns.** The per-provider `3.5.x` documents and `4-resource-management.md`'s statement of what the seal
buys are phase 10. This plan touches `4.3` alone, because `4.3` is where the developer reads how to add a resource.

## Implementation Phases

### Phase 1: The check and the refusal

- [ ] `CheckSealedShape` in `resource_implementation.go`, seven rules, each its own error naming the type.
- [ ] `AnnounceResource` asserts it after resolving the implementation; `resourceImplementationFor`'s doc says the
      pass-through exists so the check can name a struct announcement.
- [ ] `TestAnnounceResource_ANonConformingShapeIsRefused`: the three fixtures, each refusal matched by message.
- [ ] Every existing test stays green: the twelve announcements pass the check unchanged.

**Files:** `pkg/op/resource_implementation.go`, `pkg/op/receiver_registry.go`, `pkg/op/resource_implementation_test.go` (create).

### Phase 2: The suite reads the rule

- [ ] `ResourceReceiverType.AnnouncedType()`; `resourceReceiverType` keeps the announced type.
- [ ] `TestBootDiscipline_EveryResourceTypeIsASealedInterface` in `pkg/op/inventory/discipline_test.go`.

**Files:** `pkg/op/receiver_type.go`, `pkg/op/inventory/discipline_test.go`.

### Phase 3: The document is the contract

- [ ] `4.3-resource-registration.md` §1, §5, §6 as Requirement 5 states; its status file gains the row.
- [ ] `625-sealed-provider-resources.md` phase 9 reads complete; this plan's status reads complete.

**Files:** `docs/architecture/4.3-resource-registration.md`, `docs/architecture/4.3-resource-registration.status.md`,
`docs/plans/feature/625-sealed-provider-resources.md`, this plan.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | Every announced resource type passes the check | unit, inventory | a provider exports its struct or drops its seal |
| 2 | A struct announcement is refused, naming the type | unit | the pre-seal tolerance returns |
| 3 | An unsealed interface is refused | unit | rule 3 is dropped |
| 4 | A sealed interface over an exported struct is refused | unit | rule 5 is dropped |
| 5 | Nothing non-conforming reaches the registry | unit | the check moves after registration |

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/resource_implementation.go` | Modify | `CheckSealedShape`; the pass-through's doc |
| `pkg/op/receiver_registry.go` | Modify | `AnnounceResource` asserts the check |
| `pkg/op/receiver_type.go` | Modify | `AnnouncedType()` on the resource receiver type |
| `pkg/op/resource_implementation_test.go` | Create | the three refusals |
| `pkg/op/inventory/discipline_test.go` | Modify | the walk |
| `docs/architecture/4.3-resource-registration.md` | Modify | the contract |
| `docs/architecture/4.3-resource-registration.status.md` | Modify | the row |
| `docs/plans/feature/625-sealed-provider-resources.md` | Modify | phase 9 status |

## Related Documents

- [#646](https://github.com/NobleFactor/devlore-cli/issues/646) -- this plan; phase 9 of #625
- [625-sealed-provider-resources.md](625-sealed-provider-resources.md) -- the feature plan; its judgment scenario 8 is this plan's Requirement 4
- [#647](https://github.com/NobleFactor/devlore-cli/issues/647) -- phase 10, the design record
- [#854](https://github.com/NobleFactor/devlore-cli/issues/854) -- the providers' `Unmarshal*` constructors, under the same feature
- [#837](https://github.com/NobleFactor/devlore-cli/issues/837) -- where a star lint rule for the shape would file
- `docs/architecture/4.3-resource-registration.md` -- the document this plan makes the contract
