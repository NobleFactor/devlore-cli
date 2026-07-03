// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package flow implements flow-control methods for execution graphs.
//
// Its methods are dispatched during graph execution — they are actions, not modules. The Provider holds a graph and
// operates on it directly, the same way the plan provider does.
package flow

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var _ op.Provider = (*Provider)(nil) // Interface Guard

// Provider implements flow-control actions for execution graphs.
//
// Flow is a root-planned provider (Phase 8 D12): its methods surface flat under the plan namespace (e.g., plan.choose,
// plan.gather) rather than nested under plan.flow.*. Starlark authors call these as plain planner primitives; Go-side
// the planner primitives carry bare action names on the created graph nodes (choose, gather, subgraph, …).
//
// +devlore:access=planned
// +devlore:root=true
type Provider struct {
	op.ProviderBase
}

// Case is one case statement of a `plan.choose`: a `when`-subgraph evaluated for truthiness, paired with a
// `then`-subgraph run when the `when` is truthy.
//
// A case holds its two subgraphs by value. [ChoosePlanner] assembles the cases (plus the default) into the choose
// subgraph's conditional ladder — see `docs/plans/extract-starlark-from-op/phase-8/steps/10-plan-choose.md`. Constructed
// by `plan.case(when=<body>, then=<body>)`.
type Case struct {
	When op.Subgraph // the when-subgraph, evaluated for truthiness
	Then op.Subgraph // the then-subgraph, run when When is truthy
}

// NewCase constructs a [Case] by sealing the `when` and `then` bodies into subgraphs.
//
// Each body is a list of invocations — the same construction `plan.subgraph(body=[...])` uses ([resolveBodyChildren]
// → [op.NewSubgraph] under a by-name flow.subgraph binding). The case is plan-time data: [ChoosePlanner] lays the two
// subgraphs into the choose subgraph's decision tree.
//
// Parameters:
//   - `when`: the when-body; its subgraph is a decision node, evaluated for truthiness at execution.
//   - `then`: the then-body; its subgraph is the leaf run when the when's result is truthy.
//
// Returns:
//   - `*Case`: the constructed case.
//   - `error`: non-nil when either body is malformed.
func NewCase(when, then any) (*Case, error) {

	whenSubgraph, err := bodySubgraph("when", when)
	if err != nil {
		return nil, fmt.Errorf("flow.NewCase: %w", err)
	}

	thenSubgraph, err := bodySubgraph("then", then)
	if err != nil {
		return nil, fmt.Errorf("flow.NewCase: %w", err)
	}

	return &Case{When: *whenSubgraph, Then: *thenSubgraph}, nil
}

// NewProvider creates a flow Provider bound to the given context.
//
// The graph reference is not captured at construction — flow methods read it per dispatch from
// [op.ActivationRecord.Graph], stamped by the executor when the activation is built.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {

	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// region EXPORTED METHODS

// region Behaviors

// Compensable actions

// Choose executes the choose subgraph — and nothing else.
//
// The choose's logic lives entirely in the graph [ChoosePlanner] builds from the case statements: a conditional ladder
// in which each `when`-subgraph's truthy edge routes to its own `then`-subgraph and its falsy edge to the next case's
// `when`, with the last falsy edge routing to the default. Executing that topology is what produces the first-truthy
// short-circuit; Choose carries **no** selection logic of its own — it walks the children exactly as [Provider.Subgraph]
// does. See the step-10 design doc (`docs/plans/extract-starlark-from-op/phase-8/steps/10-plan-choose.md`).
//
// A zero-case choose (default only) carries no guarded edges and takes the ordinary run-all walk — defined behavior,
// per the switch-statement precedent: the single default child runs and its result is the choose result.
//
// +devlore:planner=ChoosePlanner
//
// Parameters:
//   - `activation`: the per-dispatch record; `activation.Unit` is the choose `*op.Subgraph` and `activation.Stack` is
//     the executor-owned recovery stack.
//   - `kwargs`: frame-binding kwargs, layered onto the inherited variable frame like [Provider.Subgraph].
//
// Returns:
//   - `any`: the result of whichever branch the graph walk reached.
//   - *op.RecoveryStack: `activation.Stack`, carrying the branches that ran.
//   - `error`: non-nil on a child failure or when `activation.Unit` is not a `*op.Subgraph`.
func (p *Provider) Choose(
	activation *op.ActivationRecord,
	kwargs map[string]any,
) (any, *op.RecoveryStack, error) {

	subgraph, ok := activation.Unit.(*op.Subgraph)
	if !ok {
		return nil, nil, fmt.Errorf("flow.Choose: activation.Unit is %T, want *op.Subgraph", activation.Unit)
	}

	stack := activation.Stack

	result, err := walkSubgraphChildren(
		activation, activation.Context, subgraph, stack, activation.Variables, subgraph.ErrorAction())
	if err != nil {
		return nil, stack, fmt.Errorf("flow.Choose: %w", err)
	}

	return result, stack, nil
}

// CompensateChoose unwinds the recovery state captured by a successful [Provider.Choose] call.
//
// Today this is structurally a delegation to [op.RecoveryStack.Unwind] — the stack is empty until phase-8 / step 16
// lands the executor-side traversal that pushes the chosen branch's compensation entries into it. Once that's wired,
// this body still does the same thing: unwinds whatever the executor populated.
//
// Parameters:
//   - `activation`: the per-dispatch record; supplies the [*op.RuntimeEnvironment] passed to [op.RecoveryStack.Unwind].
//   - `stack`: the [op.RecoveryStack] returned by the forward Choose call.
//
// Returns:
//   - `error`: non-nil if the unwind fails.
func (p *Provider) CompensateChoose(activation *op.ActivationRecord, stack *op.RecoveryStack) error {

	if stack == nil {
		return nil
	}
	return stack.Unwind(activation.RuntimeEnvironment)
}

// Gather runs the activation's subgraph body once per item, concurrently up to `limit`, and nests one stamped substack
// per iteration onto the gather's own recovery stack.
//
// Gather is a quantifier over Subgraph (phase-8 step 28.2): it runs the same body-walk Subgraph runs
// ([walkSubgraphChildren]) N times — once per item — each on its own child stack with a frame that binds `item` to the
// iteration value. The two builtin parameter names, `items` and `limit`, are consumed here and stripped from the
// per-iteration frame ([buildIterationFrame]); bodies read the iteration value via `plan.variable("item")`. Concurrency
// is goroutine-per-item throttled by a `limit`-sized semaphore; each goroutine walks the body on its **own** child
// stack, so `activation.Stack` stays single-writer — stamping and nesting run on the dispatching goroutine, in index
// order.
//
// Each iteration's child stack is [op.RecoveryStack.Stamp]ed with its identity (`"<gatherID>#<i>"`), result, and status,
// then nested onto `activation.Stack` ([op.RecoveryStack.PushNested]). The gather returns that stack as its complement,
// exactly as Subgraph returns `activation.Stack`; `CompensateGather` unwinds it. On resume of a *paused* gather the
// stack comes back carrying the prior iterations' stamped substacks, and Gather classifies each iteration against it
// ([op.RecoveryStack.NestedStackByUnitID]): a completed run replays its stamped result and is skipped, a paused run
// adopts its partial substack and re-enters, a never-run iteration runs fresh.
//
// On any iteration error Gather does not self-unwind: it returns the stamped stack as the complement alongside the
// error, so the executor rolls it back (or, on ErrPaused, checkpoints it) — the same contract as Subgraph.
//
// +devlore:planner=GatherPlanner
//
// Parameters:
//   - `activation`: the per-dispatch record; cancellation flows through `activation.Context` and a scoped child is
//     derived for this gather's iterations. `activation.Variables` is the parent frame the per-iteration frames derive
//     from, `activation.Stack` is the gather's own (resume-adopted) stack, and `activation.Unit` must be a
//     `*op.Subgraph` (the gather's bound unit).
//   - `items`: the list of items to iterate over.
//   - `kwargs`: catchall sink — `limit` (max concurrent iterations; defaults to platform concurrency when non-positive)
//     is read here. Other keys are reserved for future extension.
//
// Returns:
//   - `any`: a []any of terminal results from each iteration, indexed by original item order; nil on error.
//   - *op.RecoveryStack: `activation.Stack`, carrying one stamped substack per iteration in index order.
//   - `error`: non-nil if any iteration failed or paused, or the body is malformed.
func (p *Provider) Gather(
	activation *op.ActivationRecord,
	items []any,
	kwargs map[string]any,
) (any, *op.RecoveryStack, error) {

	// The per-subgraph executor owns this gather's stack and supplies it as activation.Stack — a fresh stack on a first
	// dispatch, or, on resume of a paused gather, the restored stack carrying the prior iterations' stamped substacks.
	// Gather nests one stamped substack per iteration onto it and returns it as its complement, exactly as Subgraph
	// returns activation.Stack.
	stack := activation.Stack

	if len(items) == 0 {
		return []any{}, stack, nil
	}

	limit := 0
	if raw, ok := kwargs["limit"]; ok {
		switch v := raw.(type) {
		case int:
			limit = v
		case int64:
			limit = int(v)
		}
	}
	if limit <= 0 {
		limit = p.RuntimeEnvironment().Platform.DefaultConcurrency()
	}

	subgraph, ok := activation.Unit.(*op.Subgraph)
	if !ok {
		return nil, stack, fmt.Errorf("flow.Gather: activation.Unit is %T, want *op.Subgraph", activation.Unit)
	}

	results := make([]any, len(items))

	// Classify each iteration against the (possibly resume-adopted) stack, mirroring Subgraph.Execute's adopt-guard: a
	// completed run replays its stamped result and is skipped; a paused run adopts its partial substack and re-enters
	// (its own child skip-guards finish it); a never-run iteration gets a fresh child stack.
	type run struct {
		index      int
		childStack *op.RecoveryStack
		fresh      bool
	}
	var pending []run

	for i := range items {
		if prior, found := stack.NestedStackByUnitID(gatherIterationID(activation.Unit, i)); found {
			if prior.Err() == nil {
				results[i] = prior.Result()
				continue
			}
			pending = append(pending, run{index: i, childStack: prior, fresh: false})
			continue
		}
		pending = append(pending, run{index: i, childStack: op.NewChildRecoveryStack(stack), fresh: true})
	}

	if len(pending) == 0 {
		return results, stack, nil
	}

	gatherCtx, gatherCancel := context.WithCancel(activation.Context)
	defer gatherCancel()

	type completion struct {
		run    run
		result any
		err    error
	}

	events := make(chan completion, len(pending))
	sem := make(chan struct{}, limit)

	var wg sync.WaitGroup

	for _, r := range pending {

		wg.Add(1)
		sem <- struct{}{}

		go func(r run) {

			defer wg.Done()
			defer func() { <-sem }()

			// Each iteration walks the body — the same walk Subgraph runs — on its own child stack, so activation.Stack
			// stays single-writer; stamping and nesting happen on this goroutine below, in index order.
			frame := buildIterationFrame(activation.Variables, items[r.index])
			result, runErr := walkSubgraphChildren(activation, gatherCtx, subgraph, r.childStack, frame, nil)

			events <- completion{run: r, result: result, err: runErr}
		}(r)
	}

	byIndex := make(map[int]completion, len(pending))
	var firstErr error

	for range pending {

		c := <-events
		byIndex[c.run.index] = c

		if c.err != nil && firstErr == nil {
			firstErr = c.err
			gatherCancel()
		}
	}

	wg.Wait()

	// Stamp each run's substack and nest the fresh ones, in index order (single-writer). A paused run's substack was
	// adopted in place, so it is re-stamped, not re-nested. On any iteration error the stamped stack is still returned as
	// the complement so the executor rolls it back (or checkpoints it, on ErrPaused) — Gather does not self-unwind.
	for i := range items {
		c, ran := byIndex[i]
		if !ran {
			continue
		}
		c.run.childStack.Stamp(gatherIterationID(activation.Unit, i), c.result, c.err)
		if c.run.fresh {
			stack.PushNested(c.run.childStack)
		}
		results[i] = c.result
	}

	if firstErr != nil {
		return nil, stack, fmt.Errorf("flow.Gather: %w", firstErr)
	}

	return results, stack, nil
}

// CompensateGather unwinds the per-iteration recovery stacks accumulated by a successful Gather.
//
// Called by the executor when the parent stack unwinds and hits gather's compensable entry. The single returned
// [op.RecoveryStack] holds one nested substack per iteration in completion order; [op.RecoveryStack.Unwind] walks the
// entries LIFO so the iteration that finished last (and therefore produced the freshest side effects) undoes first,
// mirroring standard compensation semantics.
//
// Parameters:
//   - `activation`: the per-dispatch record; supplies the [*op.RuntimeEnvironment] passed to [op.RecoveryStack.Unwind].
//   - `stack`: the gather stack as returned by Gather (one nested substack per iteration in completion order).
//
// Returns:
//   - `error`: a joined error across any substack that failed to unwind; nil on total success.
func (p *Provider) CompensateGather(activation *op.ActivationRecord, stack *op.RecoveryStack) error {

	if stack == nil {
		return nil
	}
	return stack.Unwind(activation.RuntimeEnvironment)
}

// Subgraph dispatches the children of a `plan.subgraph(...)` container in declaration order.
//
// Reached from [op.GraphExecutor.executeSubgraph]'s bound-action path: the executor's shim resolves the
// subgraph's slots, builds the [op.ActivationRecord] with the subgraph as `Unit`, installs the
// child-dispatch closure, and calls [op.Action.Do]. This method walks `activation.Unit.(*op.Subgraph)
// .Children()` and dispatches each child via [op.ActivationRecord.DispatchChild], which routes through
// the parent executor (preserving observability hooks, the resolved variable map, and the active
// results map for promise resolution).
//
// Per-child retry policy: the child's own [op.ExecutableUnit.RetryPolicy] drives retry attempts (interim;
// the frame-chain `effectiveRetryPolicy` helper is pending). Nil policy means one attempt with no retry.
// Delays between retries are computed via [op.RetryPolicy.ComputeDelay]; cooperative cancellation via
// `activation.Context` aborts the wait.
//
// Failure handling: when a child exhausts its retries, the subgraph's [op.Subgraph.ErrorAction]
// (if non-nil) is dispatched against the subgraph-local stack as a single best-effort observation
// pass. Whether the errorAction succeeds or fails, the original child error surfaces — errorAction is
// an observation hook, not a recovery path. The default-sentinel fallback to [flow.Provider.Failed]
// when ErrorAction is nil is deferred.
//
// `items` iteration is not yet implemented; passing a non-empty `items=` to `plan.subgraph(...)` is
// an error today. The pure-container shape (children walk only) is what this method supports.
//
// +devlore:planner=SubgraphPlanner
//
// Parameters:
//   - `activation`: the per-dispatch [*op.ActivationRecord] the executor built. `activation.Unit` must
//     type-assert to [*op.Subgraph]; `activation.DispatchChild` must be installed (both invariants are
//     the executor's contract on the bound-action path).
//   - `items`: the resolved value of the `items=` kwarg from `plan.subgraph(items=[...], body=[...])`.
//     Must be empty for now.
//   - `kwargs`: frame-binding kwargs (every `plan.subgraph(...)` kwarg except the slot/parameter
//     names declared on this method). Read by children that reference them via `plan.variable(name)`;
//     this method does not consume them directly.
//
// Returns:
//   - `any`: the last child's terminal result, mirroring the structural-container contract in
//     [op.Subgraph.Execute] — the leaf unit's output bubbles up through the subgraph to the parent.
//     Nil for a zero-child subgraph. Children's results also flow into the parent results map via
//     [op.ActivationRecord.DispatchChild].
//   - *op.RecoveryStack: the subgraph-local saga stack. Children's compensations accumulated here
//     via the installed `DispatchChild` closure; the executor pushes this nested onto the parent
//     stack as the subgraph's complement.
//   - `error`: non-nil on (a) `items` iteration request, (b) `activation.Unit` not a *op.Subgraph,
//     (c) any child's exhausted-retry failure (with the original child error wrapped).
func (p *Provider) Subgraph(
	activation *op.ActivationRecord,
	kwargs map[string]any,
) (any, *op.RecoveryStack, error) {

	subgraph, ok := activation.Unit.(*op.Subgraph)
	if !ok {
		return nil, nil, fmt.Errorf("flow.Subgraph: activation.Unit is %T, want *op.Subgraph", activation.Unit)
	}

	// Bind kwargs to the subgraph's parameters, layered onto the inherited variable frame: each kwarg whose name
	// matches a declared parameter sets that parameter's value for the children's slot resolution.
	parameters, err := subgraph.Parameters()
	if err != nil {
		return nil, nil, fmt.Errorf("flow.Subgraph: %w", err)
	}
	frame := make(map[string]op.Variable, len(activation.Variables)+len(parameters))
	for name, variable := range activation.Variables {
		frame[name] = variable
	}
	for _, parameter := range parameters {
		if value, present := kwargs[parameter.Name]; present {
			frame[parameter.Name] = op.Variable{Name: parameter.Name, Value: value}
		}
	}

	// The per-subgraph executor owns this subgraph's recovery stack and supplies it as activation.Stack (phase-8 step
	// 28). Children's receipts accumulate on it, and it is returned as this action's complement so CompensateSubgraph
	// can unwind it.
	stack := activation.Stack

	lastResult, err := walkSubgraphChildren(
		activation,
		activation.Context,
		subgraph,
		stack,
		frame,
		subgraph.ErrorAction())
	if err != nil {
		return nil, stack, fmt.Errorf("flow.Subgraph: %w", err)
	}

	return lastResult, stack, nil
}

// CompensateSubgraph unwinds the subgraph's local saga stack as a single transactional unit.
//
// The stack carries one entry per compensable child (a Receipt or a deeper nested substack). Unwind walks
// LIFO and dispatches per entry kind, recursing into nested substacks. Until phase-8 / step 16 wires the
// executor-side population, the stack is empty and Unwind is a no-op.
//
// Parameters:
//   - `activation`: the per-dispatch record; supplies the [*op.RuntimeEnvironment] passed to [op.RecoveryStack.Unwind].
//   - `stack`: the [op.RecoveryStack] returned by the forward Subgraph call.
//
// Returns:
//   - `error`: non-nil if any entry fails to unwind.
func (p *Provider) CompensateSubgraph(activation *op.ActivationRecord, stack *op.RecoveryStack) error {

	if stack == nil {
		return nil
	}

	return stack.Unwind(activation.RuntimeEnvironment)
}

// Fallible actions

// Failed halts graph execution immediately.
//
// Parameters:
//   - `format`: format string.
//   - `args`: positional format arguments.
//   - `kwargs`: keyword arguments for template rendering.
//
// Returns:
//   - `error`: always non-nil FatalError.
func (p *Provider) Failed(format string, args []any, kwargs map[string]any) error {

	return &op.FatalError{Message: op.RenderError(format, args, kwargs).Error()}
}

// WaitUntil polls a predicate at the configured interval until it returns true or the timeout expires.
//
// +devlore:planner=WaitUntilPlanner
//
// Parameters:
//   - `target`: the value to evaluate the predicate against.
//   - `predicate`: condition to evaluate.
//   - `timeout`: maximum wait time.
//   - `interval`: poll interval (default 5s).
//
// Returns:
//   - `any`: the target value when the predicate returns true.
//   - `error`: non-nil if the timeout expires or the predicate fails.
func (p *Provider) WaitUntil(
	target any,
	predicate func(any) (bool, error),
	timeout, interval time.Duration,
) (any, error) {

	if timeout == 0 {
		return nil, fmt.Errorf("wait_until: timeout is required")
	}
	if interval == 0 {
		interval = 5 * time.Second
	}

	matched, err := predicate(target)

	if err != nil {
		return nil, fmt.Errorf("wait_until: predicate error: %w", err)
	}
	if matched {
		return target, nil
	}

	runtimeEnvironment := p.RuntimeEnvironment()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-runtimeEnvironment.Context.Done():
			return nil, runtimeEnvironment.Context.Err()
		case <-deadline.C:
			return nil, fmt.Errorf("wait_until: timeout after %s", timeout)
		case <-ticker.C:
			matched, err := predicate(target)
			if err != nil {
				return nil, fmt.Errorf("wait_until: predicate error: %w", err)
			}
			if matched {
				return target, nil
			}
		}
	}
}

// Actions

// Complete is the default, healthy conclusion of a graph path.
//
// +devlore:defaults output=nil
//
// Parameters:
//   - `output`: optional output value.
//
// Returns:
//   - `any`: the output value.
func (p *Provider) Complete(output any) any {
	return output
}

// Degraded marks a branch as non-optimal while allowing graph execution to continue.
//
// Parameters:
//   - `format`: format string.
//   - `args`: positional format arguments.
//   - `kwargs`: keyword arguments for template rendering.
//
// Returns:
//   - `string`: the rendered warning message.
func (p *Provider) Degraded(format string, args []any, kwargs map[string]any) string {
	rendered := op.RenderError(format, args, kwargs)
	_, _ = fmt.Fprintln(os.Stderr, "degraded:", rendered)
	return rendered.Error()
}

// endregion

// endregion
