// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !windows

package platform

import "github.com/NobleFactor/devlore-cli/pkg/op"

func newWindows() *op.Platform {
	panic("newWindows called on non-windows platform")
}
