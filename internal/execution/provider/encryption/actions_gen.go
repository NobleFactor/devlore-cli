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

func (o *Decrypt) Name() string { return "encryption.decrypt" }

func (o *Decrypt) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	decryptor, _ := slots["decryptor"].(func(string, []byte) ([]byte, error))
	source, _ := slots["source"].(string)
	content, _ := slots["content"].([]byte)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] decrypt %v\n", source)
		return nil, nil, nil
	}
	result, err := o.Impl.Decrypt(decryptor, source, content)
	if err != nil {
		return nil, nil, err
	}
	return result, nil, nil
}

func (o *Decrypt) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
	return nil
}

// Register registers all encryption actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Decrypt{Impl: p})
}
