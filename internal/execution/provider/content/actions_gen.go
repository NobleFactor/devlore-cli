// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package content

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Literal reads inline content and stores it for downstream nodes.
type Literal struct{ Impl *Provider }

func (o *Literal) Name() string { return "literal" }

func (o *Literal) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	raw, _ := node.GetSlot("content").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] literal (%d bytes)\n", len(raw))
		return nil, nil, nil
	}
	content := o.Impl.Literal([]byte(raw))
	ctx.StoreContent(node, content)
	return nil, nil, nil
}

func (o *Literal) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Register registers all content actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Literal{Impl: p})
}
