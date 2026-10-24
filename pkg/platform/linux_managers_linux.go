// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build linux

package platform

import (
	"strings"
	"time"
)

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
	return m.run([]string{"apt-cache", "show", name}, AccountCaller).OK
}

// installRaw installs the named packages.
//
// Parameters:
//   - `names`: the package names to install.
//   - `kwargs`: opaque native flags (unused by apt).
//
// Returns:
//   - `Result`: the command result.
func (m *aptManager) installRaw(names []string, _ map[string]any) Result {
	return m.run(append([]string{"apt-get", "install", "-y"}, names...), AccountRoot)
}

// refresh updates the apt package index.
//
// Returns:
//   - `Result`: the command result.
func (m *aptManager) refresh() Result {
	return m.run([]string{"apt-get", "update"}, AccountRoot)
}

// indexAge reports how long ago the apt package index was last refreshed (the mtime of /var/lib/apt/lists).
//
// Returns:
//   - `time.Duration`: the index age, or [unknownIndexAge] when the list directory is absent.
func (m *aptManager) indexAge() time.Duration {
	return indexAgeOf("/var/lib/apt/lists")
}

// installed reports whether the named package is installed.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when `dpkg-query` resolves the package.
func (m *aptManager) installed(name string) bool {
	return m.run([]string{"dpkg-query", "-W", name}, AccountCaller).OK
}

// removeRaw uninstalls the named packages.
//
// Parameters:
//   - `names`: the package names to uninstall.
//
// Returns:
//   - `Result`: the command result.
func (m *aptManager) removeRaw(names []string) Result {
	return m.run(append([]string{"apt-get", "remove", "-y"}, names...), AccountRoot)
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
	result := m.run([]string{"apt-cache", "search", query}, AccountCaller)
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
	// The single quotes were only ever hiding ${Version} from a shell; dpkg parses the format itself, so as
	// an argv element it needs none.
	result := m.run([]string{"dpkg-query", "-W", "-f=${Version}", name}, AccountCaller)
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
	return m.run([]string{"dnf", "info", name}, AccountCaller).OK
}

// installRaw installs the named packages.
//
// Parameters:
//   - `names`: the package names to install.
//   - `kwargs`: opaque native flags (unused by dnf).
//
// Returns:
//   - `Result`: the command result.
func (m *dnfManager) installRaw(names []string, _ map[string]any) Result {
	return m.run(append([]string{"dnf", "install", "-y"}, names...), AccountRoot)
}

// refresh rebuilds the dnf metadata cache.
//
// Returns:
//   - `Result`: the command result.
func (m *dnfManager) refresh() Result {
	return m.run([]string{"dnf", "makecache"}, AccountRoot)
}

// installed reports whether the named package is installed.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when `rpm -q` resolves the package.
func (m *dnfManager) installed(name string) bool {
	return m.run([]string{"rpm", "-q", name}, AccountCaller).OK
}

// removeRaw uninstalls the named packages.
//
// Parameters:
//   - `names`: the package names to uninstall.
//
// Returns:
//   - `Result`: the command result.
func (m *dnfManager) removeRaw(names []string) Result {
	return m.run(append([]string{"dnf", "remove", "-y"}, names...), AccountRoot)
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
	result := m.run([]string{"dnf", "search", query}, AccountCaller)
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
	// As with dpkg above, the quotes existed to hide %{VERSION} from a shell. rpm parses it itself.
	result := m.run([]string{"rpm", "-q", "--queryformat", "%{VERSION}", name}, AccountCaller)
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
	return m.run([]string{"pacman", "-Si", name}, AccountCaller).OK
}

// installRaw installs the named packages.
//
// Parameters:
//   - `names`: the package names to install.
//   - `kwargs`: opaque native flags (unused by pacman).
//
// Returns:
//   - `Result`: the command result.
func (m *pacmanManager) installRaw(names []string, _ map[string]any) Result {
	return m.run(append([]string{"pacman", "-S", "--noconfirm", "--needed"}, names...), AccountRoot)
}

// refresh synchronizes the pacman package databases.
//
// Returns:
//   - `Result`: the command result.
func (m *pacmanManager) refresh() Result {
	return m.run([]string{"pacman", "-Sy", "--noconfirm"}, AccountRoot)
}

// indexAge reports how long ago the pacman sync databases were last refreshed (the mtime of /var/lib/pacman/sync).
//
// Returns:
//   - `time.Duration`: the index age, or [unknownIndexAge] when the sync directory is absent.
func (m *pacmanManager) indexAge() time.Duration {
	return indexAgeOf("/var/lib/pacman/sync")
}

// installed reports whether the named package is installed.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when `pacman -Q` resolves the package.
func (m *pacmanManager) installed(name string) bool {
	return m.run([]string{"pacman", "-Q", name}, AccountCaller).OK
}

// removeRaw uninstalls the named packages.
//
// Parameters:
//   - `names`: the package names to uninstall.
//
// Returns:
//   - `Result`: the command result.
func (m *pacmanManager) removeRaw(names []string) Result {
	return m.run(append([]string{"pacman", "-R", "--noconfirm"}, names...), AccountRoot)
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

	result := m.run([]string{"pacman", "-Ss", query}, AccountCaller)

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

	result := m.run([]string{"pacman", "-Q", name}, AccountCaller)

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
//   - `Result`: the command result.
func (m *systemdManager) Disable(name string) Result {
	return m.run([]string{"systemctl", "disable", name}, AccountRoot)
}

// Enable enables the named service to start at boot.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `Result`: the command result.
func (m *systemdManager) Enable(name string) Result {
	return m.run([]string{"systemctl", "enable", name}, AccountRoot)
}

// Exists reports whether a unit with the given name is known to systemd.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `bool`: true when `systemctl cat` resolves the unit.
func (m *systemdManager) Exists(name string) bool {
	return m.run([]string{"systemctl", "cat", name}, AccountCaller).OK
}

// IsEnabled reports whether the named service is enabled to start at boot.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `bool`: true when `systemctl is-enabled` succeeds.
func (m *systemdManager) IsEnabled(name string) bool {
	return m.run([]string{"systemctl", "is-enabled", "--quiet", name}, AccountCaller).OK
}

// IsRunning reports whether the named service is currently active.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `bool`: true when `systemctl is-active` succeeds.
func (m *systemdManager) IsRunning(name string) bool {
	return m.run([]string{"systemctl", "is-active", "--quiet", name}, AccountCaller).OK
}

// Start starts the named service.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `Result`: the command result.
func (m *systemdManager) Start(name string) Result {
	return m.run([]string{"systemctl", "start", name}, AccountRoot)
}

// Status returns the active-state string for the named service.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `string`: the trimmed `systemctl is-active` output (e.g. "active", "inactive").
func (m *systemdManager) Status(name string) string {
	result := m.run([]string{"systemctl", "is-active", name}, AccountCaller)
	return strings.TrimSpace(result.Stdout)
}

// Stop stops the named service.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `Result`: the command result.
func (m *systemdManager) Stop(name string) Result {
	return m.run([]string{"systemctl", "stop", name}, AccountRoot)
}

// sysVinitNoBoot is the error returned by [sysVinitManager.Enable] / [sysVinitManager.Disable], which are
// intentionally unsupported.
//
// On the systemd-less boxes where SysVinit is selected (containers, WSL, minimal/CI hosts) "start at boot" is
// effectively meaningless — the box is recreated from an image — so enablement fails honestly rather than guessing
// between update-rc.d (Debian) and chkconfig (RHEL).
const sysVinitNoBoot = "sysvinit: enable/disable is unsupported (boot-persistence is out of scope for SysVinit environments)"

// Disable fails: SysVinit boot-persistence is intentionally unsupported.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `Result`: an error result; see [sysVinitNoBoot].
func (m *sysVinitManager) Disable(_ string) Result {
	return Result{OK: false, Stderr: sysVinitNoBoot}
}

// Enable fails: SysVinit boot-persistence is intentionally unsupported.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `Result`: an error result; see [sysVinitNoBoot].
func (m *sysVinitManager) Enable(_ string) Result {
	return Result{OK: false, Stderr: sysVinitNoBoot}
}

// Exists reports whether an init script exists for the named service.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `bool`: true when /etc/init.d/<name> is present and executable.
func (m *sysVinitManager) Exists(name string) bool {
	return m.run([]string{"test", "-x", "/etc/init.d/" + name}, AccountCaller).OK
}

// IsEnabled reports false: SysVinit boot-persistence is not tracked here.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false (enable/disable are unsupported; see [sysVinitNoBoot]).
func (m *sysVinitManager) IsEnabled(_ string) bool { return false }

// IsRunning reports whether the named service is currently running.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `bool`: true when `service <name> status` exits zero.
func (m *sysVinitManager) IsRunning(name string) bool {
	return m.run([]string{"service", name, "status"}, AccountCaller).OK
}

// Start starts the named service via the SysVinit `service` wrapper.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `Result`: the command result.
func (m *sysVinitManager) Start(name string) Result {
	return m.run([]string{"service", name, "start"}, AccountRoot)
}

// Status returns a coarse run-state for the named service.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `string`: "running" when `service <name> status` exits zero, else "stopped".
func (m *sysVinitManager) Status(name string) string {
	if m.run([]string{"service", name, "status"}, AccountCaller).OK {
		return "running"
	}
	return "stopped"
}

// Stop stops the named service via the SysVinit `service` wrapper.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `Result`: the command result.
func (m *sysVinitManager) Stop(name string) Result {
	return m.run([]string{"service", name, "stop"}, AccountRoot)
}

// endregion

// endregion
