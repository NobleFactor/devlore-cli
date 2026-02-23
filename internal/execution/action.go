// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package execution orchestrates graph-based action execution.
package execution

import "github.com/NobleFactor/devlore-cli/pkg/op"

// Action is the primary action interface, re-exported from pkg/op.
type Action = op.Action

// CompensableAction is an action that supports undo, re-exported from pkg/op.
type CompensableAction = op.CompensableAction

// Context is the execution context passed to actions, re-exported from pkg/op.
type Context = op.Context

// Result is the return value from an action, re-exported from pkg/op.
type Result = op.Result

// UndoState is the opaque state used for compensation, re-exported from pkg/op.
type UndoState = op.UndoState

// ErrNotCompensable is the sentinel indicating an action cannot be undone, re-exported from pkg/op.
var ErrNotCompensable = op.ErrNotCompensable
