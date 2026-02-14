// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Code generated from gen-receiver templates; DO NOT EDIT.

package execution

import "fmt"

// EncryptionDecryptOp decrypts content using the configured decryptor
// (transformer: reads content, stores decrypted result).
type EncryptionDecryptOp struct{ impl *EncryptionService }

func (o *EncryptionDecryptOp) Name() string { return "decrypt" }

func (o *EncryptionDecryptOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
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
	result, err := o.impl.Decrypt(decryptor, source, content)
	if err != nil {
		return nil, nil, err
	}
	ctx.StoreContent(node, result)
	return nil, nil, nil
}

func (o *EncryptionDecryptOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// EncryptionOps returns all encryption actions backed by the given EncryptionService.
func EncryptionOps(impl *EncryptionService) []Action {
	return []Action{
		&EncryptionDecryptOp{impl: impl},
	}
}
