// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package manifest

import (
	"fmt"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Resolve reads a packages-manifest file and stores the package list.
// The planning step expands this into a lore package lifecycle pipeline.
type Resolve struct{ Impl *Provider }

func (o *Resolve) Name() string { return "manifest-resolve" }

func (o *Resolve) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	source, _ := node.GetSlot("source").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] manifest-resolve %v\n", source)
		return nil, nil, nil
	}

	packages, err := o.Impl.Resolve(source)
	if err != nil {
		return nil, nil, err
	}

	// Store package list for the planning step to expand
	ctx.StoreContent(node, []byte(strings.Join(packages, "\n")))
	return nil, nil, nil
}

func (o *Resolve) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Register registers all manifest actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Resolve{Impl: p})
}
