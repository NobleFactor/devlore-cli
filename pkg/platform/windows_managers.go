// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

// Windows managers (winget, Service Control Manager) split across three files for cross-host build support:
//
//   - windows_managers.go          types + identity + driver wiring (this file, always compiled)
//   - windows_managers_windows.go  real shell-out primitives + runWindowsCommand helper (Windows only)
//   - windows_managers_other.go    stub primitives (every non-Windows host)
//
// On Windows: this file + windows_managers_windows.go combine; primitives are real.
// On any other host: this file + windows_managers_other.go combine; the shell-out primitives return error
// PlatformResults so cross-host fixtures construct successfully but fail loudly at run time.

// Interface guards: each type satisfies its interface on every host (real on Windows, stubbed elsewhere).
var (
	_ leaf           = (*wingetManager)(nil)
	_ ServiceManager = (*windowsServiceManager)(nil)
)

// =============================================================================
// Windows Service Manager (sc.exe)
// =============================================================================

type windowsServiceManager struct{}

// region EXPORTED METHODS

// region Behaviors

// NeedsSudo reports that Service Control Manager mutations require elevation.
//
// Returns:
//   - `bool`: always true.
func (m *windowsServiceManager) NeedsSudo() bool { return true }

// endregion

// endregion

// =============================================================================
// winget Package Manager — purl type "winget"
// =============================================================================
//
// winget package ids are publisher-scoped (e.g. Microsoft.VisualStudioCode). The purl carries the publisher as
// the namespace and the product as the name; [wingetManager.token] folds them back into the native id.

type wingetManager struct{ driver }

// newWingetManager constructs a winget leaf with its [driver] wired to its own primitives.
//
// Returns:
//   - `*wingetManager`: the wired leaf.
func newWingetManager() *wingetManager {
	m := &wingetManager{}
	m.driver = newDriver(m)
	return m
}

// region UNEXPORTED METHODS

// region State management

// name returns the manager's name — the user-facing pkg.Resource prefix.
//
// Returns:
//   - `string`: "winget".
func (m *wingetManager) name() string { return "winget" }

// purlType returns the manager's purl type — the routing key and the URI type.
//
// Returns:
//   - `string`: "winget".
func (m *wingetManager) purlType() string { return "winget" }

// endregion

// region Behaviors

// token derives the native winget id, folding the publisher namespace back in when present.
//
// Parameters:
//   - `p`: the package whose native id to derive.
//
// Returns:
//   - `string`: "Publisher.Name" when the purl has a namespace, otherwise the bare name.
func (m *wingetManager) token(p PURL) string {
	if p.Namespace != "" {
		return p.Namespace + "." + p.Name
	}
	return p.Name
}

// endregion

// endregion
