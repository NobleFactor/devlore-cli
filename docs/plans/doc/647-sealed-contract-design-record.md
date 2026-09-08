---
title: "The design record states the sealed contract"
issue: https://github.com/NobleFactor/devlore-cli/issues/647
status: draft
created: 2026-09-08
updated: 2026-09-08
---

# Plan: The design record states the sealed contract

## Summary

This plan brings the design record up to the sealed tree, and changes no code. Nine providers seal their resource
types and the announcement enforces the shape (#625 phases 1 through 9, #646), but six per-provider documents still
name a resource as a pointer-to-struct, none of them says the type is an interface, the resource-management
document's summary describes provider resources as types that "embed the base and add domain fields", and the cost
of the seal is recorded in one document only. This plan says the shape once where a provider is described, says what
the seal buys where the model's guarantees are stated, records the cost beside them, and updates the status files.

## Issue 647

Phase 10 of the sealed-provider-resources plan ([#625](https://github.com/NobleFactor/devlore-cli/issues/625),
[625-sealed-provider-resources.md](../feature/625-sealed-provider-resources.md)), the feature's closure. Its four
items are this plan's four requirements. Closed by this plan's pull request.

## Goals

- [ ] No per-provider document describes a sealed resource as a struct or spells it as a pointer.
- [ ] Each of the nine providers with a sealed resource records the shape, in one line, in the same form.
- [ ] `4-resource-management.md` says what the seal buys, in the sections that state the guarantees.
- [ ] The accepted `%T` cost is recorded where a reader meets the resource model, not only in the registration doc.
- [ ] Every touched document's status file records the pass.

## Current State

Measured 2026-09-08 in the worktree, against develop at b6a3d60c.

| Component | Status | Notes |
| --- | --- | --- |
| Sealed resource types | ✅ twelve, nine providers | `appnet`, `file` (four), `function`, `git`, `json`, `mem`, `pkg`, `service`, `yaml` |
| `3.5.4-file-provider.md` | ✅ states the shape | §1's taxonomy table names the sealed interface, the unexported base, and the seal -- the model for the others |
| `3.5.5-json`, `3.5.6-yaml`, `3.5.14-function` | ❌ pointer prose | `*json.Resource`, `*yaml.Resource`, `*function.Resource`; no mention of an interface or a seal |
| `3.5.10-git`, `3.5.11-service`, `3.5.12-appnet` | ❌ silent | no occurrence of "sealed" or "interface" at all |
| `mem`, `pkg` | ⚠️ no 3.5.x document | `mem` is described by `4.2-mem-resource.md`; `pkg` has no design document of its own (a gap this plan records, not fills) |
| `3.5.1-archive`, `3.5.13-encryption` | ✅ correct as written | `*file.Regular` names a variant's implementation pointer in a Go signature, which is what a signature holds |
| `4-resource-management.md` §2 | ❌ stale | "Provider resource types embed the base and add domain fields" describes the pre-seal shape |
| `4-resource-management.md` §9 item 1 | ⚠️ partial | records `op.Resource`'s own seal, not the per-provider seal the feature added |
| The `%T` cost | ⚠️ one place | `4.3-resource-registration.md` §5 only |

## Requirements

### Requirement 1: A provider's document names its resource's shape, once

Each of the six documents that describe a sealed resource gains one sentence, in the same form, where the resource is
first named:

> `X.Resource` is a sealed interface: an exported interface embedding `op.Resource` and sealed by an unexported
> method, over an unexported struct. `4.3-resource-registration.md` §5 is the contract.

and the pointer spellings in prose become the interface's name. A Go signature keeps its pointer where the code has
one -- `*file.Regular` in an action's signature is what that function receives, and archive's and encryption's
documents are correct as they stand.

### Requirement 2: The resource-management document says what the seal buys

`4-resource-management.md` §2's `Resource` paragraph states the two-level shape: the framework interface sealed by
its base accessor, and each provider's own sealed interface over an unexported struct. A new resolved decision
records what that buys: the model's guarantees -- claims are true when made (§9 item 18), identity is the catalog
key, a string is a key and never a constructor (§9 item 15), the graph carries complete intent (§9 item 9) -- rest on
the compiler rather than on convention, because a resource cannot be constructed outside its provider, and the
announcement refuses any other shape (#646).

### Requirement 3: The cost is recorded beside the guarantee

The same decision records the accepted cost in its own sentence: reflection prints the implementation
(`*service.resource`) while the type id and the URI name the interface (`service.Resource`). Matching one against the
other is wrong by construction. The registration document keeps its statement; this is where a reader of the model
meets it.

### Requirement 4: Status files follow

Each touched document's status file gains a row naming this pass and its date. `3.5-provider-catalog.status.md`
records that the per-provider documents now state the shape.

## Design

**Why one sentence per provider rather than a table.** The shape is identical for all nine; a table would restate
`4.3` nine times and drift nine ways. The sentence names the shape and points at the contract, so the contract has
one home.

**Why `mem` and `pkg` are recorded, not fixed.** `mem`'s resource is described by `4.2-mem-resource.md`, which gets
the sentence. `pkg` has no design document at all; writing one is a task under the pkg provider's feature
([#836](https://github.com/NobleFactor/devlore-cli/issues/836)), filed by this plan rather than folded into a
documentation pass.

**What this plan does not touch.** No code, no tests, no generated file. The gate runs to prove that.

## Implementation Phases

### Phase 1: The per-provider documents

- [ ] `3.5.5-json`, `3.5.6-yaml`, `3.5.14-function`: pointer prose becomes the interface; the sentence added.
- [ ] `3.5.10-git`, `3.5.11-service`, `3.5.12-appnet`: the sentence added where the resource is first named.
- [ ] `4.2-mem-resource.md`: the sentence added.
- [ ] `3.5-provider-catalog.md`: its `json` and `yaml` rows name the interface.
- [ ] `3.5.4-file-provider.md`: verified as the model; no change expected.

### Phase 2: The model and the cost

- [ ] `4-resource-management.md` §2's `Resource` paragraph states the two-level shape.
- [ ] A resolved decision in §9 records what the seal buys and the accepted `%T` cost.

### Phase 3: Status files and closure

- [ ] The status file of every document touched gains its row.
- [ ] `625-sealed-provider-resources.md` phase 10 reads complete; the feature's plan closes.
- [ ] The `pkg` design-document gap filed under #836.

## Test Plan

Documentation only; the gate is what proves it changed nothing else.

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | No sealed resource is described as a pointer-to-struct in prose | grep | a document reverts |
| 2 | Each of the nine providers' documents names the shape | grep | one is missed |
| 3 | The tree is otherwise untouched | `make vet lint complexity verify-ldflags test` + `star lint shell .` | any code change slips in |
| 4 | Frontmatter stays valid | `./scripts/Test-GuideFrontmatter.sh` (CI) | a status file's header breaks |

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/architecture/3.5.5-json-provider.md` | Modify | the shape; pointer prose |
| `docs/architecture/3.5.6-yaml-provider.md` | Modify | the shape; pointer prose |
| `docs/architecture/3.5.14-function-provider.md` | Modify | the shape; pointer prose |
| `docs/architecture/3.5.10-git-provider.md` | Modify | the shape |
| `docs/architecture/3.5.11-service-provider.md` | Modify | the shape |
| `docs/architecture/3.5.12-appnet-provider.md` | Modify | the shape |
| `docs/architecture/4.2-mem-resource.md` | Modify | the shape |
| `docs/architecture/3.5-provider-catalog.md` | Modify | the json and yaml rows |
| `docs/architecture/4-resource-management.md` | Modify | §2's shape; the decision and the cost |
| `docs/architecture/*.status.md` (the above) | Modify | the pass |
| `docs/plans/feature/625-sealed-provider-resources.md` | Modify | phase 10 complete |

## Related Documents

- [#647](https://github.com/NobleFactor/devlore-cli/issues/647) -- this plan; phase 10 of #625
- [625-sealed-provider-resources.md](../feature/625-sealed-provider-resources.md) -- the feature plan
- [646-sealed-shape-enforceable.md](../feature/646-sealed-shape-enforceable.md) -- phase 9, which the record now states
- `docs/architecture/4.3-resource-registration.md` §5 -- the contract every sentence points at
