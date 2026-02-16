// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import "github.com/NobleFactor/devlore-cli/internal/execution"

// Choose is an OR-selector flow action. It evaluates alternatives (multiple
// predecessors) and selects one based on criteria. Only the selected branch
// is executed; unchosen branches are skipped.
type Choose struct{}

// Name returns the dotted action name.
func (a *Choose) Name() string { return "flow.choose" }

// Do evaluates alternatives and selects one. The selection criteria is stored
// in the "criteria" slot.
func (a *Choose) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	// Stub: the executor will implement branch selection via edge traversal.
	// The choose action itself is a passthrough — the executor skips unchosen
	// predecessor branches based on the selection result.
	return nil, nil, nil
}

// Undo is a no-op for choose — selection is not reversible.
func (a *Choose) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
	return nil
}
