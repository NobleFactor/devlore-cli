// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build darwin

package platform

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// detectHost runs `sw_vers -productVersion` for the macOS version, reads the hostname, and constructs a
// [Platform] from [newDarwinDefault] with arch defaulted to runtime.GOARCH.
//
// Returns:
//   - Platform: the detected platform value.
//   - error: when [PlatformSpec.Build] fails.
func detectHost() (Platform, error) {

	spec := newDarwinDefault().WithArch("")

	if out, err := exec.CommandContext(context.Background(), "sw_vers", "-productVersion").Output(); err == nil {
		spec.WithVersion(strings.TrimSpace(string(out)))
	}

	if hostname, err := os.Hostname(); err == nil {
		spec.WithHostname(hostname)
	}

	return spec.Build()
}
