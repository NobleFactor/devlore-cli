// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package encryption

import "github.com/NobleFactor/devlore-cli/pkg/op"

// Tombstone holds encryption-specific compensation state.
type Tombstone struct {
	op.TombstoneBase
	DestinationPath string
}
