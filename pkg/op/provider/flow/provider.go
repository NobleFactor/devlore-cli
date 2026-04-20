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
	Graph *op.Graph
}

type Case struct {
	When func() bool
	Then func() any
}

// NewProvider creates a flow Provider bound to the given context.
//
// The graph is extracted from ctx.Data["graph"].
func NewProvider(ctx *op.ExecutionContext) *Provider {
	var graph *op.Graph
	if g, ok := ctx.Data["graph"].(*op.Graph); ok {
		graph = g
	}
	return &Provider{
		ProviderBase: op.NewProviderBase(ctx),
		Graph:        graph,
	}
}

// region EXPORTED METHODS

// Choose reads a boolean condition and executes the matching branch subgraph on the graph.
//
// Parameters:
//   - when: condition value.
//   - then: subgraph ID to execute when true.
//
// Returns:
//   - any: the selected branch's terminal result.
//   - error: non-nil if the branch fails.
func (p *Provider) Choose(defaultValue func() any, cases ...Case) any {

	for _, c := range cases {
		if c.When != nil && c.When() {
			if c.Then != nil {
				return c.Then()
			}
			return nil
		}
	}

	return defaultValue()
}

// Complete is the default, healthy conclusion of a graph path.
//
// Parameters:
//   - output: optional output value.
//
// Returns:
//   - any: the output value.
func (p *Provider) Complete(output any) any {
	return output
}

// Degraded marks a branch as non-optimal while allowing graph execution to continue.
//
// Parameters:
//   - format: format string.
//   - args: positional format arguments.
//   - kwargs: keyword arguments for template rendering.
//
// Returns:
//   - string: the rendered warning message.
func (p *Provider) Degraded(format string, args []any, kwargs map[string]any) string {
	rendered := op.RenderError(format, args, kwargs)
	_, _ = fmt.Fprintln(os.Stderr, "degraded:", rendered)
	return rendered.Error()
}

// Elevate marks the boundary between unprivileged and privileged execution.
func (p *Provider) Elevate() {
}

// Fatal halts graph execution immediately.
//
// Parameters:
//   - format: format string.
//   - args: positional format arguments.
//   - kwargs: keyword arguments for template rendering.
//
// Returns:
//   - error: always non-nil FatalError.
func (p *Provider) Fatal(format string, args []any, kwargs map[string]any) error {
	return &op.FatalError{Message: op.RenderError(format, args, kwargs).Error()}
}

// Gather executes a subgraph body once per item, collecting terminal results across concurrent iterations.
//
// Gather is a compensable method. On total success it returns the per-iteration recovery stacks (in completion
// order) as its complement; the executor's PushAction wraps them into a single entry on the parent stack so a
// later parent-level failure unwinds every iteration's work in reverse completion order via CompensateGather.
//
// On any iteration failure gather cancels its scoped ctx to signal the other iterations to bail at their next
// node, waits for all iterations to finish, unwinds the locally-held stacks, and returns (nil, nil, err) so no
// residue lands on the parent stack.
//
// Cancellation scope is derived as a child of ctx: a cancel on ctx (root or ancestor gather) propagates down to
// iterations, while a cancel on the derived gatherCtx stays scoped to this gather's iterations only.
//
// Parameters:
//   - ctx: the ambient cancellation context; a scoped child is derived for this gather's iterations.
//   - items: the list of items to iterate over.
//   - do: subgraph or node ID of the body to execute per item.
//   - limit: max concurrent iterations; defaults to the platform concurrency when non-positive.
//
// Returns:
//   - []any: terminal result from each iteration, indexed by original item order; nil on failure.
//   - op.Complement: []*op.RecoveryStack in completion order on success; nil on failure.
//   - error: non-nil if any iteration failed or the body is malformed.
func (p *Provider) Gather(ctx context.Context, items []any, do string, limit int) ([]any, op.Complement, error) {

	if len(items) == 0 {
		return []any{}, nil, nil
	}

	if limit <= 0 {
		limit = p.ExecutionContext().Platform.DefaultConcurrency
	}

	body, err := p.Graph.ResolveExecutable(do)

	if err != nil {
		return nil, nil, fmt.Errorf("gather: %w", err)
	}

	params := body.Parameters()

	if len(params) != 1 {
		return nil, nil, fmt.Errorf("gather: body %q must declare exactly one parameter; got %d", do, len(params))
	}

	inputName := params[0].Name

	gatherCtx, gatherCancel := context.WithCancel(ctx)
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

			r, runErr := p.Graph.ExecuteWithStack(gatherCtx, body, iterStack, map[string]op.SlotValue{
				inputName: op.ImmediateValue{Value: item},
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

	results := make([]any, len(items))
	stacks := make([]*op.RecoveryStack, len(completed))

	for i, c := range completed {
		results[c.index] = c.result
		stacks[i] = c.stack
	}

	return results, stacks, nil
}

// CompensateGather unwinds the per-iteration recovery stacks accumulated by a successful Gather.
//
// Called by the executor when the parent stack unwinds and hits gather's compensable entry. Stacks are unwound in
// reverse completion order — the iteration that finished last (and therefore produced the freshest side effects)
// undoes first, mirroring standard LIFO compensation semantics.
//
// Parameters:
//   - stacks: the iteration stacks as returned by Gather, in completion order.
//
// Returns:
//   - error: a joined error across any stack that failed to unwind; nil on total success.
func (p *Provider) CompensateGather(stacks []*op.RecoveryStack) error {

	var errs []error

	for i := len(stacks) - 1; i >= 0; i-- {
		if err := stacks[i].Unwind(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Subgraph bundles a set of detached invocations into one executable unit.
//
// Surfaces in starlark as plan.subgraph(...) because flow is a root-planned provider (phase-8 D12). The variadic
// children are detached invocations from prior plan.* calls; step 11's target-type dispatch fills the slot with
// each child's structural reference (an [op.ExecutableUnit]) rather than a value-side promise. The container's
// output is the list of terminal values produced by the children in topological order, typed []any per D3.
//
// Empty subgraphs are a plan-time error per D10 — enforced at plan.run materialization (step 16), not in this
// method body. plan.run also walks the children to materialize the structural [op.Subgraph] in the executable
// graph; this method is the codegen-discoverable signature that defines the surface, not the runtime executor of
// the subgraph itself (which is handled by the [op.Graph] dispatcher once materialization completes).
//
// Parameters:
//   - children: the variadic invocations bundled into this subgraph.
//
// Returns:
//   - []any: the list of terminal values, in topological order.
func (p *Provider) Subgraph(children ...op.ExecutableUnit) []any {

	results := make([]any, 0, len(children))
	for range children {
		results = append(results, nil)
	}
	return results
}

// WaitUntil polls a predicate at the configured interval until it returns true or the timeout expires.
//
// Parameters:
//   - target: the value to evaluate the predicate against.
//   - predicate: condition to evaluate.
//   - timeout: maximum wait time.
//   - interval: poll interval (default 5s).
//
// Returns:
//   - any: the target value when the predicate returns true.
//   - error: non-nil if the timeout expires or the predicate fails.
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

	ctx := p.ExecutionContext()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
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
