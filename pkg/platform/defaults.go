// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import "runtime"

// defaultLinuxPlatforms maps each known Linux distro to a factory that produces a fresh, fully-populated
// [PlatformSpec] for that distro. Factories (rather than shared spec values) ensure each call to [Linux]
// materializes an independent spec with an independent available-package-managers map; mutations via
// `With*` chaining never leak across calls.
//
// All entries are workstation-flavored (since most devs run desktop installs). [Detect] does the
// runtime refinement on actual hosts, stripping desktop-only managers (flatpak) when the host reports
// `multi-user.target` instead of `graphical.target`.
//
// Per-distro mapping (from the 13.0(i) plan-row table):
//
//	debian        -> apt              | apt
//	ubuntu        -> apt              | apt, snap
//	mint          -> apt              | apt, flatpak
//	rhel          -> dnf              | dnf, flatpak
//	fedora        -> dnf              | dnf, flatpak
//	centos-stream -> dnf              | dnf, flatpak
//	almalinux     -> dnf              | dnf, flatpak
//	rocky         -> dnf              | dnf, flatpak
//	arch          -> pacman           | pacman
//	manjaro       -> pacman           | pacman, snap, flatpak
var defaultLinuxPlatforms = map[string]func() *PlatformSpec{

	"debian": func() *PlatformSpec {
		apt := &aptManager{}
		return &PlatformSpec{
			os:                    "linux",
			distro:                "debian",
			defaultConcurrency:    4 * runtime.NumCPU(),
			defaultPackageManager: apt,
			availablePackageManagers: map[string]PackageManager{
				apt.Name(): apt,
			},
			serviceManager: &systemdManager{},
		}
	},

	"ubuntu": func() *PlatformSpec {
		apt, snap := &aptManager{}, &snapManager{}
		return &PlatformSpec{
			os:                    "linux",
			distro:                "ubuntu",
			defaultConcurrency:    4 * runtime.NumCPU(),
			defaultPackageManager: apt,
			availablePackageManagers: map[string]PackageManager{
				apt.Name():  apt,
				snap.Name(): snap,
			},
			serviceManager: &systemdManager{},
		}
	},

	"mint": func() *PlatformSpec {
		apt, flatpak := &aptManager{}, &flatpakManager{}
		return &PlatformSpec{
			os:                    "linux",
			distro:                "mint",
			defaultConcurrency:    4 * runtime.NumCPU(),
			defaultPackageManager: apt,
			availablePackageManagers: map[string]PackageManager{
				apt.Name():     apt,
				flatpak.Name(): flatpak,
			},
			serviceManager: &systemdManager{},
		}
	},

	"rhel": func() *PlatformSpec {
		dnf, flatpak := &dnfManager{}, &flatpakManager{}
		return &PlatformSpec{
			os:                    "linux",
			distro:                "rhel",
			defaultConcurrency:    4 * runtime.NumCPU(),
			defaultPackageManager: dnf,
			availablePackageManagers: map[string]PackageManager{
				dnf.Name():     dnf,
				flatpak.Name(): flatpak,
			},
			serviceManager: &systemdManager{},
		}
	},

	"fedora": func() *PlatformSpec {
		dnf, flatpak := &dnfManager{}, &flatpakManager{}
		return &PlatformSpec{
			os:                    "linux",
			distro:                "fedora",
			defaultConcurrency:    4 * runtime.NumCPU(),
			defaultPackageManager: dnf,
			availablePackageManagers: map[string]PackageManager{
				dnf.Name():     dnf,
				flatpak.Name(): flatpak,
			},
			serviceManager: &systemdManager{},
		}
	},

	"centos-stream": func() *PlatformSpec {
		dnf, flatpak := &dnfManager{}, &flatpakManager{}
		return &PlatformSpec{
			os:                    "linux",
			distro:                "centos-stream",
			defaultConcurrency:    4 * runtime.NumCPU(),
			defaultPackageManager: dnf,
			availablePackageManagers: map[string]PackageManager{
				dnf.Name():     dnf,
				flatpak.Name(): flatpak,
			},
			serviceManager: &systemdManager{},
		}
	},

	"almalinux": func() *PlatformSpec {
		dnf, flatpak := &dnfManager{}, &flatpakManager{}
		return &PlatformSpec{
			os:                    "linux",
			distro:                "almalinux",
			defaultConcurrency:    4 * runtime.NumCPU(),
			defaultPackageManager: dnf,
			availablePackageManagers: map[string]PackageManager{
				dnf.Name():     dnf,
				flatpak.Name(): flatpak,
			},
			serviceManager: &systemdManager{},
		}
	},

	"rocky": func() *PlatformSpec {
		dnf, flatpak := &dnfManager{}, &flatpakManager{}
		return &PlatformSpec{
			os:                    "linux",
			distro:                "rocky",
			defaultConcurrency:    4 * runtime.NumCPU(),
			defaultPackageManager: dnf,
			availablePackageManagers: map[string]PackageManager{
				dnf.Name():     dnf,
				flatpak.Name(): flatpak,
			},
			serviceManager: &systemdManager{},
		}
	},

	"arch": func() *PlatformSpec {
		pacman := &pacmanManager{}
		return &PlatformSpec{
			os:                    "linux",
			distro:                "arch",
			defaultConcurrency:    4 * runtime.NumCPU(),
			defaultPackageManager: pacman,
			availablePackageManagers: map[string]PackageManager{
				pacman.Name(): pacman,
			},
			serviceManager: &systemdManager{},
		}
	},

	"manjaro": func() *PlatformSpec {
		pacman, snap, flatpak := &pacmanManager{}, &snapManager{}, &flatpakManager{}
		return &PlatformSpec{
			os:                    "linux",
			distro:                "manjaro",
			defaultConcurrency:    4 * runtime.NumCPU(),
			defaultPackageManager: pacman,
			availablePackageManagers: map[string]PackageManager{
				pacman.Name():  pacman,
				snap.Name():    snap,
				flatpak.Name(): flatpak,
			},
			serviceManager: &systemdManager{},
		}
	},
}

// newDarwinDefault returns a fresh [PlatformSpec] for macOS. Brew is the default; brew + port are
// available. macOS uses launchd for service management.
func newDarwinDefault() *PlatformSpec {
	brew, port := &brewManager{}, &portManager{}
	return &PlatformSpec{
		os:                    "darwin",
		distro:                "macos",
		defaultConcurrency:    4 * runtime.NumCPU(),
		defaultPackageManager: brew,
		availablePackageManagers: map[string]PackageManager{
			brew.Name(): brew,
			port.Name(): port,
		},
		serviceManager: &launchdManager{},
	}
}

// newWindowsDefault returns a fresh [PlatformSpec] for Windows. winget is the only default-shipped
// manager. Windows uses Service Control Manager (sc.exe) for services.
func newWindowsDefault() *PlatformSpec {
	winget := &wingetManager{}
	return &PlatformSpec{
		os:                    "windows",
		distro:                "windows",
		defaultConcurrency:    4 * runtime.NumCPU(),
		defaultPackageManager: winget,
		availablePackageManagers: map[string]PackageManager{
			winget.Name(): winget,
		},
		serviceManager: &windowsServiceManager{},
	}
}
