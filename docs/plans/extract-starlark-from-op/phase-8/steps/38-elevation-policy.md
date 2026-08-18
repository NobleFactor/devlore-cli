---
step: 38
issue: https://github.com/NobleFactor/devlore-cli/issues/520
former_step: 35
title: "Elevation policy — elaborate the model and find its place in config"
status: deferred — problem space framed; design resumes after the develop PR (2026-07-23)
parent: ../../phase-8.md
---

# Step 38 — Elevation policy: elaborate the model and find its place in config

**Status:** `deferred` — problem space framed; design resumes after the develop PR. The **final task of phase 8**,
deliberately separated from the configuration work: the config model is settled, elevation policy is not.

**Problem space — framed 2026-07-23.** The two-substrate framing now opens
[`6.1-privilege-elevation.md` § Problem space](../../../../architecture/6.1-privilege-elevation.md#problem-space--elevation-serves-two-substrates):
**substrate 1** is command execution (`shell` / `powershell` / `pkg` / `service`), which poses a *process-management*
question alongside elevation; **substrate 2** is network access (`appnet.Download` and any token-taking method), where
`elevation.Provider` is the token source. This reshapes the open questions below — the strategies ↔ brokers question
(3) now splits along the two substrates, and process management joins the scope.

**Set aside 2026-07-23 — design resumes after the develop PR.** The problem space is framed and committed; the design
(the answers below) is deferred until after phase 8's PR to `develop`.

## Configuration dependency (design proceeds, implementation is gated)

Elevation's config home — `elevation.ProviderConfig` as a recursive section resolved per profile — assumes the
configuration foundation in [`configuration.md`](../../../../architecture/configuration.md). That foundation is
**designed but largely unimplemented**, which is why the design can proceed now while the *implementation* waits:

- **`pkg/devconfig` carries only the flat predecessor** — `Config` is still `map[string]Section`
  (`pkg/devconfig/config.go:45`); no `ConfigBase`, no `Path()`. The recursive-tree reshape (configuration plan item 1)
  is the gate and **has not started**. No loader, no overlay, no `${…}` converter, no data-path sections.
- **`pkg/op/provider/elevator/config.go` is a freestanding stub** — its own `Config` / `EnvironmentConfig` /
  `TokenProviderConfig` (`config.go:14`, `:24`, `:47`), *not* on `devconfig`, still the environment-keyed shape that
  6.1 supersedes.
- **So the ordering is:** the config reshape + loader (configuration plan item 1 + the loader) land first; elevation's
  implementation (a later step) builds on them. The step-38 *design* has no such dependency — it designs against the
  settled config *design*.

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
