// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"fmt"
)

// Linux returns a [Platform] for the named Linux distro and architecture.
//
// distro must be one of the known Linux distros (debian, ubuntu, mint, rhel, fedora, centos-stream,
// almalinux, rocky, arch, manjaro). arch must be one of the known Docker-vocabulary architectures
// (amd64, arm64, arm/v7, arm/v6, 386, ppc64le, s390x, mips64le, riscv64); empty arch defaults to
 // runtime.GOARCH at [NewPlatform] time.
//
// The returned Platform is workstation-flavored (since most devs run desktop installs) — see the
// per-distro defaults table in [defaultLinuxPlatforms]. Cross-host fixtures work: a Mac dev can call
// platform.Linux("ubuntu", "amd64") to construct a Linux platform value for plan-time use. Run-time
// preflight catches target-vs-host mismatches before any wrong-platform manager invocation.
//
// Parameters:
//   - distro: one of the known Linux distros.
//   - arch: one of the known Docker-vocabulary architectures, or "" for runtime.GOARCH.
//
// Returns:
//   - Platform: the constructed platform value.
//   - error: if distro is unknown, arch is unknown, or any spec validation fails.
func Linux(distro, arch string) (Platform, error) {

	factory, ok := defaultLinuxPlatforms[distro]
	if !ok {
		return nil, fmt.Errorf("platform: unknown linux distro %q; expected one of debian, ubuntu, mint, rhel, fedora, centos-stream, almalinux, rocky, arch, manjaro", distro)
	}
	return NewPlatform(factory().WithArch(arch))
}

// Darwin returns a [Platform] for macOS at the given architecture.
//
// arch must be one of the known Docker-vocabulary architectures; empty defaults to runtime.GOARCH.
// The default package manager is brew; brew and port are both available.
//
// Parameters:
//   - arch: one of the known Docker-vocabulary architectures, or "" for runtime.GOARCH.
//
// Returns:
//   - Platform: the constructed platform value.
//   - error: if arch is unknown or any spec validation fails.
func Darwin(arch string) (Platform, error) {
	return NewPlatform(newDarwinDefault().WithArch(arch))
}

// Windows returns a [Platform] for Windows at the given architecture.
//
// arch must be one of the known Docker-vocabulary architectures; empty defaults to runtime.GOARCH.
// The default and only package manager is winget.
//
// Parameters:
//   - arch: one of the known Docker-vocabulary architectures, or "" for runtime.GOARCH.
//
// Returns:
//   - Platform: the constructed platform value.
//   - error: if arch is unknown or any spec validation fails.
func Windows(arch string) (Platform, error) {
	return NewPlatform(newWindowsDefault().WithArch(arch))
}

// Detect inspects the running host and returns a [Platform] reflecting its actual OS, distro,
// architecture, and managers.
//
// On Linux, Detect reads /etc/os-release for the distro identifier (the ID field) and refines the
// available-managers set per the running variant — VARIANT_ID workstation/server distinctions when
// present, falling back to `systemctl get-default` (graphical.target keeps workstation defaults;
// multi-user.target strips desktop-only managers like flatpak). On Darwin, runs `sw_vers` for the
// version. On Windows, runs `cmd /c ver`.
//
// Detect is the runtime gate per the 13.0(i) design: named constructors ([Linux], [Darwin], [Windows])
// are deterministic; only Detect performs host inspection.
//
// Returns:
//   - Platform: the detected platform value.
//   - error: when the host OS is not one of linux/darwin/windows, or when detection fails.
func Detect() (Platform, error) {
	return detectHost()
}
