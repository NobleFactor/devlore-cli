// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

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
	return runCommand([]string{"brew", "info", name}, false).OK
}

// installRaw installs the named packages, honoring a `cask` kwarg for GUI applications.
//
// Parameters:
//   - `names`: the package names to install.
//   - `kwargs`: opaque native flags; a truthy `cask` selects `brew install --cask`.
//
// Returns:
//   - `Result`: the command result.
func (m *brewManager) installRaw(names []string, kwargs map[string]any) Result {

	argv := []string{"brew", "install"}
	if cask, ok := kwargs["cask"].(bool); ok && cask {
		argv = append(argv, "--cask")
	}

	return runCommand(append(argv, names...), false)
}

// refresh updates Homebrew's formula and cask metadata.
//
// Returns:
//   - `Result`: the command result.
func (m *brewManager) refresh() Result {
	return runCommand([]string{"brew", "update"}, false)
}

// installed reports whether the named package is installed as a formula or a cask.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when the package is installed under either kind.
func (m *brewManager) installed(name string) bool {
	if runCommand([]string{"brew", "list", "--formula", name}, false).OK {
		return true
	}
	return runCommand([]string{"brew", "list", "--cask", name}, false).OK
}

// removeRaw uninstalls the named packages.
//
// Parameters:
//   - `names`: the package names to uninstall.
//
// Returns:
//   - `Result`: the command result.
func (m *brewManager) removeRaw(names []string) Result {
	return runCommand(append([]string{"brew", "uninstall"}, names...), false)
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
	result := runCommand([]string{"brew", "search", query}, false)
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
	result := runCommand([]string{"brew", "list", "--versions", name}, false)
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
//   - `Result`: the command result.
func (m *launchdManager) Disable(name string) Result {
	userPlist := "~/Library/LaunchAgents/" + name + ".plist"
	systemPlist := "/Library/LaunchDaemons/" + name + ".plist"

	result := runCommand([]string{"launchctl", "unload", "-w", userPlist}, false)
	if result.OK {
		return result
	}
	return runCommand([]string{"launchctl", "unload", "-w", systemPlist}, true)
}

// Enable loads the named launchd job, trying the user agent before the system daemon.
//
// Parameters:
//   - `name`: the launchd label.
//
// Returns:
//   - `Result`: the command result.
func (m *launchdManager) Enable(name string) Result {
	userPlist := "~/Library/LaunchAgents/" + name + ".plist"
	systemPlist := "/Library/LaunchDaemons/" + name + ".plist"

	result := runCommand([]string{"launchctl", "load", "-w", userPlist}, false)
	if result.OK {
		return result
	}
	return runCommand([]string{"launchctl", "load", "-w", systemPlist}, true)
}

// Exists reports whether a launchd job with the given label is loaded.
//
// Parameters:
//   - `name`: the launchd label.
//
// Returns:
//   - `bool`: true when the job appears in `launchctl list`.
func (m *launchdManager) Exists(name string) bool {
	// `| grep -q` in Go rather than in a shell: the pipe's right-hand side was a substring test, and a
	// substring test is strings.Contains.
	return strings.Contains(runCommand([]string{"launchctl", "list"}, false).Stdout, name)
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
//   - `Result`: the command result.
func (m *launchdManager) Start(name string) Result {
	return runCommand([]string{"launchctl", "start", name}, false)
}

// Status returns "running" when the named job is loaded, otherwise "stopped".
//
// Parameters:
//   - `name`: the launchd label.
//
// Returns:
//   - `string`: "running" or "stopped".
func (m *launchdManager) Status(name string) string {
	if runCommand([]string{"launchctl", "list", name}, false).OK {
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
//   - `Result`: the command result.
func (m *launchdManager) Stop(name string) Result {
	return runCommand([]string{"launchctl", "stop", name}, false)
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
	return runCommand([]string{"port", "info", name}, false).OK
}

// installRaw installs the named ports (MacPorts requires elevation).
//
// Parameters:
//   - `names`: the package names to install.
//   - `kwargs`: opaque native flags (unused by MacPorts).
//
// Returns:
//   - `Result`: the command result.
func (m *portManager) installRaw(names []string, _ map[string]any) Result {
	return runCommand(append([]string{"port", "install", "-N"}, names...), true)
}

// refresh updates MacPorts and synchronizes the ports tree, non-interactively.
//
// Requires elevation: MacPorts lives under /opt/local (root-owned), so selfupdate runs under sudo. `-N` keeps port
// non-interactive (accept defaults) so a refresh can't block on one of port's prompts.
//
// Returns:
//   - `Result`: the command result.
func (m *portManager) refresh() Result {
	return runCommand([]string{"port", "-N", "selfupdate"}, true)
}

// installed reports whether the named port is installed.
//
// Parameters:
//   - `name`: the package name to query.
//
// Returns:
//   - `bool`: true when `port installed` lists the package.
func (m *portManager) installed(name string) bool {
	// As with launchctl above: port reports "None of the specified ports are installed" on a miss, so the
	// name's presence in its own output is the answer.
	return strings.Contains(runCommand([]string{"port", "installed", name}, false).Stdout, name)
}

// removeRaw uninstalls the named ports.
//
// Parameters:
//   - `names`: the package names to uninstall.
//
// Returns:
//   - `Result`: the command result.
func (m *portManager) removeRaw(names []string) Result {
	return runCommand(append([]string{"port", "uninstall"}, names...), true)
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
	result := runCommand([]string{"port", "search", "--name", query}, false)
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
	result := runCommand([]string{"port", "installed", name}, false)
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
