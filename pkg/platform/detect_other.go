// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !linux && !darwin && !windows

package platform

import (
	"fmt"
	"runtime"
)

// detectHost returns an error on unsupported host operating systems. Supported hosts (linux, darwin,
// windows) provide their own [detectHost] in build-tagged files.
func detectHost() (Platform, error) {
	return nil, fmt.Errorf("platform: Detect not supported on host OS %q; supported hosts are linux, darwin, windows", runtime.GOOS)
}
