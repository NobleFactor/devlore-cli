// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package flow implements flow-control methods for execution graphs.
//
// Its methods are dispatched during graph execution — they are actions, not modules. The Provider holds a graph and
// operates on it directly, the same way the plan provider does.
package flow

import (
	"context"
	"errors"
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

// Case is one branch of a [Provider.Choose] dispatch.
//
// Both fields are typed any to accept the variety of values plan.choose's branches handle: literal scalars, resolved
// values, or detached invocations from prior plan.* calls. The structural materialization at plan.run (step 16) and the
// executor's `choose` dispatch resolve the values; this type is pure data.
//
// Constructed by plan.case(when=..., then=...) (an immediate method on plan.Provider) and passed as a variadic argument
// to plan.choose.
type Case struct {
	When any // condition the branch tests against (literal, value, or invocation reference)
	Then any // body the branch produces if When is truthy (literal, value, or invocation reference)
}

// NewProvider creates a flow Provider bound to the given context.
//
// The graph reference is not captured at construction — flow methods read it per dispatch from
// [op.ActivationRecord.Graph], stamped by the executor when the activation is built.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {

	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// region EXPORTED METHODS

// Choose walks the cases in declaration order, resolving each case's When and yielding the first branch whose When is
// truthy. Once a match is found, only that case's Then is evaluated; remaining cases are short-circuited (their When
// and Then arguments are never resolved). If no case matches, `defaultValue` is returned.
//
// Surfaces in starlark as plan.choose(default_value, plan.case(when=..., then=...), ...) because flow is a root-planned
// provider (phase-8 D12). Branches are detached by default per D5 — each plan.case is a pure data container constructed
// by plan.case(...) and passed by value; the When and Then fields hold whatever the starlark author supplied (literal
// scalar, op.Resource, or *starlarkbridge.Invocation reference), which the executor's `choose` dispatch resolves at
// execution time. This method is the codegen-discoverable signature; the structural materialization (lazy branch
// dispatch via [op.Graph.ExecuteWithStack]) is wired by plan.run (step 16).
//
// Truthiness rule (the executor's contract; encoded here as the stub fallback):
//
//   - `bool`: true is truthy.
//   - `integer`: zero is falsy, others truthy.
//   - `string`: empty is falsy, others truthy.
//   - `nil`: falsy.
//   - Anything else (op.Resource, non-nil pointer, etc.): truthy.
//
// When matches starlark.Value.Truth() for native starlark types. When a Case's When is a *starlarkbridge.Invocation
// reference, the executor dispatches the When invocation and applies the truthiness rule to its resolved value.
//
// Compensable per the [op.Method] convention: returns (result, complement, error). The complement is the recovery state
// of whichever branch actually ran, so [Provider.CompensateChoose] can unwind it on a later parent-level failure.
//
// Container output type per D3: T when defaultValue and every case's Then are homogeneous, any otherwise. Go can't
// express the homogeneous case statically; the return type is any.
//
// Parameters:
//   - `defaultValue`: the value used when no `case`'s When is truthy.
//   - `cases`: the variadic cases to evaluate in declaration order.
//
// Returns:
//   - `any`: the chosen branch's value (or defaultValue when no case matches).
//   - `*op.RecoveryStack`: the recovery state of the executed branch, for [Provider.CompensateChoose]. Currently, an
//     empty stack — the chosen branch's actual compensation is collected by the executor's traversal of the
//     materialized op.Choose node, not by this method body. Phase-8 / step 13's plan.choose redesign reshapes the
//     runtime semantics; phase-8 / step 16 (plan.run + executor.op.Choose handling) wires the local-stack splice. The
//     empty stack here keeps the saga-shape contract intact in the meantime.
//   - `error`: non-nil if branch evaluation fails.
func (p *Provider) Choose(defaultCase any, cases ...Case) (any, *op.RecoveryStack, error) {

	for _, c := range cases {
		if isTruthy(c.When) {
			return c.Then, op.NewRecoveryStack(), nil
		}
	}

	return defaultCase, op.NewRecoveryStack(), nil
}

// CompensateChoose unwinds the recovery state captured by a successful [Provider.Choose] call.
//
// Today this is structurally a delegation to [op.RecoveryStack.Unwind] — the stack is empty until phase-8 / step 16
// lands the executor-side traversal that pushes the chosen branch's compensation entries into it. Once that's wired,
// this body still does the same thing: unwinds whatever the executor populated.
//
// Parameters:
//   - `stack`: the [op.RecoveryStack] returned by the forward Choose call.
//
// Returns:
//   - `error`: non-nil if the unwind fails.
func (p *Provider) CompensateChoose(stack *op.RecoveryStack) error {

	if stack == nil {
		return nil
	}
	return stack.Unwind()
}

// Complete is the default, healthy conclusion of a graph path.
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

// Elevate marks the boundary between unprivileged and privileged execution.
func (p *Provider) Elevate() {
}

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

// Gather executes a subgraph body once per item, collecting terminal results across concurrent iterations.
//
// Gather is a compensable method. On total success it returns the per-iteration recovery stacks (in completion
// order) as its complement; the executor calls [op.RecoveryStack.PushComplement] to nest them onto the parent
// stack so a later parent-level failure unwinds every iteration's work in reverse completion order via
// CompensateGather.
//
// On any iteration failure gather cancels its scoped ctx to signal the other iterations to bail at their next
// node, waits for all iterations to finish, unwinds the locally held stacks, and returns (nil, nil, err) so no
// residue lands on the parent stack.
//
// Cancellation scope is derived as a child of ctx: a cancel on ctx (root or ancestor gather) propagates down to
// iterations, while a cancel on the derived gatherCtx stays scoped to this gather's iterations only.
//
// Parameters:
//   - `activationRecord`: the per-dispatch record; cancellation flows through `activationRecord.Context` and a
//     scoped child is derived for this gather's iterations.
//   - `items`: the list of items to iterate over.
//   - `do`: subgraph or node ID of the body to execute per item.
//   - `limit`: max concurrent iterations; defaults to the platform concurrency when non-positive.
//
// Returns:
//   - `[]any`: terminal result from each iteration, indexed by original item order; nil on failure.
//   - `*op.RecoveryStack`: a single stack containing the per-iteration substacks in completion order via
//     [op.RecoveryStack.PushNested]. On failure, returns nil.
//   - `error`: non-nil if any iteration failed or the body is malformed.
func (p *Provider) Gather(activationRecord *op.ActivationRecord, items []any, do string, limit int) ([]any, *op.RecoveryStack, error) {

	if len(items) == 0 {
		return []any{}, nil, nil
	}

	if limit <= 0 {
		limit = p.RuntimeEnvironment().Platform.DefaultConcurrency()
	}

	graph := activationRecord.Graph
	if graph == nil {
		return nil, nil, fmt.Errorf("gather: dispatch has no graph in scope")
	}

	body, err := graph.ResolveExecutable(do)

	if err != nil {
		return nil, nil, fmt.Errorf("gather: %w", err)
	}

	// Per-iteration frame binding name is fixed. Bodies that need the iteration value reference
	// `plan.variable("item")`; the executor resolves it against the per-iteration frame. The body
	// may carry any number of bubble-up variables — gather only contributes the one named `item`;
	// the rest resolve up the frame chain to the gather's enclosing scope.
	const iterationVariable = "item"

	gatherCtx, gatherCancel := context.WithCancel(activationRecord.Context)
	defer gatherCancel()

	type completion struct {
		index  int
		result any
		stack  *op.RecoveryStack
		err    error
	}

	events := make(chan completion, len(items))
	sem := make(chan struct{}, limit)

	var wg sync.WaitGroup

	for i, item := range items {

		wg.Add(1)
		sem <- struct{}{}

		go func() {

			defer wg.Done()
			defer func() { <-sem }()

			iterStack := op.NewRecoveryStack()

			r, runErr := graph.ExecuteWithStack(gatherCtx, body, iterStack, map[string]op.SlotValue{
				iterationVariable: op.ImmediateValue{Value: item},
			})

			events <- completion{index: i, result: r, stack: iterStack, err: runErr}
		}()
	}

	completed := make([]completion, 0, len(items))
	var firstErr error

	for range items {

		c := <-events
		completed = append(completed, c)

		if c.err != nil && firstErr == nil {
			firstErr = c.err
			gatherCancel()
		}
	}

	wg.Wait()

	if firstErr != nil {

		var unwindErrs []error

		for i := len(completed) - 1; i >= 0; i-- {
			if err := completed[i].stack.Unwind(); err != nil {
				unwindErrs = append(unwindErrs, err)
			}
		}

		if len(unwindErrs) > 0 {
			return nil, nil, fmt.Errorf("gather: %w; compensation: %w", firstErr, errors.Join(unwindErrs...))
		}

		return nil, nil, fmt.Errorf("gather: %w", firstErr)
	}

	gathered := op.NewRecoveryStack()
	results := make([]any, len(items))

	for _, c := range completed {
		results[c.index] = c.result
		gathered.PushNested(c.stack)
	}

	return results, gathered, nil
}

// CompensateGather unwinds the per-iteration recovery stacks accumulated by a successful Gather.
//
// Called by the executor when the parent stack unwinds and hits gather's compensable entry. The single returned
// [op.RecoveryStack] holds one nested substack per iteration in completion order; [op.RecoveryStack.Unwind] walks the
// entries LIFO so the iteration that finished last (and therefore produced the freshest side effects) undoes first,
// mirroring standard compensation semantics.
//
// Parameters:
//   - `stack`: the gather stack as returned by Gather (one nested substack per iteration in completion order).
//
// Returns:
//   - `error`: a joined error across any substack that failed to unwind; nil on total success.
func (p *Provider) CompensateGather(stack *op.RecoveryStack) error {

	if stack == nil {
		return nil
	}
	return stack.Unwind()
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
//   - `any`: nil. The container has no terminal output of its own; children's results flow into the
//     parent results map via [op.ActivationRecord.DispatchChild].
//   - `*op.RecoveryStack`: the subgraph-local saga stack. Children's compensations accumulated here
//     via the installed `DispatchChild` closure; the executor pushes this nested onto the parent
//     stack as the subgraph's complement.
//   - `error`: non-nil on (a) `items` iteration request, (b) `activation.Unit` not a `*op.Subgraph`,
//     (c) any child's exhausted-retry failure (with the original child error wrapped).
func (p *Provider) Subgraph(activation *op.ActivationRecord, items []any, kwargs map[string]any) (any, *op.RecoveryStack, error) {

	_ = kwargs

	if len(items) > 0 {
		return nil, nil, fmt.Errorf("flow.Subgraph: items iteration not yet implemented")
	}

	subgraph, ok := activation.Unit.(*op.Subgraph)
	if !ok {
		return nil, nil, fmt.Errorf("flow.Subgraph: activation.Unit is %T, want *op.Subgraph", activation.Unit)
	}

	stack := op.NewRecoveryStack()

	for _, child := range subgraph.Children() {

		if err := dispatchWithRetry(activation, child, stack); err != nil {

			if errorAction := subgraph.ErrorAction(); errorAction != nil {
				_, _ = activation.DispatchChild(activation.Context, errorAction, stack, nil)
			}
			return nil, stack, fmt.Errorf("flow.Subgraph: child %q: %w", child.ID(), err)
		}
	}

	return nil, stack, nil
}

// CompensateSubgraph unwinds the subgraph's local saga stack as a single transactional unit.
//
// The stack carries one entry per compensable child (a Receipt or a deeper nested substack). Unwind walks
// LIFO and dispatches per entry kind, recursing into nested substacks. Until phase-8 / step 16 wires the
// executor-side population, the stack is empty and Unwind is a no-op.
//
// Parameters:
//   - `stack`: the [op.RecoveryStack] returned by the forward Subgraph call.
//
// Returns:
//   - `error`: non-nil if any entry fails to unwind.
func (p *Provider) CompensateSubgraph(stack *op.RecoveryStack) error {

	if stack == nil {
		return nil
	}
	return stack.Unwind()
}

// WaitUntil polls a predicate at the configured interval until it returns true or the timeout expires.
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
func (p *Provider) WaitUntil(target any, predicate func(any) (bool, error), timeout, interval time.Duration) (any, error) {

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

	ctx := p.RuntimeEnvironment()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Context.Done():
			return nil, ctx.Context.Err()
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

// endregion

// endregion

// region UNEXPORTED FUNCTIONS

// dispatchWithRetry dispatches a child through [op.ActivationRecord.DispatchChild], retrying per
// the child's own [op.RetryPolicy] until it succeeds, the policy's MaxAttempts is exhausted, or the
// activation's context is cancelled.
//
// Interim implementation: reads `child.RetryPolicy()` directly (no frame-chain effective-policy walk
// yet). A nil policy means one attempt with no retry; a non-nil policy with MaxAttempts == 0 is
// treated as the explicit opt-out (one attempt, terminates any future frame-chain walk).
//
// Backoff: between attempts, `policy.ComputeDelay(prevAttempt)` is honored. The wait is interruptible
// via `activation.Context` — a cancel returns `ctx.Err()` immediately rather than completing the
// delay.
//
// Parameters:
//   - `activation`: the per-dispatch record carrying the cancellation context and the
//     child-dispatch closure.
//   - `child`: the unit to dispatch (with retry).
//   - `stack`: the subgraph-local recovery stack that the child's compensations push onto.
//
// Returns:
//   - `error`: nil when the child succeeds within its retry budget; otherwise the last failure from
//     the child (or `activation.Context.Err()` if cancelled mid-backoff).
func dispatchWithRetry(activation *op.ActivationRecord, child op.ExecutableUnit, stack *op.RecoveryStack) error {

	policy := child.RetryPolicy()

	maxAttempts := 1
	if policy != nil {
		maxAttempts = policy.MaxAttempts + 1
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {

		if attempt > 0 && policy != nil {
			delay := policy.ComputeDelay(attempt - 1)
			if delay > 0 {
				select {
				case <-time.After(delay):
				case <-activation.Context.Done():
					return activation.Context.Err()
				}
			}
		}

		_, err := activation.DispatchChild(activation.Context, child, stack, nil)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return lastErr
}

// endregion
