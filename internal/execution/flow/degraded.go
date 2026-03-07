// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"fmt"
	"os"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Degraded is a terminal leaf node that marks a branch as non-optimal
// while allowing graph execution to continue. It formats a Go template
// message from its slots, writes to stderr, and returns the rendered
// warning as the node output.
type Degraded struct{}

// Name returns the dotted action name.
func (a *Degraded) Name() string { return "flow.degraded" }

// Params returns nil — Degraded uses untyped slots.
func (a *Degraded) Params() []op.ParamInfo { return nil }

// Do formats the template message and writes it to stderr. Returns the
// rendered warning as the node output with nil error — graph continues.
// Not compensable — a warning has no side effect to undo.
func (a *Degraded) Do(_ *op.Context, slots map[string]any) (op.Result, op.Complement, error) {
	format, _ := slots["format"].(string)
	args := reassembleArgs(slots)
	kwargs := reassembleKwargs(slots)

	rendered := op.RenderError(format, args, kwargs)
	fmt.Fprintln(os.Stderr, "degraded:", rendered)
	return rendered, nil, nil
}

// reassembleArgs reconstructs the positional args list from sub-slots.
// The planned receiver packs args as args[0], args[1], ... with args.len.
func reassembleArgs(slots map[string]any) []any {
	length, ok := slots["args.len"].(int)
	if !ok {
		return nil
	}
	args := make([]any, length)
	for i := range length {
		args[i] = slots[fmt.Sprintf("args[%d]", i)]
	}
	return args
}

// reassembleKwargs reconstructs the keyword args map from sub-slots.
// The planned receiver packs kwargs as kwargs.key1, kwargs.key2, etc.
func reassembleKwargs(slots map[string]any) map[string]any {
	kwargs := make(map[string]any)
	for k, v := range slots {
		if key, ok := strings.CutPrefix(k, "kwargs."); ok {
			kwargs[key] = v
		}
	}
	if len(kwargs) == 0 {
		return nil
	}
	return kwargs
}
