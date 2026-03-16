// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import "github.com/NobleFactor/devlore-cli/pkg/op"

// Fatal is a terminal leaf node that halts graph execution immediately.
// Same signature as Degraded — a Go template format string with
// args/kwargs. The executor detects FatalError and stops.
type Fatal struct{}

// Name returns the dotted action name.
func (a *Fatal) Name() string { return "flow.fatal" }

// Params returns nil — Fatal uses untyped slots.
func (a *Fatal) Params() []op.ParamInfo { return nil }

// Do formats the template message and returns a FatalError. Not
// compensable — prior nodes unwind via the existing recovery stack.
func (a *Fatal) Do(_ *op.Context, slots map[string]any) (op.Result, op.Complement, error) {
	format, _ := slots["format"].(string)
	args := reassembleArgs(slots)
	kwargs := reassembleKwargs(slots)

	return nil, nil, &op.FatalError{Message: op.RenderError(format, args, kwargs).Error()}
}
