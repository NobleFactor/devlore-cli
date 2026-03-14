// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "runtime"

// NewPlatform returns a fully populated Platform for the current OS.
func NewPlatform() *Platform {
	switch runtime.GOOS {
	case "darwin":
		return newDarwin()
	case "linux":
		return newLinux()
	case "windows":
		return newWindows()
	default:
		return newLinux()
	}
}
