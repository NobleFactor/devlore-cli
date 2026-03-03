// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Elevate is a privilege transition flow action. It marks the boundary between
// unprivileged and privileged execution as an explicit graph node. In dry-run
// mode it reports "root required here"; the receipt records when privilege was
// acquired and released.
type Elevate struct{}

// Name returns the dotted action name.
func (a *Elevate) Name() string { return "flow.elevate" }

// Do acquires elevated privilege. Stub implementation — full sudo/privilege
// integration is a separate plan.
func (a *Elevate) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.UndoState, err error) {
	// Stub: privilege acquisition will be wired when the privilege model is
	// implemented. For now this is a passthrough that makes privilege
	// boundaries visible in the graph.
	return nil, nil, nil
}
