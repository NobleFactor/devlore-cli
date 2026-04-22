// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import "github.com/NobleFactor/devlore-cli/pkg/op"

// Tombstone holds file-specific compensation state.
//
// The embedded [op.TombstoneBase] carries the affected [Resource] whose identity is preserved. SourcePath always
// reflects the file's true home.
type Tombstone struct {
	op.TombstoneBase

	// RecoveryID records where the data was temporarily moved during the operation (backup, recovery site, or move
	// destination). An empty RecoveryID means no prior data existed to recover.
	RecoveryID string
}
