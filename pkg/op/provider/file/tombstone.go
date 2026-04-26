// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import "github.com/NobleFactor/devlore-cli/pkg/op"

// Tombstone holds file-specific compensation state.
//
// The embedded [op.ReceiptBase] carries the affected [Resource] whose identity is preserved and the opaque
// RecoveryID that locates the archived state. SourcePath always reflects the file's true home.
type Tombstone struct {
	op.ReceiptBase
}

func (t *Tombstone) RecoveryID() string {
	return t.TransactionID()
}
