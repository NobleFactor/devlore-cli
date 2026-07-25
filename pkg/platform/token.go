// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import "strings"

// Token returns the canonical dotted platform token: "Darwin", "Windows", "Linux.Debian", "Linux.Fedora", or
// "Linux" when the distro family is unknown.
//
// The token is the vocabulary the devlore tools share — lore's registry resolves platform-partitioned script
// directories by it (`docker/Linux.Debian/Deploy/`), writ's tree matches segment-variant directories with the
// same capitalized names, and manifest planning selects releases by it. Distro grouping is by package-manager
// family: apt-based distributions render as `Linux.Debian`, dnf/yum-based as `Linux.Fedora`.
//
// Parameters:
//   - `p`: the platform to render.
//
// Returns:
//   - `string`: the canonical token.
func Token(p Platform) string {

	switch p.OS() {
	case "darwin":
		return "Darwin"
	case "windows":
		return "Windows"
	case "linux":
		switch strings.ToLower(p.Distro()) {
		case "debian", "ubuntu":
			return "Linux.Debian"
		case "fedora", "rhel", "centos", "rocky", "alma":
			return "Linux.Fedora"
		default:
			return "Linux"
		}
	default:
		return "Linux"
	}
}

// DetectToken detects the host platform and returns its canonical token, falling back to "Linux" when
// detection fails.
//
// Returns:
//   - `string`: the host's canonical token, or "Linux" when the host cannot be detected.
func DetectToken() string {

	spec, err := Detect()
	if err != nil {
		return "Linux"
	}

	host, err := New(spec)
	if err != nil {
		return "Linux"
	}

	return Token(host)
}
