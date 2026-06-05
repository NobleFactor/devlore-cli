// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

// Detect inspects the running host and returns a fresh, mutable [*Spec] reflecting it.
//
// The spec reflects the host's actual OS, distro, architecture, version, hostname, and managers; seal it into a
// [Platform] with [New]. Detect is the only host-inspecting entry point; the named factories ([Debian], [Fedora],
// [Darwin], [Windows], …) are deterministic and touch nothing on the host. On Linux, Detect reads /etc/os-release
// for the distro and refines the available-manager set per the running variant (stripping desktop-only managers
// like flatpak on server installs). On Darwin it runs `sw_vers`; on Windows, `cmd /c ver`.
//
// Returns:
//   - `*Spec`: the detected host spec.
//   - `error`: when the host OS is not one of linux / darwin / windows, or when detection fails.
func Detect() (*Spec, error) {
	return detectHost()
}
