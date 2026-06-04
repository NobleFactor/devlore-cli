// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build windows

package platform

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// detectHost runs `cmd /c ver` for the Windows version, reads the hostname, and constructs a [Platform]
// from [newWindowsDefault] with arch defaulted to runtime.GOARCH.
//
// Returns:
//   - Platform: the detected platform value.
//   - error: when [NewPlatform] fails.
func detectHost() (Platform, error) {

	spec := newWindowsDefault().WithArch("")

	if out, err := exec.CommandContext(context.Background(), "cmd", "/c", "ver").Output(); err == nil {
		spec.WithVersion(strings.TrimSpace(string(out)))
	}

	if hostname, err := os.Hostname(); err == nil {
		spec.WithHostname(hostname)
	}

	return NewPlatform(spec)
}
