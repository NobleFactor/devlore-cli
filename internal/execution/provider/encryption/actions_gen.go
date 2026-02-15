// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package encryption

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Decrypt decrypts content using the configured decryptor
// (transformer: reads content, stores decrypted result).
type Decrypt struct{ Impl *Provider }

func (o *Decrypt) Name() string { return "decrypt" }

func (o *Decrypt) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	decryptor, _ := node.GetSlot("decryptor").(func(string, []byte) ([]byte, error))
	source, _ := node.GetSlot("source").(string)
	content, err := ctx.ContentFor(node)
	if err != nil {
		return nil, nil, err
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] decrypt %v\n", source)
		return nil, nil, nil
	}
	result, err := o.Impl.Decrypt(decryptor, source, content)
	if err != nil {
		return nil, nil, err
	}
	ctx.StoreContent(node, result)
	return nil, nil, nil
}

func (o *Decrypt) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Register registers all encryption actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Decrypt{Impl: p})
}
