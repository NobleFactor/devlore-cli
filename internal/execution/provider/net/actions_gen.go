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
// is written to disk (file mode). Otherwise it is returned as Result for
// downstream consumption.
type Download struct{ Impl *Provider }

func (o *Download) Name() string { return "net.download" }

func (o *Download) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	url, _ := slots["url"].(string)
	if url == "" {
		return nil, nil, fmt.Errorf("download: no url specified")
	}
	path, _ := slots["path"].(string)

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
		mode, _ := slots["mode"].(os.FileMode)
		if mode == 0 {
			mode = 0644
		}
		if err := os.WriteFile(path, data, mode); err != nil {
			return nil, nil, err
		}
		return data, map[string]any{"path": path}, nil
	}

	return data, nil, nil
}

func (o *Download) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	return o.Impl.CompensateDownload(s)
}

// Register registers all net actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Download{Impl: p})
}
