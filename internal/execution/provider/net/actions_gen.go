// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package net

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Download fetches content from a URL. If a "path" slot is set, the content
// is written to disk (file mode). Otherwise it is stored in the content
// pipeline for downstream consumption.
type Download struct{ Impl *Provider }

func (o *Download) Name() string { return "download" }

func (o *Download) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	url, _ := node.GetSlot("url").(string)
	if url == "" {
		return nil, nil, fmt.Errorf("download: no url specified")
	}
	path, _ := node.GetSlot("path").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] download %v\n", url)
		return nil, nil, nil
	}

	data, err := o.Impl.Download(url)
	if err != nil {
		return nil, nil, err
	}

	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, nil, fmt.Errorf("create parent dirs: %w", err)
		}
		mode := node.GetMode()
		if mode == 0 {
			mode = 0644
		}
		if err := os.WriteFile(path, data, mode); err != nil {
			return nil, nil, err
		}
	}

	ctx.StoreContent(node, data)
	return nil, nil, nil
}

func (o *Download) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Register registers all net actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Download{Impl: p})
}
