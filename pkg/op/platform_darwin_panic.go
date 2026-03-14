// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !darwin

package op

func newDarwin() *Platform {
	panic("newDarwin called on non-darwin platform")
}
