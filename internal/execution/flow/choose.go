// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"errors"
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Choose is a predicate-driven branch selector. It evaluates case predicates
// against an input value and executes the first matching phase.
//
// Slots:
//   - input: any — the value to evaluate predicates against
//   - cases: []execution.ChooseCase — predicate+phase pairs, evaluated in order
//   - default: string — phase ID to execute when no predicate matches (optional)
//
// Result: the selected branch phase's terminal node Result.
// UndoState: *ChooseUndoState — the branch's recovery entries.
type Choose struct{}

// Name returns the dotted action name.
func (a *Choose) Name() string { return "flow.choose" }

// Do evaluates predicates and executes the matching branch phase.
func (a *Choose) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	input := slots["input"]

	cases, err := extractCases(slots)
	if err != nil {
		return nil, nil, err
	}
	defaultPhaseID, _ := slots["default"].(string)

	// Evaluate predicates in order — first match wins.
	var selectedPhaseID string
	for _, c := range cases {
		matched, evalErr := c.Predicate.Eval(input)
		if evalErr != nil {
			return nil, nil, fmt.Errorf("choose: predicate %s: %w", c.Predicate, evalErr)
		}
		if matched {
			selectedPhaseID = c.PhaseID
			break
		}
	}

	if selectedPhaseID == "" {
		if defaultPhaseID == "" {
			return nil, nil, fmt.Errorf("choose: no predicate matched and no default phase")
		}
		selectedPhaseID = defaultPhaseID
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
		stack.Push(execution.RecoveryEntry{Node: node, UndoState: undoState})
	}

	// Terminal result is the last ordered node's result.
	var terminalResult any
	if len(ordered) > 0 {
		terminalResult = results[ordered[len(ordered)-1].ID]
	}

	undoState := &execution.ChooseUndoState{
		Results: results,
		Entries: stack.Entries(),
	}

	return terminalResult, undoState, nil
}

// Undo walks the selected branch's entries in reverse and calls Action.Undo.
func (a *Choose) Undo(ctx *execution.Context, _ map[string]any, state execution.UndoState) error {
	cs, ok := state.(*execution.ChooseUndoState)
	if !ok || cs == nil {
		return nil
	}

	var errs []error
	for i := len(cs.Entries) - 1; i >= 0; i-- {
		entry := cs.Entries[i]
		if entry.Node.Action == nil {
			continue
		}
		entrySlots := entry.Node.ResolvedSlots(cs.Results)
		execution.FillSlotsFromData(entrySlots, ctx.Data)
		if err := entry.Node.Action.Undo(ctx, entrySlots, entry.UndoState); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// extractCases pulls the cases list from slots.
func extractCases(slots map[string]any) ([]execution.ChooseCase, error) {
	raw, ok := slots["cases"]
	if !ok {
		return nil, fmt.Errorf("choose: missing 'cases' slot")
	}
	cases, ok := raw.([]execution.ChooseCase)
	if !ok {
		return nil, fmt.Errorf("choose: 'cases' slot must be []ChooseCase, got %T", raw)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("choose: 'cases' slot is empty")
	}
	return cases, nil
}
