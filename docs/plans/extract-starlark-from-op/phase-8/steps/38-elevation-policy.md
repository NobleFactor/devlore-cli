---
step: 38
former_step: 35
title: "Elevation policy — elaborate the model and find its place in config"
status: not-started — design/research (added 2026-07-02)
parent: ../../phase-8.md
---

# Step 38 — Elevation policy: elaborate the model and find its place in config

**Status:** `not-started` — design/research. The **final task of phase 8**, deliberately separated from the
configuration work: the config model is settled, elevation policy is not.

## Why this is its own task

The configuration model is settled ([`configuration.md`](../../../../architecture/configuration.md)): a recursive
`Config`/`ConfigBase` + `Section`/`SectionBase` tree, resolved per profile over `base` / `profiles` / `applications`,
with provider config sections named `<Type>Config`. The **broker** model is settled too
([`3.2`](../../../../architecture/3.2-projected-provider-api.md#pluggable-brokers--provider-owned-routers)): a
provider-owned router over interface-conformant brokers — no global registry, no self-announcement.

The elevation **mechanism** in [`6.1-privilege-elevation.md`](../../../../architecture/6.1-privilege-elevation.md)
predates both and conflicts with them; its mechanism sections are banner-flagged **"superseded — under research
(step 38)"**. This task does the research and reconciles them.

## Open questions to resolve

1. **Offer ↔ router.** How does an abstract *offer* (the graph's `offer_reference_id`) map onto the provider-owned
   broker router? Does an offer name a broker (the router routes by offer)? Where does the offer→broker mapping live —
   an `Offers` map in `elevation.ProviderConfig` alongside `Brokers`, or is an offer just a broker name?
2. **Orchestrator vs router.** `6.1`'s orchestrator (`RegisterDriver` / `InitializeFromConfig` / `RequestElevation`)
   and its self-registering `TokenProvider` drivers are the self-registration / global-registry model that the broker
   design **rejected**. Reconcile: the provider builds a router; brokers are interface-conformant types it allocates
   from config. These two designs cannot both stand.
3. **Strategies ↔ brokers.** How do `ProcessSpawn` (sudo / runas / UAC) and `IdentityAssumption` (token minting) map
   onto brokers? Is a `TokenProvider` driver (`aws_sts`, `vault`) a leaf broker? Is `ProcessSpawn` a broker at all, or
   a separate mechanism? Is the router per-strategy?
4. **Per-profile resolution.** The flat "resolved per environment" shape becomes the recursive
   `elevation.ProviderConfig` resolved per profile via `base` / `profiles` / `applications` — the only purely-mechanical
   part; it folds in once 1–3 settle.

## Deliverable

A reconciled elevation design that (a) fits the provider-owned-router + recursive-config models, (b) gives elevation a
concrete config home (the `elevation.ProviderConfig` shape, including how offers are carried), and (c) replaces `6.1`'s
superseded mechanism sections and lifts its "under research" banner.
