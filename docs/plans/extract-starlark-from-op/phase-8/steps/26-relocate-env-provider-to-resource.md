---
step: 26
title: "Relocate RuntimeEnvironment from the Provider surface to the Resource surface"
status: not-started — design settled 2026-06-18; gated on step 24
proof_run: n/a (not started)
parent: ../../phase-8.md
---

# Step 26 — Relocate `RuntimeEnvironment` from providers to resources

**Status:** `not-started`. Design settled (2026-06-18). Gated on [step 24](24-activation-record-first-invariant.md) (the
universal-activation invariant) for the provider half.

## What this step delivers

The `RuntimeEnvironment` is held by providers *and* resources today through one shared `ProviderBase`. Providers use it
only at dispatch (and can read `activation.RuntimeEnvironment` instead); resources use it **off** the dispatch path
(catalog/preflight/compensation/serialization), where no activation is in scope. So the env moves off providers and onto
resources:

- **Remove from the provider surface:** drop `RuntimeEnvironment() *RuntimeEnvironment` from the `op.Provider` interface
  (`pkg/op/provider.go:12-14`) and the `runtimeEnvironment` field + `RuntimeEnvironment()` accessor from `op.ProviderBase`
  (`pkg/op/provider.go:24`, `:34`).
- **Add to the resource surface:** declare `RuntimeEnvironment() *RuntimeEnvironment` on the `op.Resource` interface
  (`pkg/op/resource.go:42`) and give `op.ResourceBase` its own `runtimeEnvironment` field + accessor, no longer sourced
  from the embedded `ProviderBase`.

The result: providers become **stateless dispatch targets**; resources keep the env they need for off-dispatch I/O.

## Why this split (evidence)

- Providers read the env only in method bodies (≈87 `p.RuntimeEnvironment()` sites); the resolver INVARIANT comment
  states "no provider reads a variable at construction… every read happens at dispatch." So providers can read
  `activation.RuntimeEnvironment` once every method carries an activation (step 24).
- Resources read the env in activation-free methods called off-dispatch: `Digest` (`file/resource.go:241`→`:243`),
  `Etag` (`:306`), `Exists` (`:338`), `IsDir` (`:352`), `Resolve` (`:438`) all read `r.RuntimeEnvironment().Root`; and the
  **fixed-signature marshalers** rehydrate via `DiscoverResource(r.RuntimeEnvironment(), …)` (`:478`, `:503`).
  `UnmarshalJSON([]byte)` cannot take an env parameter — so keeping the env on resources is what **avoids** the
  marshaler-rehydration re-architecture.
- `ResourceBase` embeds `ProviderBase` *solely* for the env (`pkg/op/resource.go:79`). After this step `ProviderBase`
  loses its only data field, and `ResourceBase` carries its own.

## Change-set

1. **Provider surface:** remove `RuntimeEnvironment()` from the `op.Provider` interface; remove the `runtimeEnvironment`
   field + `RuntimeEnvironment()` accessor from `op.ProviderBase` (it keeps `providerBase()` as the interface marker).
2. **Provider method bodies (≈87 sites):** `p.RuntimeEnvironment()` → `activation.RuntimeEnvironment`. **Gated on step
   24** — methods that take no activation today (file.Remove/RemoveAll/Unlink/WalkTree, getters/pure-utils, every
   no-activation provider) have nothing to read from until the invariant lands.
3. **Resource surface:** add `RuntimeEnvironment()` to the `op.Resource` interface; give `op.ResourceBase` its own
   `runtimeEnvironment` field + accessor, set by the resource constructors (`NewResource`/`DiscoverResource`). Resource
   method bodies (`Resolve`/`Digest`/`Etag`/`Exists`/`IsDir`/marshalers) are unchanged — they keep reading the held env,
   now from `ResourceBase`'s own field.

## To settle at implementation

1. **Resource-is-a-Provider coupling.** `op.Resource` currently embeds `op.Provider` (interface) and `ResourceBase`
   embeds `ProviderBase` (struct). Once the env is gone from `ProviderBase`, decide whether `Resource` should stop
   embedding `Provider` entirely (and `ResourceBase` stop embedding the now-fieldless `ProviderBase`), or retain the
   embed for `providerBase()`/`resourceBase()` marker symmetry.
2. **Sequencing with step 24.** Group 2 cannot land before step 24; groups 1 and 3 (the struct/interface split) can be
   staged first behind the still-present provider accessor, then the accessor removed once method bodies migrate.

## Cost

Provider half ≈87 mechanical rewrites (rides step 24). Resource half is a small structural split (own field + interface
method). The expensive Tier-2 marshaler-rehydration problem is **avoided by design** — resources retain their env.

## Exit

`op.ProviderBase` carries no env; providers are stateless; `op.ResourceBase` carries its own env; the `op.Provider` /
`op.Resource` interfaces reflect the split; full `make test` green.
