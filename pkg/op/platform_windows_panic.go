// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !windows

package op

import "github.com/NobleFactor/devlore-cli/pkg/assert"

func newWindows() *Platform {
	assert.Unreachable("newWindows called on non-windows platform")
	return nil
}
