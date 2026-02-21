// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"errors"
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// chooseUndoState preserves the selected branch's recovery state.
type chooseUndoState struct {
	Results map[string]any          // node results for promise re-resolution
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
func (a *Choose) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	when, _ := slots["when"].(bool)
	thenPhaseID, _ := slots["then"].(string)
	elsePhaseID, _ := slots["else"].(string)

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

		ctx.SourceChecksum = ""
		ctx.TargetChecksum = ""

		nodeSlots := node.ResolvedSlots(results)
		execution.FillSlotsFromData(nodeSlots, ctx.Data)

		result, undoState, doErr := node.Action.Do(ctx, nodeSlots)
		if doErr != nil {
			stack.Unwind(ctx, results)
			return nil, nil, fmt.Errorf("choose: phase %s node %s: %w", selectedPhaseID, node.ID, doErr)
		}

		if result != nil {
			results[node.ID] = result
		}
		if _, ok := node.Action.(execution.CompensableAction); ok {
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
func (a *Choose) Undo(ctx *execution.Context, _ map[string]any, state execution.UndoState) error {
	cs, ok := state.(*chooseUndoState)
	if !ok || cs == nil {
		return nil
	}

	var errs []error
	for i := len(cs.Entries) - 1; i >= 0; i-- {
		entry := cs.Entries[i]
		undoable, ok := entry.Node.Action.(execution.CompensableAction)
		if !ok {
			continue
		}
		entrySlots := entry.Node.ResolvedSlots(cs.Results)
		execution.FillSlotsFromData(entrySlots, ctx.Data)
		if err := undoable.Undo(ctx, entrySlots, entry.UndoState); err != nil {
			if errors.Is(err, execution.NotCompensableError) {
				if ctx.Writer != nil {
					fmt.Fprintf(ctx.Writer, "  [warn] %s: not compensable, skipping\n", undoable.Name())
				}
				continue
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
