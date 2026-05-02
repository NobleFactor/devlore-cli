// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import "strings"

// Linux managers (apt, dnf, pacman, systemd) split across three files for cross-host build support:
//
//   - linux_managers.go        types + pure methods (this file, always compiled)
//   - linux_managers_linux.go  real shell-out implementations (Linux only)
//   - linux_managers_other.go  stub shell-out implementations (every non-Linux host)
//
// On Linux: this file + linux_managers_linux.go combine; methods are real.
// On any other host: this file + linux_managers_other.go combine; shell-out methods return an error
// PlatformResult ("apt-get not available on this host (target=linux)") so cross-host fixtures construct
// successfully but fail loudly at runtime.

// Compile-time interface guards — on every host, each type implements its full interface (real on
// Linux, stubbed on every other host).
var (
	_ PackageManager = (*aptManager)(nil)
	_ PackageManager = (*dnfManager)(nil)
	_ PackageManager = (*pacmanManager)(nil)
	_ ServiceManager = (*systemdManager)(nil)
)

// =============================================================================
// APT Package Manager (Debian, Ubuntu, Mint)
// =============================================================================

type aptManager struct{}

func (m *aptManager) Name() string { return "apt" }

func (m *aptManager) ParsePURL(id string) PURL {

	name, version, _ := strings.Cut(id, "@")
	return PURL{Type: "deb", Name: name, Version: version}
}

func (m *aptManager) NeedsSudo() bool { return true }

// =============================================================================
// DNF Package Manager (Fedora, RHEL, CentOS, Alma, Rocky)
// =============================================================================

type dnfManager struct{}

func (m *dnfManager) Name() string { return "dnf" }

func (m *dnfManager) ParsePURL(id string) PURL {

	name, version, _ := strings.Cut(id, "@")
	return PURL{Type: "rpm", Name: name, Version: version}
}

func (m *dnfManager) NeedsSudo() bool { return true }

// =============================================================================
// Pacman Package Manager (Arch, Manjaro)
// =============================================================================

type pacmanManager struct{}

func (m *pacmanManager) Name() string { return "pacman" }

func (m *pacmanManager) ParsePURL(id string) PURL {

	name, version, _ := strings.Cut(id, "@")
	return PURL{Type: "alpm", Name: name, Version: version}
}

func (m *pacmanManager) NeedsSudo() bool { return true }

// =============================================================================
// systemd Service Manager
// =============================================================================

type systemdManager struct{}

func (m *systemdManager) NeedsSudo() bool { return true }
