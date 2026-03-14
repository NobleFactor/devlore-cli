// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !windows

package op

func newWindows() *Platform {
	panic("newWindows called on non-windows platform")
}
