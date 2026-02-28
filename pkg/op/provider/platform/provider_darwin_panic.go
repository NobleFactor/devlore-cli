// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !darwin

package platform

import "github.com/NobleFactor/devlore-cli/pkg/op"

func newDarwin() *op.Platform {
	panic("newDarwin called on non-darwin platform")
}
