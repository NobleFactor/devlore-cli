// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import "github.com/NobleFactor/devlore-cli/pkg/op"

// Receipt holds file-specific compensation state.
//
// The embedded [op.ReceiptBase] carries the affected [Resource] whose identity is preserved across
// compensation and the opaque [op.ReceiptBase.TransactionID] that [op.RecoverySite] interprets as the
// recovery key when restoring archived bytes. SourcePath always reflects the file's true home.
type Receipt struct {
	op.ReceiptBase
}

// NewReceipt constructs a [Receipt] anchored to the affected [Resource].
//
// The transactionID and action name remain zero-valued until [op.ReceiptBase.Commit] is called by
// [op.RecoveryStack.PushReceipt]. Use this constructor wherever a forward action needs to return a Receipt
// rather than building the struct literal inline.
//
// Parameters:
//   - resource: the [Resource] affected by the compensable forward method call.
//
// Returns:
//   - Receipt: the constructed receipt with only its resource populated.
func NewReceipt(resource *Resource) Receipt {
	return Receipt{op.NewReceiptBase(resource)}
}
