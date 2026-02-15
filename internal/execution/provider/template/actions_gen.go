// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package template

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Render processes content as a Go text/template (transformer: reads content, stores result).
type Render struct{ Impl *Provider }

func (o *Render) Name() string { return "render" }

func (o *Render) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	source, _ := node.GetSlot("source").(string)
	path, _ := node.GetSlot("path").(string)
	project := node.GetProject()
	templateData := make(map[string]any)
	for k, v := range ctx.Data {
		templateData[k] = v
	}
	content, err := ctx.ContentFor(node)
	if err != nil {
		return nil, nil, err
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] render %v %v\n", source, path)
		return nil, nil, nil
	}
	result, err := o.Impl.Render(templateData, source, path, project, content)
	if err != nil {
		return nil, nil, err
	}
	ctx.StoreContent(node, result)
	return nil, nil, nil
}

func (o *Render) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Register registers all template actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Render{Impl: p})
}
