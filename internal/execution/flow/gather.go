// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import "github.com/NobleFactor/devlore-cli/internal/execution"

// Gather is an AND-join flow action. It waits for all predecessors to complete
// before proceeding. Equivalent to Promise.all() — every input must succeed
// for the gather node to succeed.
type Gather struct{}

// Name returns the dotted action name.
func (a *Gather) Name() string { return "flow.gather" }

// Do verifies that all predecessors completed successfully.
func (a *Gather) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	// Stub: the executor enforces predecessor completion via edge ordering.
	// The gather action itself is a passthrough that makes AND-join semantics
	// explicit in the graph.
	return nil, nil, nil
}

// Undo is a no-op for gather — join points are not reversible.
func (a *Gather) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
	return nil
}
