// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
)

// ErrNotCompensable signals that an action acknowledges rollback but cannot
// undo its effect. The executor logs a warning and continues unwinding.
var ErrNotCompensable = errors.New("action is not compensable")

// Result is data that flows to downstream nodes via edges (e.g., file content,
// a rendered template, a query result). The executor stores this keyed by node
// ID and resolves promise slots from stored Results before calling downstream Do.
type Result = any

// UndoState is the state captured by Do and passed to Undo during saga
// rollback. Each action defines its own state shape. Actions with no rollback
// return nil from Do; their Undo ignores the state parameter.
type UndoState = any

// Action is the forward-only interface. All executable actions implement this.
// Actions receive resolved slots — they never touch *Node. The executor
// resolves all promise slots before calling Do.
type Action interface {

	// Name returns the action identifier (e.g., "file.link", "template.render").
	Name() string

	// Do executes the action with the given context and resolved slot values.
	//
	// Parameters:
	//   - ctx: execution context
	//   - slots: resolved slot values
	//
	// Returns:
	//   - result: A result for downstream nodes
	//   - undo: undo state for saga rollback
	//   - err: any error encountered during execution
	Do(ctx *Context, slots map[string]any) (Result, UndoState, error)
}

// CompensableAction is the backward-only interface.
//
// All actions that can be undone implement this.
type CompensableAction interface {
	Action

	// Undo performs the compensating action using the state captured by Do.
	//
	// Parameters:
	//   - ctx: execution context
	//   - state: undo state captured by Do
	//
	// Returns:
	//   - err: any error encountered during compensation
	Undo(ctx *Context, undo UndoState) error
}
