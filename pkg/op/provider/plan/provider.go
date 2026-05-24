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
	"sync"

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
// Provider implements a three-tier attribute resolution (phase-8 D12, plus I4):
//
//   - Tier 1 — sub-namespace adapters (`plan.file`, `plan.shell`, ...). Lazy-minted in [Provider.ResolveAttr] via
//     [newAdapter], cached in `adapters`. Each adapter is a [starlark.HasAttrs] that routes `.<method>(args, kwargs)`
//     through [Provider.invocation].
//   - Tier 2 — promoted methods from root-placed providers (`plan.choose`, `plan.gather`, ...). Surfaced flat under
//     plan.* via builtins discovered from [op.ReceiverRegistry.RootProviders] at construction (any RoleAction+RoleRoot
//     provider contributes its methods).
//   - Tier 3 — Provider's own methods (`plan.variable`, `plan.assemble`, `plan.run`, ...). Surfaced by the executing
//     receiver path that wraps plan.Provider itself as a [goReceiver].
//
// Any collision across the three tiers fails Provider construction with a message naming both providers and the
// offending method. promotedBuiltins is write-once at construction; the adapters are lazily populated under
// `adaptersMutex`.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
	invocations      *op.InvocationRegistry    // session-scoped ledger of plan-mode invocations
	rootNames        map[string]struct{}       // names of root providers (excluded from Tier 1 resolution)
	adapters         map[string]*adapter       // Tier 1: per-sub-namespace adapters, lazily populated
	adaptersMutex    sync.Mutex                // guards adapters
	promotedBuiltins map[string]starlark.Value // Tier 2: root-placed providers' promoted method builtins, write-once
}

// NewProvider creates a plan Provider bound to the given context.
//
// Per phase-8 D5, no [op.Graph] is constructed here — nodes produced during script evaluation live on detached
// [*op.Invocation] handles registered in [Provider.invocations]. The graph is materialized by [Provider.Assemble]
// from the supplied invocation set.
//
// At construction, the Provider instantiates the invocation registry, then discovers every RoleAction+RoleRoot provider
// via the registry to build Tier 2 builtins for their promoted methods. Any name collision across Tier 1 (sub-namespace
// adapter names), Tier 2 (promoted method names), or Tier 3 (this Provider's own method names) is a program-init panic.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {

	p := &Provider{
		ProviderBase:     op.NewProviderBase(ctx),
		invocations:      op.NewInvocationRegistry(),
		rootNames:        make(map[string]struct{}),
		adapters:         make(map[string]*adapter),
		promotedBuiltins: make(map[string]starlark.Value),
	}

	p.buildPromotedBuiltins()
	return p
}

// region EXPORTED METHODS

// Case constructs a [flow.Case] value for use as a variadic argument to plan.choose.
//
// Exposed to starlark as `plan.case(when=..., then=...)`. Both fields are typed any so the starlark author can pass
// literals, resolved values, or detached invocations from prior plan.* calls; the executor's `choose` dispatch resolves
// them at execution time per phase-8 D5. Empty cases (both fields nil) compose with `plan.choose`'s defaultValue path —
// no when ever matches, defaultValue wins — but supplying an empty case is unusual and not a validation error here.
//
// Parameters:
//   - `when`: the condition the branch tests against (literal, value, or invocation reference).
//   - `then`: the body the branch produces if when is truthy.
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
// Walks the attribute tiers in order:
//
//  1. Tier 2: promoted method builtins (including `plan.choose`, `plan.gather`, ...) discovered from
//     [op.ReceiverRegistry.RootProviders] at construction. `promotedBuiltins` are write-once, so the read is lock-free.
//
//  2. Tier 1: sub-namespace adapters (`plan.file`, `plan.shell`, ...). Looked up via [op.ReceiverRegistry.PlannerByName].
//     Root-placed providers are excluded, so their methods surface flat via Tier 2 instead. On hit, the adapter is
//     minted via [Provider.adapterFor] (lazy, cached).
//
// Tier 3: This Provider's own methods: `plan.case`, `plan.variable`, `plan.assemble`, `plan.run`, `plan.save`,
// `plan.load`, `plan.clear`) are resolved upstream by the [starlarkBridge.goReceiver] path's method lookup via the
// codegen-emitted [op.MethodMetadata]; those names never reach ResolveAttr.
//
// A final miss returns `nil` so the upstream `starlarkbridge.goReceiver` reports a clean NoSuchAttr instead of
// panicking.
//
// Parameters:
//   - `name`: the snake-cased attribute name from starlark.
//
// Returns:
//   - `any`: the resolved attribute (a [starlark.Value] from promotedBuiltins, or an [*adapter]), or nil when no tier
//     matches.
func (p *Provider) ResolveAttr(name string) any {

	if builtin, ok := p.promotedBuiltins[name]; ok {
		return builtin
	}

	if _, isRoot := p.rootNames[name]; isRoot {
		return nil
	}

	if receiverType, ok := p.RuntimeEnvironment().Registry.PlannerByName(name); ok {
		return p.adapterFor(receiverType)
	}

	return nil
}

// Variable constructs an [op.Variable] reference that the bridge translates to [op.VariableValue]{Name: name}
// at slot-fill time. Authored as `plan.variable(name)` (required) or `plan.variable(name, default_value=value)`
// (optional with a fallback). The default arg is accepted by Phase 1 but not yet propagated into the parameter
// surface — that wiring lands in Phase 3.
//
// Parameters:
//   - `name`: the variable name to look up in the resolved variable map at execution time.
//   - `defaultValue`: the optional fallback value when no source supplies the variable. A nil value means "no default
//     declared", meaning that the variable is required.
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
// Signature is codegen-compatible — all parameter types are reachable from starlark via the
// standard [starlarkbridge] conversion path; plan-specific projections happen inside this method.
//
// +devlore:defaults retryPolicy=nil, errorAction=nil, frameBindings=nil
//
// Pipeline:
//
//  1. Allocate a fresh [*op.Graph] via [op.NewGraph].
//  2. Bind it to this Provider's runtime environment via [op.Graph.Rebind].
//  3. Root each invocation's Target as a child of `graph.Root` via [op.Subgraph.AddChild],
//     which stamps `parentID = "root"` on the Target.
//  4. Stamp `retryPolicy` on `graph.Root` via [op.ExecutableUnit.SetRetryPolicy] when non-nil.
//  5. If `errorAction` is non-empty, materialize the list of invocations into a `*op.Subgraph`
//     via [subgraphFromInvocations] and stamp it via [op.Subgraph.SetErrorAction].
//  6. Project each `frameBindings` entry through [projectToSlotValue] (ImmediateValue /
//     PromiseValue / VariableValue) and stamp the result on `graph.Root.FrameBindings`.
//  7. Materialize the per-Subgraph edge constraints from slot-level dependencies via
//     [op.Subgraph.MaterializeEdges] (PromiseValue UnitRefs and Resource producerIDs).
//  8. Topologically sort each reachable Subgraph's children via [op.Subgraph.SortAll].
//  9. Orphan scan: every invocation in the registry whose Target carries an empty parentID is an
//     orphan (it wasn't rooted by this Assemble call and isn't a child of any other container).
//     Aggregate via [errors.Join] and return `(nil, err)` when the set is non-empty.
//  10. Catalog handoff: assign `graph.Catalog = env.Catalog`, then nil `env.Catalog`. The graph carries the
//     planning-interned [op.ResourceCatalog] into execution; the planning env loses its catalog handle,
//     enforcing the freeze invariant — any post-Assemble Discover / GetOrCreate against this env crashes loudly.
//
// Parameters:
//   - `invocations`: the top-level invocations to root under `graph.Root`.
//   - `retryPolicy`: the resolved retry policy from `retry_policy=`, or nil.
//   - `errorAction`: the list of invocations from `error_action=[...]`. Materializes internally
//     into a Subgraph; empty / nil means no error action.
//   - `frameBindings`: the non-reserved kwargs to populate as frame bindings on `graph.Root`.
//     Values are projected to [op.SlotValue] via [projectToSlotValue].
//
// Returns:
//   - *op.Graph: the assembled graph, bound to this Provider's env.
//   - `error`: non-nil when the orphan scan reports any unreachable invocations; the returned
//     error is an [errors.Join] of one entry per orphan.
func (p *Provider) Assemble(
	invocations []*op.Invocation,
	retryPolicy *op.RetryPolicy,
	errorAction []*op.Invocation,
	frameBindings map[string]any,
) (*op.Graph, error) {

	graph := op.NewGraph()
	graph.Rebind(p.RuntimeEnvironment())

	for _, invocation := range invocations {
		graph.Root.AddChild(invocation.Target)
	}

	if retryPolicy != nil {
		graph.Root.SetRetryPolicy(retryPolicy)
	}
	if len(errorAction) > 0 {
		errorActionSg, err := subgraphFromInvocations(p.RuntimeEnvironment(), "error_action", errorAction)
		if err != nil {
			return nil, fmt.Errorf("plan.assemble: %w", err)
		}
		graph.Root.SetErrorAction(errorActionSg)
	}

	for name, value := range frameBindings {
		graph.Root.SetSlot(name, projectToSlotValue(value))
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

	if err := op.ValidateGraph(graph); err != nil {
		return nil, fmt.Errorf("plan.assemble: %w", err)
	}

	// Hand the planning [op.ResourceCatalog] off from the runtime environment to the graph. From this point on the
	// graph is self-contained — `graph.Catalog` carries the interned catalog into serialization (well, into in-memory
	// post-plan hand-off; the catalog itself is not serialized) and into the per-run clone performed by
	// [op.GraphExecutor.Run]. Nilling `env.Catalog` enforces the freeze invariant: any provider Discover / GetOrCreate
	// call routed through this planning env after Assemble returns will dereference a nil catalog and crash loudly,
	// which is the correct behavior — the planning phase is over.
	graph.Catalog = p.RuntimeEnvironment().Catalog
	p.RuntimeEnvironment().Catalog = nil

	return graph, nil
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

	var format string
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		format = "json"
	case ".yaml", ".yml":
		format = "yaml"
	default:
		return nil, fmt.Errorf("plan.Provider.Load: unsupported format for %q (use .json, .yaml, or .yml)", path)
	}

	graph, err := op.LoadGraph(p.RuntimeEnvironment(), data, format)
	if err != nil {
		return nil, fmt.Errorf("plan.Provider.Load %q: %w", path, err)
	}

	if err := op.ValidateGraph(graph); err != nil {
		return nil, fmt.Errorf("plan.Provider.Load %q: %w", path, err)
	}

	return graph, nil
}

// Clear resets this Provider's session ledger via [op.InvocationRegistry.Reset], discarding every registered invocation
// and zeroing the auto-label counters.
//
// Previously assembled Graphs (returned by [Provider.Assemble] or [Provider.Load]) hold their own references to
// *Invocation values and are unaffected — Clear only drops the registry's view, so subsequent plan-mode calls start
// with a clean ledger for the next assembly.
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

// buildPromotedBuiltins populates promotedBuiltins from every RoleAction+RoleRoot provider in the registry and asserts
// there are no collisions across Tier 1 (sub-namespace adapter names), Tier 2 (promoted methods), or Tier 3 (this
// Provider's own methods).
//
// Called exactly once from NewProvider. Panics on collision or on failure to construct a promoted
// builtin — collisions are program-init errors by design (invariant I4).
func (p *Provider) buildPromotedBuiltins() {

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

// adapterFor returns the cached adapter for `receiverType`, minting one via [newAdapter] on first
// lookup. The lookup-then-mint pair runs under [Provider.adaptersMutex] so concurrent first-touch
// resolutions from multiple starlark threads don't race on the cache.
//
// Parameters:
//   - `receiverType`: the sub-namespace's provider receiver type.
//
// Returns:
//   - *adapter: the cached or freshly-minted adapter; never nil.
func (p *Provider) adapterFor(receiverType op.ProviderReceiverType) *adapter {

	name := receiverType.Name()

	p.adaptersMutex.Lock()
	defer p.adaptersMutex.Unlock()

	if existing, ok := p.adapters[name]; ok {
		return existing
	}

	fresh := newAdapter(p, receiverType)
	p.adapters[name] = fresh
	return fresh
}

// endregion

// endregion
