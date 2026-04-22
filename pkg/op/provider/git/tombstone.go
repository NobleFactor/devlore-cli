// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Tombstone is the git provider's compensation state record.
//
// Rule: a Tombstone exists for any object moved to a RecoverySite during the forward action. Git's Clone
// creates a directory rather than displacing one — no object is moved to a RecoverySite — so this Tombstone
// is structurally empty. It exists only to satisfy the compensable-pair return convention; CompensateClone
// reads the clone location from the embedded [Resource]'s [Resource.SourcePath] and removes it.
type Tombstone struct {
	op.TombstoneBase
}
