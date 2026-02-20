// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package content

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Literal reads inline content and stores it for downstream nodes.
type Literal struct{ Impl *Provider }

func (o *Literal) Name() string { return "content.literal" }

func (o *Literal) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	raw, _ := slots["content"].(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Writer, "[dry-run] literal (%d bytes)\n", len(raw))
		return nil, nil, nil
	}
	content := o.Impl.Literal([]byte(raw))
	return content, nil, nil
}

// Register registers all content actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Literal{Impl: p})
}
