// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import "github.com/NobleFactor/devlore-cli/pkg/op"

// Complete is the default, healthy conclusion of a graph path.
// Leaf node — nothing depends on it. Accepts an optional output value
// that can be captured by the graph consumer.
type Complete struct{}

// Name returns the dotted action name.
func (a *Complete) Name() string { return "flow.complete" }

// Params returns nil — Complete uses untyped slots.
func (a *Complete) Params() []op.ParamInfo { return nil }

// Do returns the output slot value. Not compensable — a successful
// terminal has nothing to undo.
func (a *Complete) Do(_ *op.Context, slots map[string]any) (op.Result, op.Complement, error) {
	return slots["output"], nil, nil
}
