// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- Interface guards ---

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

func TestTombstoneImplementsInterface(t *testing.T) {
	var _ op.Receipt = (*Tombstone)(nil)
}
