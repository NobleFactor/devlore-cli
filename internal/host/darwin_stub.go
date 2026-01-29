// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !darwin

package host

// Stub for non-Darwin platforms. Never called at runtime; NewHost()
// dispatches via runtime.GOOS. Exists only for cross-platform compilation.
func newDarwinHost() Host {
	panic("newDarwinHost called on non-darwin platform")
}
