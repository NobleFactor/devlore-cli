// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Gather is a parallel comprehension flow action. It executes a phase body
// once per item with configurable concurrency, collecting terminal results.
//
// Slots:
//   - items: []any — the list of items to iterate over
//   - do: string — phase ID of the body to execute per item
//   - limit: int — max concurrent iterations (default 1 = sequential)
//
// Result: []any — terminal node Result from each iteration, in item order.
// UndoState: *GatherUndoState — per-iteration entries for rollback.
type Gather struct{}

// Name returns the dotted action name.
func (a *Gather) Name() string { return "flow.gather" }

// Do executes the referenced phase once per item, with per-iteration isolation.
func (a *Gather) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	items, err := extractItems(slots)
	if err != nil {
		return nil, nil, err
	}
	if len(items) == 0 {
		return []any{}, &execution.GatherUndoState{}, nil
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

	type iterOutcome struct {
		result  any
		undo    execution.IterationUndo
		err     error
	}
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
		// Concurrent execution with semaphore and cancellation.
		iterCtxBase, cancel := context.WithCancel(ctx.Context)
		defer cancel()

		sem := make(chan struct{}, limit)
		var wg sync.WaitGroup
		var firstErr error
		var mu sync.Mutex

		for i, item := range items {
			// Check for cancellation before starting a new iteration.
			select {
			case <-iterCtxBase.Done():
				break
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
				iterCtx := &execution.Context{
					Context: iterCtxBase,
					DryRun:  ctx.DryRun,
					Logger:  ctx.Logger,
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

	// Collect results and build undo state.
	var results []any
	undoState := &execution.GatherUndoState{}
	var gatherErr error

	for i, o := range outcomes {
		undoState.Iterations = append(undoState.Iterations, o.undo)
		if o.err != nil && gatherErr == nil {
			gatherErr = fmt.Errorf("gather: iteration %d failed: %w", i, o.err)
		}
	}

	if gatherErr != nil {
		// Unwind all completed iterations.
		a.undoCompleted(ctx, undoState)
		return nil, nil, gatherErr
	}

	for _, o := range outcomes {
		results = append(results, o.result)
	}

	return results, undoState, nil
}

// Undo walks iterations in reverse, re-resolves slots with saved proxy context,
// and calls Action.Undo per entry.
func (a *Gather) Undo(ctx *execution.Context, _ map[string]any, state execution.UndoState) error {
	gs, ok := state.(*execution.GatherUndoState)
	if !ok || gs == nil {
		return nil
	}
	return a.undoCompleted(ctx, gs)
}

// undoCompleted unwinds all iterations that have recovery entries.
func (a *Gather) undoCompleted(ctx *execution.Context, gs *execution.GatherUndoState) error {
	var errs []error
	for i := len(gs.Iterations) - 1; i >= 0; i-- {
		iter := gs.Iterations[i]
		for j := len(iter.Entries) - 1; j >= 0; j-- {
			entry := iter.Entries[j]
			if entry.Node.Action == nil {
				continue
			}
			entrySlots := entry.Node.ResolvedSlots(iter.Results, iter.ProxyCtx)
			execution.FillSlotsFromData(entrySlots, ctx.Data)
			if err := entry.Node.Action.Undo(ctx, entrySlots, entry.UndoState); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// executeIteration runs the phase body for a single item.
func executeIteration(ctx *execution.Context, ordered []*execution.Node, gatherID string, item any) struct {
	result any
	undo   execution.IterationUndo
	err    error
} {
	type outcome = struct {
		result any
		undo   execution.IterationUndo
		err    error
	}

	results := make(map[string]any)
	stack := &execution.RecoveryStack{}
	proxyCtx := map[string]any{gatherID: item}

	for _, node := range ordered {
		if node.Action == nil {
			continue
		}

		ctx.SourceChecksum = ""
		ctx.TargetChecksum = ""

		nodeSlots := node.ResolvedSlots(results, proxyCtx)
		execution.FillSlotsFromData(nodeSlots, ctx.Data)

		result, undoState, err := node.Action.Do(ctx, nodeSlots)
		if err != nil {
			// Unwind this iteration's completed nodes.
			stack.Unwind(ctx, results, proxyCtx)
			return outcome{
				undo: execution.IterationUndo{
					ProxyCtx: proxyCtx,
					Results:  results,
				},
				err: err,
			}
		}

		if result != nil {
			results[node.ID] = result
		}
		stack.Push(execution.RecoveryEntry{Node: node, UndoState: undoState})
	}

	// Terminal result is the last ordered node's result.
	var terminalResult any
	if len(ordered) > 0 {
		terminalResult = results[ordered[len(ordered)-1].ID]
	}

	return outcome{
		result: terminalResult,
		undo: execution.IterationUndo{
			ProxyCtx: proxyCtx,
			Results:  results,
			Entries:  stack.Entries(),
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
