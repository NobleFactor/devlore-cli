// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package flow implements flow-control actions for execution graphs.
package flow

import (
	"errors"
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// chooseUndoState preserves the selected branch's recovery state.
type chooseUndoState struct {
	Results map[string]any            // node results for promise re-resolution
	Entries []execution.RecoveryEntry // branch node refs + per-node undo state
}

// Choose is a conditional branch selector. It reads a boolean from its
// "when" slot and executes either the "then" or "else" phase.
//
// Slots:
//   - when: bool — condition (resolved from a predicate action's output)
//   - then: string — phase ID to execute when true
//   - else: string — phase ID to execute when false (optional)
//
// Result: the selected branch phase's terminal node Result.
// UndoState: *chooseUndoState — the branch's recovery entries.
type Choose struct{}

// Name returns the dotted action name.
func (a *Choose) Name() string { return "flow.choose" }

// Do reads the boolean condition and executes the matching branch phase.
func (a *Choose) Do(ctx *op.Context, slots map[string]any) (result op.Result, undo op.UndoState, err error) {
	when, _ := slots["when"].(bool)          //nolint:errcheck // zero value (false) is acceptable default
	thenPhaseID, _ := slots["then"].(string) //nolint:errcheck // zero value (empty) is acceptable
	elsePhaseID, _ := slots["else"].(string) //nolint:errcheck // zero value (empty) is acceptable

	var selectedPhaseID string
	if when {
		selectedPhaseID = thenPhaseID
	} else {
		selectedPhaseID = elsePhaseID
	}

	if selectedPhaseID == "" {
		// No branch to execute — no-op.
		return nil, nil, nil
	}

	// Look up and execute the selected phase.
	graph := ctx.Graph
	if graph == nil {
		return nil, nil, fmt.Errorf("choose: no graph in context")
	}
	phase := graph.PhaseByID(selectedPhaseID)
	if phase == nil {
		return nil, nil, fmt.Errorf("choose: phase %q not found", selectedPhaseID)
	}

	phaseNodes, phaseEdges := graph.CollectPhaseNodes(phase)
	ordered := execution.OrderNodes(phaseNodes, phaseEdges)

	results := make(map[string]any)
	stack := &execution.RecoveryStack{}

	for _, node := range ordered {
		if node.Action == nil {
			continue
		}

		nodeSlots := node.ResolvedSlots(results)
		execution.FillSlotsFromData(nodeSlots, ctx.Data)

		result, undoState, doErr := node.Action.Do(ctx, nodeSlots)
		if doErr != nil {
			stack.Unwind(ctx)
			return nil, nil, fmt.Errorf("choose: phase %s node %s: %w", selectedPhaseID, node.ID, doErr)
		}

		if result != nil {
			results[node.ID] = result
		}
		if _, ok := node.Action.(op.CompensableAction); ok {
			stack.Push(execution.RecoveryEntry{Node: node, UndoState: undoState})
		}
	}

	// Terminal result is the last ordered node's result.
	var terminalResult any
	if len(ordered) > 0 {
		terminalResult = results[ordered[len(ordered)-1].ID]
	}

	undoState := &chooseUndoState{
		Results: results,
		Entries: stack.Entries(),
	}

	return terminalResult, undoState, nil
}

// Undo walks the selected branch's entries in reverse and calls CompensableAction.Undo.
func (a *Choose) Undo(ctx *op.Context, state op.UndoState) error {
	cs, ok := state.(*chooseUndoState)
	if !ok || cs == nil {
		return nil
	}

	var errs []error
	for i := len(cs.Entries) - 1; i >= 0; i-- {
		entry := cs.Entries[i]
		undoable, ok := entry.Node.Action.(op.CompensableAction)
		if !ok {
			continue
		}
		if err := undoable.Undo(ctx, entry.UndoState); err != nil {
			if errors.Is(err, op.ErrNotCompensable) {
				continue
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
