// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !linux

package op

import "github.com/NobleFactor/devlore-cli/pkg/assert"

func newLinux() *Platform {
	assert.Unreachable("newLinux called on non-linux platform")
	return nil
}
