// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package archive

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Extract extracts an archive to a target directory.
type Extract struct{ Impl *Provider }

func (o *Extract) Name() string { return "archive.extract" }

func (o *Extract) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	// Accept "source" or "archive" for the archive path
	source, _ := slots["source"].(string)
	if source == "" {
		source, _ = slots["archive"].(string)
	}
	if source == "" {
		return nil, nil, fmt.Errorf("archive-extract: no source specified")
	}

	// Accept "path" or "prefix" for the extraction directory
	prefix, _ := slots["path"].(string)
	if prefix == "" {
		prefix, _ = slots["prefix"].(string)
	}
	if prefix == "" {
		return nil, nil, fmt.Errorf("archive-extract: no path specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] archive-extract %v \u2192 %v\n", source, prefix)
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Extract(source, prefix)
}

func (o *Extract) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
	return nil
}

// Register registers all archive actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Extract{Impl: p})
}
