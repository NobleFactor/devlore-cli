// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "reflect"

// MethodKind classifies a provider method by its return signature.
type MethodKind int

const (
	// MethodPure produces a result and is guaranteed not to fail. Return: () or (T).
	MethodPure MethodKind = iota

	// MethodFallible produces a result or an error. Return: (error) or (T, error).
	MethodFallible

	// MethodCompensable produces a result and complement or an error. Return: (T, U, error).
	MethodCompensable
)

// Method describes a callable method on a provider or resource.
//
// It is the shared metadata for both graph execution (action dispatch) and
// starlark bridge construction. It implements the Action and CompensableAction
// interfaces for graph dispatch. Starlark bridges read its fields to build
// closures.
//
// The Do and Undo functions are set by bind during registration — they
// capture the dispatch logic (coercion, callable init, return classification)
// that requires bind infrastructure.
type Method struct {
	Factory    ReceiverFactory
	Reflect    reflect.Method
	ActionName string   // dotted action name: "file.write_text"
	ParamNames []string // cleaned: no ?, *, **
	Compensate reflect.Method
	Kind       MethodKind

	// DoFunc is the graph execution dispatch function, set by bind during registration.
	DoFunc func(ctx *Context, slots map[string]any) (Result, Complement, error)

	// UndoFunc is the compensation function, set by bind for compensable methods.
	UndoFunc func(ctx *Context, complement Complement) error
}

// Name implements Action.
func (m *Method) Name() string { return m.ActionName }

// Params implements Action.
func (m *Method) Params() []ParamInfo {
	params := make([]ParamInfo, len(m.ParamNames))
	for i, name := range m.ParamNames {
		params[i] = ParamInfo{Name: name, Type: m.Reflect.Type.In(i + 1)}
	}
	return params
}

// Do implements Action.
func (m *Method) Do(ctx *Context, slots map[string]any) (Result, Complement, error) {
	return m.DoFunc(ctx, slots)
}

// Undo implements CompensableAction.
func (m *Method) Undo(ctx *Context, complement Complement) error {
	if m.UndoFunc == nil {
		return ErrNotCompensable
	}
	return m.UndoFunc(ctx, complement)
}

// IsCompensable reports whether this method has an undo companion.
func (m *Method) IsCompensable() bool { return m.Kind == MethodCompensable }
