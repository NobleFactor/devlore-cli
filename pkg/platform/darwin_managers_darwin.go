// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build darwin

package platform

import (
	"context"
	"os/exec"
	"strings"
)

// Real shell-out primitives for the Darwin managers (brew, port, launchd). The implicit _darwin.go build
// constraint scopes this file to Darwin hosts; non-Darwin hosts get the stub primitives from
// darwin_managers_other.go. The exported [PackageManager] surface is assembled from these primitives by the
// embedded [driver] (see darwin_managers.go).

// =============================================================================
// Homebrew Package Manager — shell-out primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports whether the named formula or cask exists in Homebrew's index.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when `brew info` resolves the package.
func (m *brewManager) available(name string) bool {
	return runShellCommand("brew info "+name, false).OK
}

// installRaw installs the named packages, honoring a `cask` kwarg for GUI applications.
//
// Parameters:
//   - `names`: the package names to install.
//   - `kwargs`: opaque native flags; a truthy `cask` selects `brew install --cask`.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *brewManager) installRaw(names []string, kwargs map[string]any) PlatformResult {

	command := "brew install "
	if cask, _ := kwargs["cask"].(bool); cask {
		command = "brew install --cask "
	}

	return runShellCommand(command+strings.Join(names, " "), false)
}

// installed reports whether the named package is installed as a formula or a cask.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when the package is installed under either kind.
func (m *brewManager) installed(name string) bool {
	if runShellCommand("brew list --formula "+name, false).OK {
		return true
	}
	return runShellCommand("brew list --cask "+name, false).OK
}

// removeRaw uninstalls the named packages.
//
// Parameters:
//   - `names`: the package names to uninstall.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *brewManager) removeRaw(names []string) PlatformResult {
	return runShellCommand("brew uninstall "+strings.Join(names, " "), false)
}

// searchRaw returns up to `limit` packages matching `query`.
//
// Parameters:
//   - `query`: the search term.
//   - `limit`: the maximum number of results; <= 0 means no limit.
//
// Returns:
//   - `[]SearchResult`: the matches, or nil on failure.
func (m *brewManager) searchRaw(query string, limit int) []SearchResult {
	result := runShellCommand("brew search "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "==>") {
			continue
		}
		for _, pkg := range strings.Fields(line) {
			if pkg == "" {
				continue
			}
			results = append(results, SearchResult{Name: pkg})
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
func (m *brewManager) version(name string) string {
	result := runShellCommand("brew list --versions "+name, false)
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
// launchd Service Manager — shell-out methods
// =============================================================================

// region EXPORTED METHODS

// region Behaviors

// Disable unloads the named launchd job, trying the user agent before the system daemon.
//
// Parameters:
//   - `name`: the launchd label.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *launchdManager) Disable(name string) PlatformResult {
	userPlist := "~/Library/LaunchAgents/" + name + ".plist"
	systemPlist := "/Library/LaunchDaemons/" + name + ".plist"

	result := runShellCommand("launchctl unload -w "+userPlist, false)
	if result.OK {
		return result
	}
	return runShellCommand("launchctl unload -w "+systemPlist, true)
}

// Enable loads the named launchd job, trying the user agent before the system daemon.
//
// Parameters:
//   - `name`: the launchd label.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *launchdManager) Enable(name string) PlatformResult {
	userPlist := "~/Library/LaunchAgents/" + name + ".plist"
	systemPlist := "/Library/LaunchDaemons/" + name + ".plist"

	result := runShellCommand("launchctl load -w "+userPlist, false)
	if result.OK {
		return result
	}
	return runShellCommand("launchctl load -w "+systemPlist, true)
}

// Exists reports whether a launchd job with the given label is loaded.
//
// Parameters:
//   - `name`: the launchd label.
//
// Returns:
//   - `bool`: true when the job appears in `launchctl list`.
func (m *launchdManager) Exists(name string) bool {
	return runShellCommand("launchctl list | grep -q "+name, false).OK
}

// IsEnabled reports whether the named job is enabled. launchd exposes no reliable query, so this is always false.
//
// Parameters:
//   - `name`: the launchd label.
//
// Returns:
//   - `bool`: always false.
func (m *launchdManager) IsEnabled(_ string) bool {
	return false
}

// IsRunning reports whether the named job has a live PID.
//
// Parameters:
//   - `name`: the launchd label.
//
// Returns:
//   - `bool`: true when `launchctl list <name>` reports a non-dash PID.
func (m *launchdManager) IsRunning(name string) bool {
	out, err := exec.CommandContext(context.Background(), "launchctl", "list", name).Output() //nolint:gosec // G204: command built from provider inputs
	if err != nil {
		return false
	}
	fields := strings.Fields(string(out))
	return len(fields) > 0 && fields[0] != "-"
}

// Start starts the named launchd job.
//
// Parameters:
//   - `name`: the launchd label.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *launchdManager) Start(name string) PlatformResult {
	return runShellCommand("launchctl start "+name, false)
}

// Status returns "running" when the named job is loaded, otherwise "stopped".
//
// Parameters:
//   - `name`: the launchd label.
//
// Returns:
//   - `string`: "running" or "stopped".
func (m *launchdManager) Status(name string) string {
	if runShellCommand("launchctl list "+name, false).OK {
		return "running"
	}
	return "stopped"
}

// Stop stops the named launchd job.
//
// Parameters:
//   - `name`: the launchd label.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *launchdManager) Stop(name string) PlatformResult {
	return runShellCommand("launchctl stop "+name, false)
}

// endregion

// endregion

// =============================================================================
// MacPorts Package Manager — shell-out primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports whether the named port exists in the MacPorts index.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when `port info` resolves the package.
func (m *portManager) available(name string) bool {
	return runShellCommand("port info "+name, false).OK
}

// installRaw installs the named ports (MacPorts requires elevation).
//
// Parameters:
//   - `names`: the package names to install.
//   - `kwargs`: opaque native flags (unused by MacPorts).
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *portManager) installRaw(names []string, _ map[string]any) PlatformResult {
	return runShellCommand("port install -N "+strings.Join(names, " "), true)
}

// installed reports whether the named port is installed.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when `port installed` lists the package.
func (m *portManager) installed(name string) bool {
	return runShellCommand("port installed "+name+" | grep -q "+name, false).OK
}

// removeRaw uninstalls the named ports.
//
// Parameters:
//   - `names`: the package names to uninstall.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *portManager) removeRaw(names []string) PlatformResult {
	return runShellCommand("port uninstall "+strings.Join(names, " "), true)
}

// searchRaw returns up to `limit` ports matching `query`.
//
// Parameters:
//   - `query`: the search term.
//   - `limit`: the maximum number of results; <= 0 means no limit.
//
// Returns:
//   - `[]SearchResult`: the matches, or nil on failure.
func (m *portManager) searchRaw(query string, limit int) []SearchResult {
	result := runShellCommand("port search --name "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	for _, line := range strings.Split(result.Stdout, "\n") {
		if strings.HasPrefix(line, " ") || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			sr := SearchResult{Name: fields[0]}
			if len(fields) >= 2 {
				sr.Version = strings.Trim(fields[1], "@")
			}
			results = append(results, sr)
			if limit > 0 && len(results) >= limit {
				return results
			}
		}
	}
	return results
}

// version returns the installed version of the named port, or "" when it is not installed.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `string`: the installed version, or "".
func (m *portManager) version(name string) string {
	result := runShellCommand("port installed "+name, false)
	if !result.OK {
		return ""
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		if strings.Contains(line, "@") {
			parts := strings.Split(line, "@")
			if len(parts) >= 2 {
				return strings.Fields(parts[1])[0]
			}
		}
	}
	return ""
}

// endregion

// endregion
