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

// detectHost returns a fresh host [*Spec] cloned from [Darwin] with arch defaulted to runtime.GOARCH at [New] time.
//
// It runs `sw_vers -productVersion` for the macOS version and reads the hostname.
//
// Returns:
//   - `*Spec`: the detected host spec.
//   - `error`: always nil on Darwin (host construction does not fail).
func detectHost() (*Spec, error) {

	spec := Darwin().WithArch("")

	if out, err := exec.CommandContext(context.Background(), "sw_vers", "-productVersion").Output(); err == nil {
		spec.WithVersion(strings.TrimSpace(string(out)))
	}

	if hostname, err := os.Hostname(); err == nil {
		spec.WithHostname(hostname)
	}

	return spec, nil
}
