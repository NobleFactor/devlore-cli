// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"reflect"
)

// ErrNotCompensable signals that "Do" acknowledges rollback but cannot undo its effect.
//
// The executor logs a warning and continues unwinding.
var ErrNotCompensable = errors.New("action is not compensable")

// Result is data that flows to downstream nodes via edges (e.g., file content, a rendered template, a query result).
//
// The executor stores this keyed by node ID and resolves promise slots from stored Results before calling downstream
// Do.
type Result = any

// Complement is the state captured by Do and passed to Undo during saga rollback.
//
// Each "Do" defines its own state shape. Actions with no rollback return nil from Do; their Undo ignores the state
// parameter.
type Complement = any

// Parameter describes a single parameter accepted by an do's Do method.
type Parameter struct {
	Name string
	Type reflect.Type
}

// Action is a pure, infallible value transformer. No side effects, cannot fail.
//
// Do returns (result, nil, nil).
type Action interface {
	FullName() string
	Name() string
	Params() []Parameter
	Do(ctx *ExecutionContext, slots map[string]any) (Result, Complement, error)
}

// FallibleAction has side effects and can fail.
//
// Do returns (result, nil, error).
type FallibleAction interface {
	Action
}

// CompensableAction has side effects, can fail, and can be undone.
//
// Do returns (result, complement, error).
type CompensableAction interface {
	Action
	Undo(ctx *ExecutionContext, complement Complement) error
}
