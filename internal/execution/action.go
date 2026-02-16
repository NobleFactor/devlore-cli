// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"context"
	"io"
)

// Result is data that flows to downstream nodes via edges (e.g., file content,
// a rendered template, a query result). The executor stores this keyed by node
// ID and resolves promise slots from stored Results before calling downstream Do.
type Result = any

// UndoState is the state captured by Do and passed to Undo during saga
// rollback. Each action defines its own state shape. Actions with no rollback
// return nil from Do; their Undo ignores the state parameter.
type UndoState = any

// Action is the interface for all executable actions.
// Actions receive resolved slots — they never touch *Node. The executor
// resolves all promise slots before calling Do.
type Action interface {
	// Name returns the action identifier (e.g., "file.link", "template.render").
	Name() string

	// Do performs the forward action using resolved slot values.
	// Returns a result (flows to downstream nodes via promise slots) and undo
	// state (stored on recovery stack for rollback).
	Do(ctx *Context, slots map[string]any) (Result, UndoState, error)

	// Undo performs the compensating action using the state captured by Do.
	Undo(ctx *Context, slots map[string]any, state UndoState) error
}

// Context provides execution context to actions.
type Context struct {
	context.Context

	// DryRun prevents filesystem modifications when true.
	DryRun bool

	// Logger receives action output messages.
	Logger io.Writer

	// Data holds tool-provided context: template variables, SOPS config,
	// identities, segment maps, etc. Each tool populates this before
	// calling GraphExecutor.Run().
	Data map[string]any

	// Per-node checksums (written by actions, read by executor).
	SourceChecksum string
	TargetChecksum string
}
