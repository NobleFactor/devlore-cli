---
title: "Sealed provider resources: every <provider>.Resource is an interface"
issue: https://github.com/NobleFactor/devlore-cli/issues/625
status: approved
created: 2026-08-23
updated: 2026-08-23
---

# Plan: Sealed provider resources — every `<provider>.Resource` is an interface

## Summary

Make the shape the file provider now has the shape every provider has: **`<provider>.Resource` is a
sealed interface**, implemented by an unexported struct **`<provider>.resource`** that carries the
identity and serializes. One rule, no exceptions: *every provider exposes `Resource`, and only that
provider can produce one.*

**This is a correctness change. Consistency is its dividend, not its motive** — and the dividend is
worth naming, because it compounds: one rule with no exceptions is easier to teach, easier to learn, and
therefore easier to onboard into and cheaper to maintain for as long as the codebase lives. A rule with
one exception is two rules, and the exception is what every newcomer trips over.

The correctness half. An exported struct resource is hand-buildable — `&git.Resource{SourcePath: …}`
compiles in any package — producing an un-interned, unclaimed resource that can be handed straight into
a dispatch. Every guarantee the resource model makes assumes resources come from the catalog:

- *claims are true when made* — a hand-built resource was never verified;
- *identity is the catalog key* — a hand-built resource holds a URI the ledger never issued;
- *a string is a key, never a constructor* — the dispatch seam refuses run-time construction, then
  accepts a struct literal that did the same thing at compile time;
- *the graph carries complete intent* — an unclaimed resource is intent the document never recorded.

Eight providers leave that door open beside those guarantees. A sealed interface closes it: the
unexported marker method means only the declaring package can implement it, and the unexported struct
means no one outside can build one. The resource model stops depending on everyone's good manners and
starts depending on the compiler.

## The rulings this plan implements (USER, 2026-08-23)

1. **Every `<provider>.Resource` is an interface** — sealed by an unexported marker method, so
   implementations cannot be added from outside the package.
2. **The underlying struct is unexported (`<provider>.resource`) and is what serializes** — the identity
   payload and the type id in the URI fragment name the concrete struct, as they do today; only its
   visibility changes.
3. **Naming is uniform**: the interface takes the provider's headline name (`Resource`), the base takes
   its lowercase form (`resource`) — the rule already applied to `file` (`Resource`/`resource`).

## Prior art in the tree

The `file` provider arrived here first, for a different reason — it has a kind axis, so its resource had
to be an interface over `Regular` / `Directory` / `SymbolicLink` / `Any`. Feature
[#616](https://github.com/NobleFactor/devlore-cli/issues/616) renamed that interface to `Resource` and
its base to `resource` precisely so file would stop being the exception. This plan finishes the thought:
the other nine adopt the same shape, and the exception disappears in the other direction too.

Providers in scope (nine): `git`, `pkg`, `service`, `appnet`, `mem`, `json`, `yaml`, `function`, and any
resource-bearing provider added before this lands. `file` is already done.

## Epic and issue placement

**Epic: #444 — The resource model (`Epic:ResourceModel`).**
**Feature: [#625](https://github.com/NobleFactor/devlore-cli/issues/625)** — *Sealed provider resources:
every `<provider>.Resource` is an interface*.

| Phase | Task |
| --- | --- |
| 1 | [#626](https://github.com/NobleFactor/devlore-cli/issues/626) seal `service` — prove the pattern |
| 2 | [#627](https://github.com/NobleFactor/devlore-cli/issues/627) seal `git`, `json`, `yaml`, `appnet` |
| 3 | [#628](https://github.com/NobleFactor/devlore-cli/issues/628) seal `mem`, `function`, `pkg` |
| 4 | [#629](https://github.com/NobleFactor/devlore-cli/issues/629) make the shape structurally enforceable |
| 5 | [#630](https://github.com/NobleFactor/devlore-cli/issues/630) closure — the design record states the contract |

## The mechanic that has to be solved first

**Announcements are generated into a sibling package.** `pkg/op/provider/<p>/gen/*.gen.go` is
`package file` / `package git` etc. — checked 2026-08-23: the gen files declare the *provider's* package,
not a separate one, so they can already name unexported types. **This is the enabling fact that makes the
whole plan cheap**, and it must be verified per provider before the sweep rather than assumed from one
sample.

If any provider's gen output turns out to live in a genuinely separate package, that provider needs one
of: the announcement emitted in-package, or an exported `func ResourceType() reflect.Type` seam. Decided
per provider at implementation, not pre-emptively.

**The generator's detection rule already accommodates this**: a resource type is identified by an
exported constructor `func(*op.RuntimeEnvironment, any) (*T, error)`. An exported constructor may return
an unexported type — `func DiscoverClone(env *op.RuntimeEnvironment, v any) (*resource, error)` is legal
Go. The constructor is the public contract; the struct behind it need not be.

## Phases

### Phase 1 — the pattern, proven on one provider — status: pending

Take the provider with the smallest external footprint (`service`: zero files outside its package name
`service.Resource`) and land the full shape there: the sealed `Resource` interface, the unexported
`resource` base, the marker method, the announcement, and the serialization proof (a saved graph still
round-trips, and the URI fragment still names the concrete struct). The result is the template every
later provider follows, and the place where every surprise surfaces cheaply.

### Phase 2 — the low-footprint providers — status: pending

`git` (1 external file), `json` (1), `yaml` (1), `appnet` (2) — the same transformation, one PR each or
batched if phase 1 proves the pattern is mechanical.

### Phase 3 — the high-footprint providers — status: pending

`mem` (4), `function` (5), `pkg` (7). These have real consumers naming the type, so each carries an
authoring sweep. `function` also carries the Starlark-callable path and `mem` the content-addressed
store, so their round-trip proofs are the load-bearing ones.

### Phase 4 — the rule becomes enforceable — status: pending

1. `4.3-resource-registration.md` states the shape as the contract for adding a provider resource.
2. A test asserts it structurally: every announced resource type's Go struct is unexported, and every
   provider package exposes a `Resource` interface. A future provider that exports its struct fails the
   suite rather than a review.

### Phase 5 — closure — status: pending

Design docs and status files record the uniform shape; `3.5.x` per-provider docs drop any language that
described the resource as a struct.

## Judgment scenarios

1. **A resource cannot be forged.** Outside its package, `&git.resource{…}` does not compile and no type
   satisfies `git.Resource` — the seal holds against a hand-built value reaching dispatch. (A compile-time
   property; pinned as a documented negative plus a `go vet`-visible marker rather than a runtime test.)
2. **Serialization is unchanged.** A graph planned before the change and one planned after produce the
   same `resources` section for the same intent — the fragment names the concrete struct either way, and
   only its visibility changed. The round-trip pins stay byte-identical.
3. **Rehydration still finds the constructor.** A saved document reloads: the type id resolves through
   the registry to the exported constructor, which returns the unexported struct behind the interface.
4. **The structural rule bites.** A deliberately non-conforming fixture provider — exported resource
   struct, no interface — fails phase 4's assertion.

## Verification

Every phase: `make check`, `make vet` under GOOS windows and linux, `gofmt -l`. The Windows baseline is
zero. Serialization is the risk surface: any change to a `resources` section's bytes is a defect in the
phase that caused it, not an accepted consequence.

## Open questions

1. **Does the seal want a shared marker?** Each package declaring its own unexported `sealedResource()`
   is the file precedent and needs no framework support. A framework-side marker would centralize the
   rule but cannot be unexported *and* implementable across packages — so per-package is likely forced,
   and phase 1 confirms it.
2. **What happens to providers whose resource has exported behavioral fields consumers read?** The
   interface must expose them as methods, or those consumers change. Sized per provider in phases 2–3;
   `pkg` (7 external files) is the likely worst case.
3. **Does `op.Resource` itself want the same treatment?** It is already an interface sealed by an
   unexported base accessor — so the framework layer is where this pattern came from, and the provider
   layer is catching up rather than inventing.
