// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package platform provides platform-specific package and service managers.
// Concrete manager types are private; consumers see only op.PackageManager
// and op.ServiceManager.
package platform

import (
	"runtime"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// New returns a fully populated Platform for the current OS.
func New() *op.Platform {
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
