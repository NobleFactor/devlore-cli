// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// gatherComplement preserves per-iteration state needed for rollback.
type gatherComplement struct {
	Iterations []iterationUndo
}

// iterationUndo captures one gather iteration's undo state.
type iterationUndo struct {
	ProxyCtx map[string]any    // {gatherID: item} for slot re-resolution
	Results  map[string]any    // node results for promise re-resolution
	Stack    *op.RecoveryStack // per-iteration compensation closures
}

// iterOutcome captures the result of a single gather iteration.
type iterOutcome struct {
	result any
	undo   iterationUndo
	err    error
}

// Gather is a parallel comprehension flow action. It executes a phase body
// once per item with configurable concurrency, collecting terminal results.
//
// Slots:
//   - items: []any — the list of items to iterate over
//   - do: string — phase ID of the body to execute per item
//   - limit: int — max concurrent iterations (default 1 = sequential)
//
// Result: []any — terminal node Result from each iteration, in item order.
// Complement: *gatherComplement — per-iteration entries for rollback.
type Gather struct{}

// Name returns the dotted action name.
func (a *Gather) Name() string { return "flow.gather" }

// Params returns nil — Gather uses untyped slots.
func (a *Gather) Params() []op.ParamInfo { return nil }

// Do executes the referenced phase once per item, with per-iteration isolation.
func (a *Gather) Do(ctx *op.Context, slots map[string]any) (result op.Result, complement op.Complement, err error) { //nolint:gocyclo // validation + sequential/concurrent branching is inherent
	items, err := extractItems(slots)
	if err != nil {
		return nil, nil, err
	}
	if len(items) == 0 {
		return []any{}, &gatherComplement{}, nil
	}

	phaseID, ok := slots["do"].(string)
	if !ok || phaseID == "" {
		return nil, nil, fmt.Errorf("gather: missing or invalid 'do' slot (phase ID)")
	}

	limit := extractLimit(slots)

	graph := ctx.Graph
	if graph == nil {
		return nil, nil, fmt.Errorf("gather: no graph in context")
	}
	phase := graph.PhaseByID(phaseID)
	if phase == nil {
		return nil, nil, fmt.Errorf("gather: phase %q not found", phaseID)
	}

	phaseNodes, phaseEdges := graph.CollectPhaseNodes(phase)
	ordered := execution.OrderNodes(phaseNodes, phaseEdges)

	gatherID := ctx.NodeID

	outcomes := make([]iterOutcome, len(items))

	if limit <= 1 || len(items) <= 1 {
		// Sequential execution.
		for i, item := range items {
			outcomes[i] = executeIteration(ctx, ordered, gatherID, item)
			if outcomes[i].err != nil {
				break
			}
		}
	} else {
		executeConcurrent(ctx, ordered, gatherID, items, limit, outcomes)
	}

	// Collect results and build complement.
	var results []any
	gc := &gatherComplement{}
	var gatherErr error

	for i, o := range outcomes {
		gc.Iterations = append(gc.Iterations, o.undo)
		if o.err != nil && gatherErr == nil {
			gatherErr = fmt.Errorf("gather: iteration %d failed: %w", i, o.err)
		}
	}

	if gatherErr != nil {
		// Unwind all completed iterations.
		_ = a.undoCompleted(ctx, gc) //nolint:errcheck // compensation is best-effort during error unwind
		return nil, nil, gatherErr
	}

	for _, o := range outcomes {
		results = append(results, o.result)
	}

	return results, gc, nil
}

// Undo walks iterations in reverse and calls Action.Undo per entry.
func (a *Gather) Undo(ctx *op.Context, complement op.Complement) error {
	gs, ok := complement.(*gatherComplement)
	if !ok || gs == nil {
		return nil
	}
	return a.undoCompleted(ctx, gs)
}

// undoCompleted unwinds all iterations that have recovery stacks.
func (a *Gather) undoCompleted(_ *op.Context, gs *gatherComplement) error {
	var errs []error
	for i := len(gs.Iterations) - 1; i >= 0; i-- {
		if stack := gs.Iterations[i].Stack; stack != nil {
			if err := stack.Unwind(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// executeConcurrent runs gather iterations with bounded concurrency.
func executeConcurrent(ctx *op.Context, ordered []*op.Node, gatherID string, items []any, limit int, outcomes []iterOutcome) {
	iterCtxBase, cancel := context.WithCancel(ctx.Context)
	defer cancel()

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var firstErr error
	var mu sync.Mutex

iterLoop:
	for i, item := range items {
		// Check for cancellation before starting a new iteration.
		select {
		case <-iterCtxBase.Done():
			break iterLoop
		default:
		}
		if iterCtxBase.Err() != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, val any) {
			defer wg.Done()
			defer func() { <-sem }()

			// Per-iteration context with its own mutable fields.
			iterCtx := &op.Context{
				Context: iterCtxBase,
				DryRun:  ctx.DryRun,
				Writer:  ctx.Writer,
				Data:    ctx.Data,
				Graph:   ctx.Graph,
				NodeID:  ctx.NodeID,
			}

			outcomes[idx] = executeIteration(iterCtx, ordered, gatherID, val)
			if outcomes[idx].err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = outcomes[idx].err
				}
				mu.Unlock()
				cancel()
			}
		}(i, item)
	}
	wg.Wait()
}

// executeIteration runs the phase body for a single item.
func executeIteration(ctx *op.Context, ordered []*op.Node, gatherID string, item any) iterOutcome {
	results := make(map[string]any)
	stack := op.NewRecoveryStack()
	proxyCtx := map[string]any{gatherID: item}

	for _, node := range ordered {
		if node.Action == nil {
			continue
		}

		nodeSlots := node.ResolvedSlots(results, proxyCtx)
		execution.FillSlotsFromData(nodeSlots, ctx.Data)

		result, complement, err := node.Action.Do(ctx, nodeSlots)
		if err != nil {
			// Unwind this iteration's completed nodes.
			_ = stack.Unwind()
			return iterOutcome{
				undo: iterationUndo{
					ProxyCtx: proxyCtx,
					Results:  results,
					Stack:    stack,
				},
				err: err,
			}
		}

		if result != nil {
			results[node.ID] = result
		}
		stack.PushAction(ctx, node.Action, complement)
	}

	// Terminal result is the last ordered node's result.
	var terminalResult any
	if len(ordered) > 0 {
		terminalResult = results[ordered[len(ordered)-1].ID]
	}

	return iterOutcome{
		result: terminalResult,
		undo: iterationUndo{
			ProxyCtx: proxyCtx,
			Results:  results,
			Stack:    stack,
		},
	}
}

// extractItems pulls the items list from slots.
func extractItems(slots map[string]any) ([]any, error) {
	raw, ok := slots["items"]
	if !ok {
		return nil, fmt.Errorf("gather: missing 'items' slot")
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("gather: 'items' slot must be []any, got %T", raw)
	}
	return items, nil
}

// extractLimit pulls the concurrency limit from slots (default 1).
func extractLimit(slots map[string]any) int {
	if v, ok := slots["limit"]; ok {
		switch n := v.(type) {
		case int:
			if n > 0 {
				return n
			}
		case float64:
			if int(n) > 0 {
				return int(n)
			}
		}
	}
	return 1
}
