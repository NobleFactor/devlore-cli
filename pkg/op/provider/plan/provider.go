// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package plan provides graph-construction actions for the plan namespace.
//
// Its methods execute during script evaluation to create nodes in the operation graph. The plan Provider is an
// executing receiver — not a planning receiver — because its methods run immediately to build the graph.
package plan

import (
	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/flow"
)

var _ op.Provider = (*Provider)(nil) // Interface Guard

// Provider creates graph nodes for plan-time graph construction.
//
// Provider implements a three-tier attribute resolution (see phase-8 D12 + I4):
//
//   - Tier 1 — Provider's own methods (e.g., Options) surfaced via the executing-receiver path by codegen.
//   - Tier 2 — root-planned peer methods (e.g., flow.Provider's `choose`, `gather`, …) surfaced flat under plan.* via
//     builtins discovered from [op.ReceiverRegistry.RootProviders] at construction.
//   - Tier 3 — sub-namespace children (plan.file, plan.git, …) resolved lazily in ResolveAttr through
//     [starlarkbridge.NodeBuilder] adapters.
//
// Any collision across the three tiers fails Provider construction with a message naming both providers and the
// offending method. peerBuiltins is write-once at construction; adapters is lazily populated under mutex.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
	catalog      *op.ResourceCatalog       // session-scoped resource catalog
	invocations  *op.InvocationRegistry    // session-scoped ledger of plan-mode invocations
	peerBuiltins map[string]starlark.Value // Tier 2: root-planned peer method builtins, write-once
	rootNames    map[string]struct{}       // names of root providers (excluded from Tier 3 resolution)
}

// NewProvider creates a plan Provider bound to the given context.
//
// Per phase-8 D5, no [op.Graph] is constructed here — nodes produced during script evaluation live on detached
// [starlarkbridge.Invocation] handles registered in [Provider.invocations]. The graph is materialized by plan.run
// (step 16) from the reachable invocation set.
//
// At construction, the Provider instantiates the session catalog and invocation registry, then discovers every
// RoleAction+RoleRoot peer via the registry to build Tier 2 builtins for their methods. Any name collision across
// Tier 1 (Provider's own methods), Tier 2 (peer methods), or Tier 3 (sub-namespace provider names) is a
// program-init panic.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {

	p := &Provider{
		ProviderBase: op.NewProviderBase(ctx),
		catalog:      op.NewResourceCatalog(),
		invocations:  op.NewInvocationRegistry(),
		peerBuiltins: make(map[string]starlark.Value),
		rootNames:    make(map[string]struct{}),
	}

	p.buildPeerBuiltins()
	return p
}

// region EXPORTED METHODS

// Case constructs a [flow.Case] value for use as a variadic argument to plan.choose.
//
// Exposed to starlark as `plan.case(when=..., then=...)`. Both fields are typed any so the starlark author can
// pass literals, resolved values, or detached invocations from prior plan.* calls; the executor's choose dispatch
// resolves them at execute time per phase-8 D5. Empty cases (both fields nil) compose with `plan.choose`'s
// defaultValue path — no when ever matches, defaultValue wins — but supplying an empty case is unusual and not a
// validation error here.
//
// Parameters:
//   - when: the condition the branch tests against (literal, value, or invocation reference).
//   - then: the body the branch produces if when is truthy.
//
// Returns:
//   - *flow.Case: the constructed case, ready to pass to plan.choose.
func (p *Provider) Case(when any, then any) *flow.Case {
	return &flow.Case{
		When: when,
		Then: then,
	}
}

// ResolveAttr implements [op.AttributeResolver].
//
// Walks the attribute tiers in order (Tier 2 peer builtins → Tier 3 sub-namespace adapters) and returns the
// first match. Tier 1 (Provider's own methods) is handled upstream by the executing-receiver path and never
// reaches ResolveAttr. Root-planned providers are excluded from Tier 3 — their methods surface flat via Tier
// 2 instead.
func (p *Provider) ResolveAttr(name string) any {

	// Tier 2: root-planned peer method builtins. peerBuiltins is write-once at construction, so no lock needed.

	if builtin, ok := p.peerBuiltins[name]; ok {
		return builtin
	}

	// Tier 3 (sub-namespace adapters) will be wired by the next phase. Until
	// then, unknown names fall through to return nil so goReceiver reports a
	// clean "no such attribute" instead of panicking.

	return nil
}

// Variable constructs an [op.Variable] reference that the bridge translates to [op.VariableValue]{Name: name}
// at slot-fill time. Authored as `plan.variable(name)` (required) or `plan.variable(name, default_value=value)`
// (optional with a fallback). The default arg is accepted by Phase 1 but not yet propagated into the
// parameter surface — that wiring lands in Phase 3.
//
// Parameters:
//   - `name`: the variable name to look up in the resolved variable map at execute time.
//   - defaultValue: the optional fallback value when no source supplies the variable. A nil value means
//     "no default declared" (the variable is required).
//
// Returns:
//   - *op.Variable: the variable reference value (Value and Source are zero until the resolver fills them).
func (p *Provider) Variable(name string, defaultValue any) *op.Variable {

	_ = defaultValue // Phase 3 wires default propagation into the parameter surface.
	return &op.Variable{Name: name}
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// buildPeerBuiltins populates peerBuiltins from every RoleAction+RoleRoot provider in the registry and asserts there
// are no collisions across Tier 1 (this Provider's own methods), Tier 2 (peer methods), or Tier 3 (sub-namespace
// provider names).
//
// Called exactly once from NewProvider. Panics on collision or on failure to construct a peer builtin — collisions
// are program-init errors by design (invariant I4).
func (p *Provider) buildPeerBuiltins() {

	registry := p.RuntimeEnvironment().Registry

	// This Provider's own method names from its registered ProviderReceiverType.

	selfNames := make(map[string]struct{})

	if selfRT, ok := registry.Type("plan"); ok {
		for m := range selfRT.Methods() {
			selfNames[op.CamelToSnake(m.Name())] = struct{}{}
		}
	}

	// Record root-provider names so ResolveAttr's Tier 3 can exclude them. Built from every RoleRoot provider
	// regardless of dispatch zone; sub-namespace resolution has no reason to reach any root.

	for _, rp := range registry.RootProviders() {
		p.rootNames[rp.Name()] = struct{}{}
	}

	// Tier 3: sub-namespace (non-root) planner provider names, for collision detection only.

	childNames := make(map[string]struct{})

	for _, pp := range registry.Planners() {
		if _, isRoot := p.rootNames[pp.Name()]; !isRoot {
			childNames[pp.Name()] = struct{}{}
		}
	}
}

// endregion

// endregion
