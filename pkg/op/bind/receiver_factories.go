// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// ExecutingReceiverFactory is optional.
//
// Receivers that contribute an immediate receiver (e.g., file, ui) implement this.
type ExecutingReceiverFactory interface {
	NewExecuting(ctx op.Context) starlark.Value
}

// PlanningReceiverFactory is optional. Checked via type assertion.
//
// Receivers that contribute a plan sub-namespace (e.g., plan.file) implement this.
type PlanningReceiverFactory interface {
	NewPlanning(graph *op.Graph, project string, reg *op.ReceiverRegistry) starlark.Value
}

// NoSuchAttrError returns an error for an unknown attribute.
func NoSuchAttrError(receiver, attr string) error {
	return fmt.Errorf("%s has no .%s attribute", receiver, attr)
}

// builtinFunc is the signature for builtin function implementations.
type builtinFunc func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)
