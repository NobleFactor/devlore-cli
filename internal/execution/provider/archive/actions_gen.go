// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package archive

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Extract extracts an archive to a target directory.
type Extract struct{ Impl *Provider }

func (o *Extract) Name() string { return "archive-extract" }

func (o *Extract) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	// Accept "source" or "archive" for the archive path
	source, _ := node.GetSlot("source").(string)
	if source == "" {
		source, _ = node.GetSlot("archive").(string)
	}
	if source == "" {
		return nil, nil, fmt.Errorf("archive-extract: no source specified")
	}

	// Accept "path" or "prefix" for the extraction directory
	prefix, _ := node.GetSlot("path").(string)
	if prefix == "" {
		prefix, _ = node.GetSlot("prefix").(string)
	}
	if prefix == "" {
		return nil, nil, fmt.Errorf("archive-extract: no path specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] archive-extract %v → %v\n", source, prefix)
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Extract(source, prefix)
}

func (o *Extract) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Register registers all archive actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Extract{Impl: p})
}
