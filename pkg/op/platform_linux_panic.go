// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !linux

package op

func newLinux() *Platform {
	panic("newLinux called on non-linux platform")
}
