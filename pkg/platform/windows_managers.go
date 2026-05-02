// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import "strings"

// Windows managers (winget, Service Control Manager) split across three files for cross-host build
// support:
//
//   - windows_managers.go          types + pure methods (this file, always compiled)
//   - windows_managers_windows.go  real shell-out implementations + runWindowsCommand helper
//                                  (Windows only)
//   - windows_managers_other.go    stub shell-out implementations (every non-Windows host)
//
// On Windows: this file + windows_managers_windows.go combine; methods are real.
// On any other host: this file + windows_managers_other.go combine; shell-out methods return error
// PlatformResults so cross-host fixtures construct successfully but fail loudly at runtime.

// Compile-time interface guards — on every host, each type implements its full interface (real on
// Windows, stubbed on every other host).
var (
	_ PackageManager = (*wingetManager)(nil)
	_ ServiceManager = (*windowsServiceManager)(nil)
)

// =============================================================================
// winget Package Manager
// =============================================================================

type wingetManager struct{}

func (m *wingetManager) Name() string { return "winget" }

func (m *wingetManager) ParsePURL(id string) PURL {

	raw, version, _ := strings.Cut(id, "@")
	ns, name, ok := strings.Cut(raw, ".")
	if !ok {
		return PURL{Type: "winget", Name: raw, Version: version}
	}
	return PURL{Type: "winget", Namespace: ns, Name: name, Version: version}
}

func (m *wingetManager) NeedsSudo() bool { return false }

// =============================================================================
// Windows Service Manager (sc.exe)
// =============================================================================

type windowsServiceManager struct{}

func (m *windowsServiceManager) NeedsSudo() bool { return true }
