// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

//go:build !windows

package host

// Stub for non-Windows platforms
func newWindowsHost() Host {
	// Return a darwin host as fallback (will work for basic operations)
	return newDarwinHost()
}
