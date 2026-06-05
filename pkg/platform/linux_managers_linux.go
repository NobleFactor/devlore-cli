// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build linux

package platform

import "strings"

// Real shell-out primitives for the Linux managers (apt, dnf, pacman, systemd). The implicit _linux.go build
// constraint scopes this file to Linux hosts; non-Linux hosts get the stub primitives from
// linux_managers_other.go. The exported [PackageManager] surface is assembled from these primitives by the
// embedded [driver] (see linux_managers.go).

// =============================================================================
// APT Package Manager — shell-out primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports whether the named package exists in the apt index.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when `apt-cache show` resolves the package.
func (m *aptManager) available(name string) bool {
	return runShellCommand("apt-cache show "+name, false).OK
}

// installRaw installs the named packages.
//
// Parameters:
//   - `names`: the package names to install.
//   - `kwargs`: opaque native flags (unused by apt).
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *aptManager) installRaw(names []string, _ map[string]any) PlatformResult {
	return runShellCommand("apt-get install -y "+strings.Join(names, " "), true)
}

// installed reports whether the named package is installed.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when `dpkg-query` resolves the package.
func (m *aptManager) installed(name string) bool {
	return runShellCommand("dpkg-query -W "+name, false).OK
}

// removeRaw uninstalls the named packages.
//
// Parameters:
//   - `names`: the package names to uninstall.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *aptManager) removeRaw(names []string) PlatformResult {
	return runShellCommand("apt-get remove -y "+strings.Join(names, " "), true)
}

// searchRaw returns up to `limit` packages matching `query`.
//
// Parameters:
//   - `query`: the search term.
//   - `limit`: the maximum number of results; <= 0 means no limit.
//
// Returns:
//   - `[]SearchResult`: the matches, or nil on failure.
func (m *aptManager) searchRaw(query string, limit int) []SearchResult {
	result := runShellCommand("apt-cache search "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	for _, line := range strings.Split(result.Stdout, "\n") {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) >= 1 && parts[0] != "" {
			sr := SearchResult{Name: strings.TrimSpace(parts[0])}
			if len(parts) >= 2 {
				sr.Description = strings.TrimSpace(parts[1])
			}
			results = append(results, sr)
			if limit > 0 && len(results) >= limit {
				return results
			}
		}
	}
	return results
}

// version returns the installed version of the named package, or "" when it is not installed.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `string`: the installed version, or "".
func (m *aptManager) version(name string) string {
	result := runShellCommand("dpkg-query -W -f='${Version}' "+name, false)
	if !result.OK {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

// endregion

// endregion

// =============================================================================
// DNF Package Manager — shell-out primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports whether the named package exists in the dnf index.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when `dnf info` resolves the package.
func (m *dnfManager) available(name string) bool {
	return runShellCommand("dnf info "+name, false).OK
}

// installRaw installs the named packages.
//
// Parameters:
//   - `names`: the package names to install.
//   - `kwargs`: opaque native flags (unused by dnf).
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *dnfManager) installRaw(names []string, _ map[string]any) PlatformResult {
	return runShellCommand("dnf install -y "+strings.Join(names, " "), true)
}

// installed reports whether the named package is installed.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when `rpm -q` resolves the package.
func (m *dnfManager) installed(name string) bool {
	return runShellCommand("rpm -q "+name, false).OK
}

// removeRaw uninstalls the named packages.
//
// Parameters:
//   - `names`: the package names to uninstall.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *dnfManager) removeRaw(names []string) PlatformResult {
	return runShellCommand("dnf remove -y "+strings.Join(names, " "), true)
}

// searchRaw returns up to `limit` packages matching `query`.
//
// Parameters:
//   - `query`: the search term.
//   - `limit`: the maximum number of results; <= 0 means no limit.
//
// Returns:
//   - `[]SearchResult`: the matches, or nil on failure.
func (m *dnfManager) searchRaw(query string, limit int) []SearchResult {
	result := runShellCommand("dnf search "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	for _, line := range strings.Split(result.Stdout, "\n") {
		if strings.HasPrefix(line, "=") || strings.HasPrefix(line, "Last metadata") || line == "" {
			continue
		}
		parts := strings.SplitN(line, " : ", 2)
		if len(parts) >= 1 {
			namePart := strings.TrimSpace(parts[0])
			if idx := strings.LastIndex(namePart, "."); idx > 0 {
				namePart = namePart[:idx]
			}
			if namePart == "" {
				continue
			}
			sr := SearchResult{Name: namePart}
			if len(parts) >= 2 {
				sr.Description = strings.TrimSpace(parts[1])
			}
			results = append(results, sr)
			if limit > 0 && len(results) >= limit {
				return results
			}
		}
	}
	return results
}

// version returns the installed version of the named package, or "" when it is not installed.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `string`: the installed version, or "".
func (m *dnfManager) version(name string) string {
	result := runShellCommand("rpm -q --queryformat '%{VERSION}' "+name, false)
	if !result.OK {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

// endregion

// endregion

// =============================================================================
// Pacman Package Manager — shell-out primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports whether the named package exists in the pacman sync database.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when `pacman -Si` resolves the package.
func (m *pacmanManager) available(name string) bool {
	return runShellCommand("pacman -Si "+name, false).OK
}

// installRaw installs the named packages.
//
// Parameters:
//   - `names`: the package names to install.
//   - `kwargs`: opaque native flags (unused by pacman).
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *pacmanManager) installRaw(names []string, _ map[string]any) PlatformResult {
	return runShellCommand("pacman -S --noconfirm --needed "+strings.Join(names, " "), true)
}

// installed reports whether the named package is installed.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when `pacman -Q` resolves the package.
func (m *pacmanManager) installed(name string) bool {
	return runShellCommand("pacman -Q "+name, false).OK
}

// removeRaw uninstalls the named packages.
//
// Parameters:
//   - `names`: the package names to uninstall.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *pacmanManager) removeRaw(names []string) PlatformResult {
	return runShellCommand("pacman -R --noconfirm "+strings.Join(names, " "), true)
}

// searchRaw returns up to `limit` packages matching `query`.
//
// Parameters:
//   - `query`: the search term.
//   - `limit`: the maximum number of results; <= 0 means no limit.
//
// Returns:
//   - `[]SearchResult`: the matches, or nil on failure.
func (m *pacmanManager) searchRaw(query string, limit int) []SearchResult { //nolint:gocognit // parsing format requires nesting

	result := runShellCommand("pacman -Ss "+query, false)

	if !result.OK {
		return nil
	}

	var results []SearchResult
	lines := strings.Split(result.Stdout, "\n")

	for i := 0; i < len(lines); i++ {

		line := lines[i]

		if strings.HasPrefix(line, " ") {
			continue
		}

		parts := strings.Fields(line)

		if len(parts) >= 1 {

			repoAndName := parts[0]

			if idx := strings.Index(repoAndName, "/"); idx >= 0 {

				pkgName := repoAndName[idx+1:]
				sr := SearchResult{Name: pkgName}

				if i+1 < len(lines) && strings.HasPrefix(lines[i+1], " ") {
					sr.Description = strings.TrimSpace(lines[i+1])
				}

				results = append(results, sr)

				if limit > 0 && len(results) >= limit {
					return results
				}
			}
		}
	}

	return results
}

// version returns the installed version of the named package, or "" when it is not installed.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `string`: the installed version, or "".
func (m *pacmanManager) version(name string) string {

	result := runShellCommand("pacman -Q "+name, false)

	if !result.OK {
		return ""
	}

	parts := strings.Fields(result.Stdout)

	if len(parts) >= 2 {
		return parts[1]
	}

	return ""
}

// endregion

// endregion

// =============================================================================
// systemd Service Manager — shell-out methods
// =============================================================================

// region EXPORTED METHODS

// region Behaviors

// Disable disables the named service from starting at boot.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *systemdManager) Disable(name string) PlatformResult {
	return runShellCommand("systemctl disable "+name, true)
}

// Enable enables the named service to start at boot.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *systemdManager) Enable(name string) PlatformResult {
	return runShellCommand("systemctl enable "+name, true)
}

// Exists reports whether a unit with the given name is known to systemd.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `bool`: true when `systemctl cat` resolves the unit.
func (m *systemdManager) Exists(name string) bool {
	return runShellCommand("systemctl cat "+name, false).OK
}

// IsEnabled reports whether the named service is enabled to start at boot.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `bool`: true when `systemctl is-enabled` succeeds.
func (m *systemdManager) IsEnabled(name string) bool {
	return runShellCommand("systemctl is-enabled --quiet "+name, false).OK
}

// IsRunning reports whether the named service is currently active.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `bool`: true when `systemctl is-active` succeeds.
func (m *systemdManager) IsRunning(name string) bool {
	return runShellCommand("systemctl is-active --quiet "+name, false).OK
}

// Start starts the named service.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *systemdManager) Start(name string) PlatformResult {
	return runShellCommand("systemctl start "+name, true)
}

// Status returns the active-state string for the named service.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `string`: the trimmed `systemctl is-active` output (e.g. "active", "inactive").
func (m *systemdManager) Status(name string) string {
	result := runShellCommand("systemctl is-active "+name, false)
	return strings.TrimSpace(result.Stdout)
}

// Stop stops the named service.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *systemdManager) Stop(name string) PlatformResult {
	return runShellCommand("systemctl stop "+name, true)
}

// endregion

// endregion
