// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import "github.com/NobleFactor/devlore-cli/pkg/op"

// Receipt holds git-specific compensation state for a [Provider.Clone] call.
//
// The embedded [op.ReceiptBase] carries the affected [Resource] (the cloned local repository) and the opaque
// [op.ReceiptBase.TransactionID] minted at [op.ReceiptBase.Commit] time. Clone is a Bucket-B (creation, not
// displacement) action — there is no prior content to archive, so the recovery key is the receipt's own
// transactionID; compensation simply removes the cloned directory tree.
type Receipt struct {
	op.ReceiptBase
}

// NewReceipt constructs a [Receipt] anchored to the cloned [Resource].
//
// The transactionID and action name remain zero-valued until [op.ReceiptBase.Commit] is invoked by
// [op.RecoveryStack.PushReceipt].
//
// Parameters:
//   - resource: the cloned [Resource] returned by [Provider.Clone].
//
// Returns:
//   - *Receipt: the constructed receipt with only its resource populated.
func NewReceipt(resource *Resource) *Receipt {
	return &Receipt{ReceiptBase: op.NewReceiptBase(resource)}
}