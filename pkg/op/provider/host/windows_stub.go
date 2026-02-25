// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !windows

package host

// Stub for non-Windows platforms. Never called at runtime; NewHost()
// dispatches via runtime.GOOS. Exists only for cross-platform compilation.
func newWindowsHost() Host {
	panic("newWindowsHost called on non-windows platform")
}
