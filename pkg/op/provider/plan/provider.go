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

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"go.starlark.net/starlark"
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/application"
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
//   - Tier 3 — Provider's own methods (`plan.variable`, `plan.assemble_definition`, `plan.save_definition`, ...).
//     Surfaced by the executing receiver path that wraps plan.Provider itself as a [goReceiver].
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

// NewProvider creates a plan Provider bound to the given runtime environment.
//
// Per phase-8 D5, no [op.Graph] is constructed here — nodes produced during script evaluation live on detached
// [*op.Invocation] handles registered in [Provider.invocations]. The graph is materialized
// by [Provider.AssembleDefinition] from the supplied invocation set.
//
// At construction, the Provider instantiates the invocation registry, then discovers every RoleAction+RoleRoot provider
// via the registry to build Tier 2 builtins for their promoted methods. Any name collision across Tier 1 (sub-namespace
// adapter names), Tier 2 (promoted method names), or Tier 3 (this Provider's own method names) is a program-init panic.
//
// Parameters:
//   - `ctx`: the runtime environment the Provider binds to and reaches for the receiver registry, runtime context,
//     and downstream provider construction.
//
// Returns:
//   - *Provider: the constructed Provider with Tier 2 promoted builtins populated and Tier 1 adapter cache empty.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {

	p := &Provider{
		ProviderBase:     op.NewProviderBase(runtimeEnvironment),
		invocations:      op.NewInvocationRegistry(),
		rootNames:        make(map[string]struct{}),
		adapters:         make(map[string]*adapter),
		promotedBuiltins: make(map[string]starlark.Value),
	}

	p.buildPromotedBuiltins()
	return p
}

// region EXPORTED METHODS

// region State management

// InvocationRegistry returns the session-scoped ledger of invocations constructed during plan-time evaluation.
//
// Provided so *Provider satisfies [op.PlanInvocator] — planners reach the registry through this accessor for
// body-resolution lookups during their dispatch.
//
// Returns:
//   - *op.InvocationRegistry: the session ledger; never nil during planning.
func (p *Provider) InvocationRegistry() *op.InvocationRegistry { return p.invocations }

// endregion

// region Behaviors

// Fallible actions

// AssembleDefinition materializes a [*op.Graph] from a list of plan-time invocations.
//
// Signature is codegen-compatible — all parameter types are reachable from starlark via the standard [starlarkbridge]
// conversion path; plan-specific projections happen inside this method.
//
// Pipeline:
//
//  1. Project the inputs: the invocation list becomes the graph's root children ([]op.ExecutableUnit); the
//     error-action invocations become a `*op.Subgraph` via [subgraphFromInvocations]; and the slot map becomes
//     [op.Binding]s via [projectToBinding].
//  2. Take ownership of the catalog: capture [op.RuntimeEnvironment.Catalog] and clear the runtime environment's
//     reference to it — ownership transfers to the graph being constructed.
//  3. Construct the graph: stamp `origin.Tool` from the planning program name ([RuntimeEnvironment.Application].Name),
//     then call the sealed [op.NewGraph] constructor with the origin, catalog, root children, retry policy, error
//     action, and slots. [op.NewGraph] materializes the edges, sorts the children, computes the canonical content, and
//     hashes it via [op.GitStyleChecksum]. Sub-graphs are left unsigned pending the sops rewrite — no signing client
//     is propagated.
//  4. Scan for orphans: any invocation in the registry whose Target carries an empty parentID was never rooted by this
//     AssembleDefinition call and is not a child of any other container. Aggregate via [errors.Join] and return
//     `(nil, err)` when the set is non-empty.
//  5. Validate: [op.ValidateGraph] runs against the sealed graph and returns its joined violations as a single error.
//
// Parameters:
//   - `invocations`: the top-level invocations to root under `graph.Root`.
//   - `slots`: the non-reserved kwargs to populate as slots on `graph.Root`. Values are projected to
//     [op.Binding] via [projectToBinding].
//   - `errorAction`: the list of invocations from `error_action=[...]`. Materializes internally into a Subgraph;
//     empty / nil means no error action.
//   - `retryPolicy`: the resolved retry policy from `retry_policy=`, or nil.
//   - `origin`: the tool-stamp [op.Origin] for the assembled graph; the zero value when omitted (the .star
//     `plan.assemble_definition` surface never supplies it — Origin is a Go-side caller concern).
//
// Returns:
//   - `*op.Graph`: the assembled graph, bound to this Provider's runtime environment.
//   - `error`: non-nil when the orphan scan reports any unreachable invocations; the returned error is an [errors.Join]
//     of one entry per orphan.
//
// +devlore:defaults retryPolicy=nil, errorAction=nil, slots=nil, origin=
func (p *Provider) AssembleDefinition(
	invocations []*op.Invocation,
	slots map[string]any,
	errorAction []*op.Invocation,
	retryPolicy *op.RetryPolicy,
	origin op.Origin,
) (*op.Graph, error) {

	rootChildren := make([]op.ExecutableUnit, 0, len(invocations))
	for _, invocation := range invocations {
		rootChildren = append(rootChildren, invocation.Target)
	}

	var errorActionSg *op.Subgraph
	if len(errorAction) > 0 {
		var err error
		errorActionSg, err = subgraphFromInvocations(p.RuntimeEnvironment(), "error_action", errorAction)
		if err != nil {
			return nil, fmt.Errorf("plan.assemble_definition: %w", err)
		}
	}

	bindings := make(map[string]op.Binding, len(slots))
	for name, value := range slots {
		bindings[name] = projectToBinding(value)
	}

	// A nil origin (the default when a Starlark caller omits origin=) carries no scope or annotations; default it to the
	// zero OriginBase so the tool-stamping below reads Scope/Annotations without a nil-interface panic.
	if origin == nil {
		origin = op.OriginBase{}
	}

	// Tool is framework-owned provenance: stamp it from the planning program name (Application.Name), so callers
	// never pass it. Scope and Annotations on `origin` remain caller-supplied.
	if app := p.RuntimeEnvironment().Application; app != nil {
		origin = op.NewOriginBase(app.Name, origin.Scope(), origin.Annotations())
	}

	catalog := p.RuntimeEnvironment().ResourceCatalog
	p.RuntimeEnvironment().ResourceCatalog = nil

	spec := op.NewGraphSpec().
		WithOrigin(origin).
		WithUnits(rootChildren...).
		WithResourceCatalog(catalog).
		WithErrorAction(errorActionSg).
		WithRetryPolicy(retryPolicy)
	for name, value := range bindings {
		spec.WithSlot(name, value)
	}

	graph, err := op.NewGraph(spec)
	if err != nil {
		return nil, fmt.Errorf("plan.assemble_definition: %w", err)
	}

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
		return nil, fmt.Errorf("plan.assemble_definition: %w", err)
	}

	return graph, nil
}

// Plan registers am invocation from Go, mirroring what the starlark bridge does from a `plan.<name>(...)` call.
//
// The framework resolves the action from `name` (e.g. "pkg.install"). The caller never builds an [op.Action].
//
// The resolved leaf is planned through the owning method's [op.ActionPlanner], wrapped in an [*op.Invocation], and
// registered in this Provider's session ledger; a later [Provider.AssembleDefinition] over the ledger materializes
// the graph, so Go-built and `.star`-built invocations pool in the same registry.
//
// Parameters:
//   - `name`: the dotted action name "<receiver>.<method>" (e.g. "pkg.install"), as [op.Method.ActionName] reports it.
//   - `args`: positional arguments for the method, in declared order.
//   - `kwargs`: keyword arguments by parameter name.
//
// Returns:
//   - `*op.Invocation`: the registered invocation; its `Target` is the planned unit.
//   - `error`: non-nil when `name` resolves to no known action, or the planner / registry rejects the call.
func (p *Provider) Plan(name string, args []any, kwargs map[string]any) (*op.Invocation, error) {

	dot := strings.LastIndex(name, ".")

	if dot < 0 {
		return nil, fmt.Errorf("plan.Provider.Plan: invalid action name %q: no dot", name)
	}

	receiverType, ok := op.ReceiverRegistry().ActionByName(name[:dot])
	if !ok {
		return nil, fmt.Errorf("plan.Provider.Plan: unknown action provider %q in %q", name[:dot], name)
	}

	// MethodByName keys on the Go (camel) name; `name` carries the snake attribute. Resolve by snake-matching, as the
	// starlark adapter does, then hand the camel name to invocation.

	methodSnake := name[dot+1:]

	for method := range receiverType.Methods() {
		if op.CamelToSnake(method.Name()) == methodSnake {
			return p.invocation(receiverType, method.Name(), args, kwargs, nil, nil, "")
		}
	}

	return nil, fmt.Errorf("plan.Provider.Plan: method %q not found on %q", methodSnake, name[:dot])
}

// Clear resets this Provider's session ledger via [op.InvocationRegistry.Reset].
//
// Discards every registered invocation and zeroes the auto-label counters. Previously assembled Graphs (returned by
// [Provider.AssembleDefinition] or [Provider.LoadDefinition]) hold their own references to *Invocation values and are
// unaffected — Clear only drops the registry's view, so subsequent plan-mode calls start with a clean ledger for the
// next assembly.
//
// Returns:
//   - `error`: always nil today; the signature carries an error return so future implementations (e.g., canceling
//     a session-scoped resource) can surface failures without breaking the bridge-side builtin signature.
func (p *Provider) Clear() error {

	p.invocations.Reset()
	return nil
}

// LoadDefinition deserializes a [*op.Graph] from a file at `path`. Format is inferred from `path`'s extension.
//
// Supported extensions: `.json` → JSON; `.yaml` / `.yml` → YAML. Any other extension is an error.
//
// The returned graph is unbound from any runtime environment; the next session-owner (a Go-side [op.GraphExecutor])
// binds it during execution.
//
// Parameters:
//   - `path`: the source file path. Format is inferred from the extension.
//
// Returns:
//   - *op.Graph: the deserialized graph (unbound).
//   - `error`: non-nil when the file cannot be read, the format is unsupported, or decoding fails.
func (p *Provider) LoadDefinition(path string) (*op.Graph, error) {

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("plan.Provider.LoadDefinition: %w", err)
	}

	var format string
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		format = "json"
	case ".yaml", ".yml":
		format = "yaml"
	default:
		return nil, fmt.Errorf("plan.Provider.LoadDefinition: unsupported format for %q (use .json, .yaml, or .yml)", path)
	}

	graph, err := op.LoadGraph(p.RuntimeEnvironment(), data, format)
	if err != nil {
		return nil, fmt.Errorf("plan.Provider.LoadDefinition %q: %w", path, err)
	}

	if err := op.ValidateGraph(graph); err != nil {
		return nil, fmt.Errorf("plan.Provider.LoadDefinition %q: %w", path, err)
	}

	return graph, nil
}

// SaveDefinition serializes `graph` to a file at `path` in JSON or YAML format selected by `path`'s extension.
//
// Supported extensions: `.json` → JSON (two-space indent); `.yaml` / `.yml` → YAML (two-space indent). Any other
// extension is an error.
//
// Parameters:
//   - `graph`: the graph to serialize.
//   - `path`: the destination file path. Format is inferred from the extension.
//
// Returns:
//   - `error`: non-nil when the file cannot be created, the format is unsupported, or encoding fails.
func (p *Provider) SaveDefinition(graph *op.Graph, path string) (err error) {

	var file *os.File

	file, err = os.Create(path)
	if err != nil {
		return fmt.Errorf("plan.Provider.SaveDefinition: %w", err)
	}

	defer iox.Close(&err, file)

	switch strings.ToLower(filepath.Ext(path)) {

	case ".json":

		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")

		if err := graph.Serialize(encoder); err != nil {
			return fmt.Errorf("plan.Provider.SaveDefinition: %w", err)
		}

		return nil

	case ".yaml", ".yml":

		encoder := yaml.NewEncoder(file)
		defer iox.Close(&err, encoder)
		encoder.SetIndent(2)

		if err := graph.Serialize(encoder); err != nil {
			return fmt.Errorf("plan.Provider.SaveDefinition: %w", err)
		}

		return nil

	default:

		return fmt.Errorf("plan.Provider.SaveDefinition: unsupported format for %q (use .json, .yaml, or .yml)", path)
	}
}

// Spec constructs a fresh [*op.RuntimeEnvironmentSpec] for use with [Provider.Run].
//
// Exposed to starlark as `plan.spec(program_name=..., root_path=..., flags=...)` — all three arguments optional.
//
// When an argument is the zero value (empty `programName`, empty `rootPath`, or nil `flags`), the planning runtime
// environment's corresponding field supplies the default. The planning env always carries these — the host that invoked
// [op.Plan] passed its own [application.Application] and [fsroot.Root]. Net effect: `plan.spec()` with no arguments
// produces a spec equivalent to the planning env's, modulo a fresh [fsroot.Root] handle at the same anchor path.
//
// Each call mints a fresh [fsroot.Root] via [fsroot.OpenConfined] anchored at the resolved `rootPath`, so successive
// [Provider.Run] calls don't share a Root that closes when the first executor finishes. The returned spec's
// [op.ReceiverRegistry] is a freshly-built one from the announced providers — independent of the planning env's
// registry.
//
// Use from a `.star` script:
//
//	graph = plan.assemble_definition([...])
//	plan.run(graph, plan.spec())                                  # all defaults — common case
//	plan.run(graph, plan.spec(root_path="/tmp/staging"))           # override one
//	plan.run(graph, plan.spec(flags={"dry-run": True}))            # override another
//
// +devlore:defaults programName="", rootPath="", flags=nil
//
// Parameters:
//   - `programName`: the tool name; flows into [application.Application.Name] and drives the variable resolver's
//     env-prefix derivation. Empty string → defaults to the planning env's `Application.Name`.
//   - `rootPath`: the absolute path the confined [fsroot.Root] is anchored at. Empty string → defaults to the planning
//     env's `Root.Name()`.
//   - `flags`: the [application.Application.Flags] map. Nil → defaults to the planning env's `Application.Flags`.
//
// Returns:
//   - *op.RuntimeEnvironmentSpec: the constructed spec.
//   - `error`: non-nil when [fsroot.OpenConfined] fails (the target root does not exist or is not accessible).
func (p *Provider) Spec(programName string, rootPath string, flags map[string]any) (*op.RuntimeEnvironmentSpec, error) {

	env := p.RuntimeEnvironment()

	if programName == "" {
		programName = env.Application.Name
	}
	if rootPath == "" {
		rootPath = env.Root.Name()
	}
	if flags == nil {
		flags = env.Application.Flags
	}

	root, err := fsroot.OpenConfined(rootPath)
	if err != nil {
		return nil, fmt.Errorf("plan.Provider.Spec: open root %s: %w", rootPath, err)
	}

	return op.NewRuntimeEnvironmentSpec(programName).
		WithRoot(root).
		WithPlatform(env.Platform).
		WithApplication(&application.Application{
			Name:      programName,
			Flags:     flags,
			Overrides: env.Application.Overrides,
			Config:    env.Application.Config,
		}), nil
}

// Run executes `graph` against the supplied [*op.RuntimeEnvironmentSpec].
//
// Exposed to starlark as `plan.run(graph, spec)`.
//
// Builds a fresh [*op.GraphExecutor] from `(graph, spec)` and dispatches via [op.GraphExecutor.Run]. The executor owns
// the per-Run env's lifecycle — env construction, Catalog clone, Root close, variable resolution preflight, graph
// dispatch, compensation unwind on failure — all the runner-side responsibilities the .star script used to rely on the
// host to handle. With `plan.run` exposed, the script drives execute itself; hosts (devlore-test, writ, lore, …) reduce
// to evaluating the script and surfacing its errors.
//
// The returned `any` is the terminal node's output — the same shape [op.GraphExecutor.Run] produces today. For the
// common case (graph with no value-producing terminal node) this is nil. Scripts that want richer post-execute
// introspection consult the env-side collectors (status narrator, result pipeline, audit receipts) rather than the
// return value.
//
// Parameters:
//   - `graph`: the assembled graph; typically the return of a preceding [Provider.AssembleDefinition] call or a
//     deserialized one from [Provider.LoadDefinition].
//   - `spec`: the runtime environment spec; typically built via [Provider.Spec] or supplied by the host.
//
// Returns:
//   - `any`: the terminal node's result, or nil when the graph has no value-producing terminal node.
//   - `error`: non-nil when preflight or dispatch fails; the unwind error joins on if compensation also fails.
func (p *Provider) Run(graph *op.Graph, spec *op.RuntimeEnvironmentSpec) (any, error) {

	if graph == nil {
		return nil, fmt.Errorf("plan.Provider.Run: graph is nil")
	}
	if spec == nil {
		return nil, fmt.Errorf("plan.Provider.Run: spec is nil")
	}

	return op.NewGraphExecutor(graph, spec).Run(p.RuntimeEnvironment().Context, nil)
}

// Actions

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
func (p *Provider) Case(when, then any) *flow.Case {
	return &flow.Case{
		When: when,
		Then: then,
	}
}

// Origin constructs an [op.Origin] carrying the planning scope for the assembled graph.
//
// Exposed to starlark as `plan.origin(scope)`. Tool is deliberately NOT a parameter — [Provider.AssembleDefinition]
// stamps it from the program name ([RuntimeEnvironment.Application].Name); Tool is framework-owned. Graph-level
// annotations are not exposed through this constructor.
//
// Parameters:
//   - `scope`: the planning scope for the graph (e.g. writ "system"/"home"); drives the persisted graph filename.
//
// Returns:
//   - op.Origin: an Origin with Scope set; Tool is stamped by [Provider.AssembleDefinition].
func (p *Provider) Origin(scope string) op.Origin {
	return op.NewOriginBase("", scope, op.AnnotationMap{})
}

// ResolveAttr implements [op.AttributeResolver].
//
// Walks the attribute tiers in order:
//
//  1. Tier 2: promoted method builtins (including `plan.choose`, `plan.gather`, ...) discovered from
//     [op.ReceiverRegistry.RootProviders] at construction. `promotedBuiltins` are write-once, so the read is lock-free.
//
//  2. Tier 1: sub-namespace adapters (`plan.file`, `plan.shell`,...). Looked up via op.ReceiverRegistry.PlannerByName].
//     Root-placed providers are excluded, so their methods surface flat via Tier 2 instead. On hit, the adapter is
//     minted via [Provider.adapterFor] (lazy, cached).
//
// Tier 3: This Provider's own methods: `plan.case`, `plan.variable`, `plan.assemble_definition`,
// `plan.save_definition`, `plan.load_definition`, `plan.clear`) are resolved upstream by the
// [starlarkBridge.goReceiver] path's method lookup via the codegen-emitted
// [op.MethodMetadata]; those names never reach ResolveAttr.
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

	if receiverType, ok := op.ReceiverRegistry().PlannerByName(name); ok {
		return p.adapterFor(receiverType)
	}

	return nil
}

// Variable constructs an [op.Variable] reference that resolves to its slot-fill value at execution time.
//
// Authored as `plan.variable(name)` (required) or `plan.variable(name, default_value=value)` (optional with a
// fallback). The bridge translates the returned reference to [op.VariableBinding]{Name: name} at slot-fill time. The
// default arg is accepted by Phase 1 but not yet propagated into the parameter surface — that wiring lands in Phase 3.
//
// +devlore:defaults defaultValue=nil
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

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// Fallible actions

// invocation is the single dispatch method for every plan-mode call.
//
// Used by Tier-1 routing via the adapter at `plan.<provider>.<method>` and Tier-2 promoted builtins on the flat
// `plan.*` namespace.
//
// Flow:
//
//  1. Look up the [*op.Method] for `methodName` on `receiverType`.
//  2. Delegate unit-shape construction to the method's [op.Planner] via Plan(...). Most methods default to
//     [op.ActionPlanner] (one starlark call → one leaf [*op.Node]); flow's container methods declare specialized
//     planners that produce [*op.Subgraph] units instead.
//  3. Stamp the reserved-kwarg payload (`retryPolicy`, `errorAction`) on the returned unit via the [op.ExecutableUnit]
//     interface setters. Reserved-kwarg extraction is the caller's job (the adapter or the Tier-2 builtin); this method
//     receives the already-resolved values.
//  4. Build an [*op.Invocation] wrapping the unit. The label resolves to `label` when non-empty, otherwise to
//     [op.InvocationRegistry.AutoLabel] of the action name.
//  5. Register the invocation in the session ledger via [op.InvocationRegistry.Register].
//
// Unexported because the only callers are within this package — the Tier-1 adapter and the Tier-2 promoted builtins —
// both of which split reserved kwargs and convert args/kwargs to Go before reaching this method.
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

	unit, err := method.Planner().Plan(p, receiverType, method, args, kwargs, nil, errorAction, retryPolicy)
	if err != nil {
		return nil, fmt.Errorf("plan.Provider.invocation: %s.%s: %w", receiverType.Name(), methodName, err)
	}

	if label == "" {
		label = p.invocations.AutoLabel(receiverType.Name() + "." + op.CamelToSnake(methodName))
	}

	invocation := &op.Invocation{
		Target: unit,
		Label:  label,
	}

	if err := p.invocations.Register(label, invocation); err != nil {
		return nil, fmt.Errorf("plan.Provider.invocation: %s.%s: %w", receiverType.Name(), methodName, err)
	}

	return invocation, nil
}

// Actions

// adapterFor returns the cached adapter for `receiverType`, minting one on first lookup.
//
// The lookup-then-mint pair runs under [Provider.adaptersMutex] so concurrent first-touch resolutions from multiple
// starlark threads don't race on the cache.
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

// buildPromotedBuiltins populates promotedBuiltins from every RoleRoot provider in the registry.
//
// Asserts there are no collisions across Tier 1 (sub-namespace adapter names), Tier 2 (promoted methods), or Tier 3
// (this Provider's own methods). Called exactly once from NewProvider. Panics on collision — collisions are
// program-init errors by design (invariant I4).
func (p *Provider) buildPromotedBuiltins() {

	registry := op.ReceiverRegistry()

	selfNames := make(map[string]struct{})

	if selfRT, ok := registry.Type("plan"); ok {
		for m := range selfRT.Methods() {
			selfNames[op.CamelToSnake(m.Name())] = struct{}{}
		}
	}

	for _, rp := range registry.RootProviders() {
		p.rootNames[rp.Name()] = struct{}{}
	}

	childNames := make(map[string]struct{})

	for _, pp := range registry.Planners() {
		if _, isRoot := p.rootNames[pp.Name()]; !isRoot {
			childNames[pp.Name()] = struct{}{}
		}
	}

	for _, rp := range registry.RootProviders() {

		for method := range rp.Methods() {

			snakeName := op.CamelToSnake(method.Name())
			actionName := rp.Name() + "." + snakeName

			if _, ok := selfNames[snakeName]; ok {
				panic(fmt.Sprintf(
					"plan: promoted method %q from %s collides with plan.Provider's own method",
					snakeName, rp.Name(),
				))
			}
			if _, ok := childNames[snakeName]; ok {
				panic(fmt.Sprintf(
					"plan: promoted method %q from %s collides with sub-namespace adapter name",
					snakeName, rp.Name(),
				))
			}
			if _, ok := p.promotedBuiltins[snakeName]; ok {
				panic(fmt.Sprintf(
					"plan: promoted method %q from %s collides with another root provider's method",
					snakeName, rp.Name(),
				))
			}

			p.promotedBuiltins[snakeName] = starlark.NewBuiltin(
				actionName,
				dispatchBuiltinBody(p, rp, method.Name(), actionName),
			)
		}
	}
}

// endregion

// endregion
