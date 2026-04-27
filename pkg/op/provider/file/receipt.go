// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import "github.com/NobleFactor/devlore-cli/pkg/op"

// Receipt holds the file-specific compensation state that the recovery system needs to undo a compensable forward call.
//
// The embedded [op.ReceiptBase] carries the affected [Resource] whose identity is preserved across compensation, and an
// opaque [op.ReceiptBase.TransactionID] that [op.RecoverySite] interprets as the recovery key when restoring archived
// bytes. SourcePath always reflects the file's true home — the location compensation will write back to.
//
// The optional boundary [Resource] marks the edge between existing filesystem state and the subtree the forward action
// created. Compensation walks toward boundary and stops at it (exclusive). [Provider.Mkdir], for example, sets boundary
// to the nearest pre-existing ancestor of its target directory so [Provider.CompensateMkdir] knows where to halt the
// upward removal walk. Methods that do not need a transactional anchor leave boundary nil.
type Receipt struct {
	op.ReceiptBase
	boundary *Resource
}

// NewReceipt constructs a [Receipt] anchored to the affected [Resource] with no transactional boundary.
//
// The transactionID and action name remain zero-valued until [op.ReceiptBase.Commit] is invoked by
// [op.RecoveryStack.PushReceipt]. Use this constructor wherever a forward action needs to return a Receipt rather than
// building the struct literal inline, and the action did not need to record a creation boundary for compensation.
//
// Parameters:
//   - resource: the [Resource] affected by the compensable forward method call.
//
// Returns:
//   - Receipt: the constructed receipt with only its resource populated.
func NewReceipt(resource *Resource) Receipt {
	return Receipt{ReceiptBase: op.NewReceiptBase(resource)}
}

// NewReceiptWithBoundary constructs a [Receipt] anchored to the affected [Resource] with a transactional boundary.
//
// Use this constructor when the forward action created or modified a subtree of filesystem state and compensation must
// stop at a known edge to avoid removing pre-existing entries. [Provider.Mkdir] is the canonical caller: resource is
// the directory created and boundary is the nearest pre-existing ancestor — compensation walks from resource up to but
// excluding boundary, removing each empty directory along the way.
//
// Parameters:
//   - resource: the [Resource] affected by the compensable forward method call.
//   - boundary: the [Resource] marking the existing-state edge; compensation stops at it (exclusive).
//
// Returns:
//   - Receipt: the constructed receipt with both resource and boundary populated.
func NewReceiptWithBoundary(resource, boundary *Resource) Receipt {
	return Receipt{ReceiptBase: op.NewReceiptBase(resource), boundary: boundary}
}

// Boundary returns the transactional boundary [Resource] supplied at construction, or nil if none was set.
//
// Compensation methods read this value to bound their cleanup walk: any walk that would step past boundary (an upward
// walk reaching it, or a downward walk descending into it) must halt. A nil boundary signals that the forward action
// did not record a creation subtree and the compensation method has no boundary-driven cleanup to perform.
//
// Returns:
//   - *Resource: the boundary supplied to [NewReceiptWithBoundary], or nil for receipts built via [NewReceipt].
func (r Receipt) Boundary() *Resource {
	return r.boundary
}
