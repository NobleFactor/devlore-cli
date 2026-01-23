// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

//go:build !linux

package host

// Stub for non-Linux platforms. Never called at runtime; NewHost()
// dispatches via runtime.GOOS. Exists only for cross-platform compilation.
func newLinuxHost() Host {
	panic("newLinuxHost called on non-linux platform")
}
