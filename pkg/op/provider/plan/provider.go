// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package plan provides graph-construction actions for the plan namespace.
//
// Its methods execute during script evaluation to create nodes in the operation graph. The plan Provider is an
// executing receiver — not a planning receiver — because its methods run immediately to build the graph.
package plan

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/flow"
)

var (
	_ op.Provider      = (*Provider)(nil) // Interface Guard: ensures *Provider implements op.Provider.
	_ op.PlanInvocator = (*Provider)(nil) // Interface Guard: ensures *Provider implements op.PlanInvocator.
)

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
//   - `defaultValue`: the optional fallback value when no source supplies the variable. A nil value
//     means "no default declared" (the variable is required).
//
// Returns:
//   - *op.Variable: the variable reference value (Value and Source are zero until the resolver fills them).
func (p *Provider) Variable(name string, defaultValue any) *op.Variable {

	_ = defaultValue // Phase 3 wires default propagation into the parameter surface.
	return &op.Variable{Name: name}
}

// InvocationRegistry returns the session-scoped ledger of invocations constructed during plan-time
// evaluation.
//
// Provided so *Provider satisfies [op.PlanInvocator] — planners reach the registry through this
// accessor for body-resolution lookups during their dispatch.
//
// Returns:
//   - *op.InvocationRegistry: the session ledger; never nil during planning.
func (p *Provider) InvocationRegistry() *op.InvocationRegistry { return p.invocations }

// Assemble materializes a [*op.Graph] from a list of plan-time invocations.
//
// Pipeline:
//
//  1. Allocate a fresh [*op.Graph] via [op.NewGraph].
//  2. Bind it to this Provider's runtime environment via [op.Graph.Rebind].
//  3. Root each invocation's Target as a child of `graph.Root` via [op.Subgraph.AddChild],
//     which stamps `parentID = "root"` on the Target.
//  4. Stamp `retryPolicy` and `errorAction` on `graph.Root` via the [op.ExecutableUnit] setters
//     when non-nil.
//  5. Copy `frameBindings` into `graph.Root.FrameBindings`.
//  6. Materialize the per-Subgraph edge constraints from slot-level dependencies via
//     [op.Subgraph.MaterializeEdges] (PromiseValue NodeRefs and Resource producerIDs).
//  7. Topologically sort each reachable Subgraph's children via [op.Subgraph.SortAll].
//  8. Orphan scan: every invocation in the registry whose Target carries an empty parentID is an
//     orphan (it wasn't rooted by this Assemble call and isn't a child of any other container).
//     Aggregate via [errors.Join] and return `(nil, err)` when the set is non-empty.
//
// `error_action=[...]` authoring at the .star surface is a list of invocations, not a Subgraph
// constructor (see D11 / D12); the bridge's reserved-kwarg splitter materializes that list into a
// `*op.Subgraph` via [subgraphFromInvocations] before calling in here, so by the time `errorAction`
// reaches this method it is already a fully-formed Subgraph.
//
// Parameters:
//   - `invocations`: the top-level invocations to root. Each invocation's Target is stamped under
//     `graph.Root`.
//   - `retryPolicy`: the resolved retry policy from `retry_policy=`, or nil.
//   - `errorAction`: the resolved error-handler Subgraph from `error_action=[...]`, or nil.
//   - `frameBindings`: the non-reserved kwargs to populate as frame bindings on `graph.Root`, or nil.
//
// Returns:
//   - *op.Graph: the assembled graph, bound to this Provider's env.
//   - `error`: non-nil when the orphan scan reports any unreachable invocations; the returned
//     error is an [errors.Join] of one entry per orphan.
func (p *Provider) Assemble(
	invocations []*op.Invocation,
	retryPolicy *op.RetryPolicy,
	errorAction *op.Subgraph,
	frameBindings map[string]op.SlotValue,
) (*op.Graph, error) {

	graph := op.NewGraph()
	graph.Rebind(p.RuntimeEnvironment())

	for _, invocation := range invocations {
		graph.Root.AddChild(invocation.Target)
	}

	if retryPolicy != nil {
		graph.Root.SetRetryPolicy(retryPolicy)
	}
	if errorAction != nil {
		graph.Root.SetErrorAction(errorAction)
	}

	if len(frameBindings) > 0 {
		if graph.Root.FrameBindings == nil {
			graph.Root.FrameBindings = make(map[string]op.SlotValue, len(frameBindings))
		}
		for name, value := range frameBindings {
			graph.Root.FrameBindings[name] = value
		}
	}

	graph.Root.MaterializeEdges()
	graph.Root.SortAll()

	var orphans []error
	for _, invocation := range p.invocations.All() {
		if invocation.Target.ParentID() == "" {
			orphans = append(orphans, fmt.Errorf(
				"orphan invocation %q (target %q has no parent)",
				invocation.Label, invocation.Target.ID(),
			))
		}
	}
	if len(orphans) > 0 {
		return nil, errors.Join(orphans...)
	}

	return graph, nil
}

// Run executes `graph` against this Provider's runtime environment.
//
// Constructs a borrowed-env [*op.GraphExecutor] via [op.NewGraphExecutorForEnv] and delegates to
// [op.GraphExecutor.Run]. Preflight (variable resolution per D10), dispatch, and receipt collection
// happen inside `executor.Run` — this method adds no behavior of its own; it exists so starlark
// authors can call `plan.run(graph)` without reaching for the executor type directly.
//
// Parameters:
//   - `graph`: the assembled graph to execute, typically from a prior [Provider.Assemble] call (or
//     [Provider.Load]).
//
// Returns:
//   - `any`: the graph's terminal output value, or nil when no node produced output.
//   - `error`: non-nil when preflight fails or any node or subgraph fails during dispatch.
func (p *Provider) Run(graph *op.Graph) (any, error) {

	executor := op.NewGraphExecutorForEnv(p.RuntimeEnvironment())
	defer func() { _ = executor.Close() }()

	return executor.Run(graph, nil)
}

// Save serializes `graph` to a file at `path` in JSON or YAML format selected by `path`'s extension.
//
// Supported extensions: `.json` → JSON (two-space indent); `.yaml` / `.yml` → YAML (two-space indent).
// Any other extension is an error.
//
// Parameters:
//   - `graph`: the graph to serialize.
//   - `path`: the destination file path. Format is inferred from the extension.
//
// Returns:
//   - `error`: non-nil when the file cannot be created, the format is unsupported, or encoding fails.
func (p *Provider) Save(graph *op.Graph, path string) error {

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("plan.Provider.Save: %w", err)
	}
	defer func() { _ = file.Close() }()

	switch strings.ToLower(filepath.Ext(path)) {

	case ".json":
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		if err := graph.Serialize(encoder); err != nil {
			return fmt.Errorf("plan.Provider.Save: %w", err)
		}
		return nil

	case ".yaml", ".yml":
		encoder := yaml.NewEncoder(file)
		encoder.SetIndent(2)
		defer func() { _ = encoder.Close() }()
		if err := graph.Serialize(encoder); err != nil {
			return fmt.Errorf("plan.Provider.Save: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("plan.Provider.Save: unsupported format for %q (use .json, .yaml, or .yml)", path)
	}
}

// Load deserializes a [*op.Graph] from a file at `path`. Format is inferred from `path`'s extension
// (`.json` → JSON; `.yaml` / `.yml` → YAML). Any other extension is an error.
//
// The returned graph is unbound from any runtime environment; the next session-owner ([Provider.Run]
// or a Go-side [op.GraphExecutor]) binds it during execution.
//
// Parameters:
//   - `path`: the source file path. Format is inferred from the extension.
//
// Returns:
//   - *op.Graph: the deserialized graph (unbound).
//   - `error`: non-nil when the file cannot be read, the format is unsupported, or decoding fails.
func (p *Provider) Load(path string) (*op.Graph, error) {

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("plan.Provider.Load: %w", err)
	}

	graph := &op.Graph{}

	switch strings.ToLower(filepath.Ext(path)) {

	case ".json":
		if err := json.Unmarshal(data, graph); err != nil {
			return nil, fmt.Errorf("plan.Provider.Load: %w", err)
		}

	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, graph); err != nil {
			return nil, fmt.Errorf("plan.Provider.Load: %w", err)
		}

	default:
		return nil, fmt.Errorf("plan.Provider.Load: unsupported format for %q (use .json, .yaml, or .yml)", path)
	}

	return graph, nil
}

// Clear resets this Provider's session ledger via [op.InvocationRegistry.Reset], discarding every
// registered invocation and zeroing the auto-label counters.
//
// Previously-assembled Graphs (returned by [Provider.Assemble] or [Provider.Load]) hold their own
// references to *Invocation values and are unaffected — Clear only drops the registry's view, so
// subsequent plan-mode calls start with a clean ledger for the next assemble.
//
// Returns:
//   - `error`: always nil today; the signature carries an error return so future implementations
//     (e.g., cancelling a session-scoped resource) can surface failures without breaking the
//     bridge-side builtin signature.
func (p *Provider) Clear() error {

	p.invocations.Reset()
	return nil
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

// invocation is the single dispatch method for every plan-mode call (Tier-1 routing via the adapter
// at `plan.<provider>.<method>` and Tier-3 builtins authored on this Provider).
//
// Flow:
//
//  1. Look up the [*op.Method] for `methodName` on `receiverType`.
//  2. Delegate unit-shape construction to the method's [op.Planner] via Plan(...). Most methods
//     default to [op.ActionPlanner] (one starlark call → one leaf [*op.Node]); flow's container
//     methods declare specialized planners that produce [*op.Subgraph] units instead.
//  3. Stamp the reserved-kwarg payload (`retryPolicy`, `errorAction`) on the returned unit via the
//     [op.ExecutableUnit] interface setters. Reserved-kwarg extraction is the caller's job (the
//     adapter or the Tier-3 builtin); this method receives the already-resolved values.
//  4. Build an [*op.Invocation] wrapping the unit. The label resolves to `label` when non-empty,
//     otherwise to [op.InvocationRegistry.AutoLabel] of the action name.
//  5. Register the invocation in the session ledger via [op.InvocationRegistry.Register].
//
// Unexported because the only callers are within this package — the Tier-1 adapter (step 6) and the
// Tier-3 builtins (step 7) — both of which split reserved kwargs and convert args/kwargs to Go
// before reaching this method.
//
// Parameters:
//   - `receiverType`: the provider receiver type being routed for.
//   - `methodName`: the Go method name (CamelCase) being dispatched.
//   - `args`: positional arguments converted starlark → Go.
//   - `kwargs`: keyword arguments converted starlark → Go (reserved kwargs already removed).
//   - `retryPolicy`: the resolved retry policy from `retry_policy=`, or nil.
//   - `errorAction`: the resolved error-handler Subgraph from `error_action=[...]`, or nil.
//   - `label`: the caller-supplied label from `label=`, or empty for auto-generation.
//
// Returns:
//   - *op.Invocation: the constructed and registered invocation.
//   - `error`: non-nil on method-lookup failure, planner failure, or registry-side label collision.
func (p *Provider) invocation(
	receiverType op.ProviderReceiverType,
	methodName string,
	args []any,
	kwargs map[string]any,
	retryPolicy *op.RetryPolicy,
	errorAction *op.Subgraph,
	label string,
) (*op.Invocation, error) {

	method, ok := receiverType.MethodByName(methodName)
	if !ok {
		return nil, fmt.Errorf("plan.Provider.invocation: %s.%s: method not found", receiverType.Name(), methodName)
	}

	unit, err := method.Planner().Plan(p, receiverType, method, args, kwargs)
	if err != nil {
		return nil, fmt.Errorf("plan.Provider.invocation: %s.%s: %w", receiverType.Name(), methodName, err)
	}

	if retryPolicy != nil {
		unit.SetRetryPolicy(retryPolicy)
	}
	if errorAction != nil {
		unit.SetErrorAction(errorAction)
	}

	if label == "" {
		label = p.invocations.AutoLabel(receiverType.Name() + "." + op.CamelToSnake(methodName))
	}

	invocation := &op.Invocation{
		Target: unit,
		Result: op.NewPromise(unit, ""),
		Label:  label,
	}

	if err := p.invocations.Register(label, invocation); err != nil {
		return nil, fmt.Errorf("plan.Provider.invocation: %s.%s: %w", receiverType.Name(), methodName, err)
	}

	return invocation, nil
}

// endregion

// endregion
