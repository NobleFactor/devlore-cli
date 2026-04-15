// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package flow implements flow-control methods for execution graphs.
//
// Its methods are dispatched during graph execution — they are actions, not modules. The Provider holds a graph and
// operates on it directly, the same way the plan provider does.
package flow

import (
	"fmt"
	"os"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var _ op.Provider = (*Provider)(nil) // Interface Guard

// Provider implements flow-control actions for execution graphs.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
	Graph *op.Graph
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

// Elevate marks the boundary between unprivileged and privileged execution.
func (p *Provider) Elevate() {
}

// Choose reads a boolean condition and executes the matching branch subgraph on the graph.
//
// Parameters:
//   - when: condition value.
//   - then: subgraph ID to execute when true.
//
// Returns:
//   - any: the selected branch's terminal result.
//   - error: non-nil if the branch fails.
func (p *Provider) Choose(when bool, then string) (any, error) {

	if !when || then == "" {
		return nil, nil
	}

	sg := p.Graph.SubgraphByID(then)
	if sg == nil {
		return nil, fmt.Errorf("choose: subgraph %q not found", then)
	}

	return p.executeSubgraph(sg)
}

// Gather executes a subgraph body once per item, collecting terminal results.
//
// Parameters:
//   - items: the list of items to iterate over.
//   - do: subgraph ID of the body to execute per item.
//   - limit: max concurrent iterations (default 1 = sequential).
//
// Returns:
//   - []any: terminal node result from each iteration, in item order.
//   - error: non-nil if any iteration fails.
func (p *Provider) Gather(items []any, do string, limit int) ([]any, error) {

	if len(items) == 0 {
		return []any{}, nil
	}

	sg := p.Graph.SubgraphByID(do)
	if sg == nil {
		return nil, fmt.Errorf("gather: subgraph %q not found", do)
	}

	// TODO: execute subgraph body per item with concurrency limit
	_ = limit
	_ = sg
	return nil, fmt.Errorf("gather: not yet implemented")
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

// region UNEXPORTED METHODS

// executeSubgraph runs all nodes in a subgraph sequentially on the graph.
func (p *Provider) executeSubgraph(sg *op.Subgraph) (any, error) {

	return p.ExecutionContext().ExecuteSubgraph(p.Graph, sg)
}

// endregion
