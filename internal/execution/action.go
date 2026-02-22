// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"context"
	"errors"
	"io"
)

// NotCompensableError signals that an action acknowledges rollback but cannot
// undo its effect. The executor logs a warning and continues unwinding.
var NotCompensableError = errors.New("action is not compensable")

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

	// Do performs the forward action using resolved slot values.
	// Returns a result (flows to downstream nodes via promise slots) and undo
	// state (stored on recovery stack for rollback).
	Do(ctx *Context, slots map[string]any) (Result, UndoState, error)
}

// CompensableAction extends Action with compensation. Only actions that
// participate in rollback implement this interface.
type CompensableAction interface {
	Action

	// Undo performs the compensating action using the state captured by Do.
	Undo(state UndoState) error
}

// Context provides execution context to actions.
type Context struct {
	context.Context

	// DryRun prevents filesystem modifications when true.
	DryRun bool

	// Writer receives user-facing output messages.
	Writer io.Writer

	// Data holds tool-provided context: template variables, SOPS config,
	// identities, segment maps, etc. Each tool populates this before
	// calling GraphExecutor.Run().
	Data map[string]any

	// Graph is the graph being executed. Flow actions use this to look up
	// phases referenced by their slots (e.g., gather body, choose branch).
	Graph *Graph

	// NodeID is the ID of the currently executing node. Flow actions use
	// this to identify themselves (e.g., gather uses it for proxy context).
	NodeID string
}

