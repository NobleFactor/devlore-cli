// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build linux

package platform

import "strings"

// Real shell-out primitives for the cross-distro Linux managers (snap, flatpak). Build-tagged linux because the
// underlying tools (snapd, flatpak) only run on Linux; non-Linux hosts get the stub primitives from
// cross_distro_managers_other.go. The exported [PackageManager] surface is assembled from these primitives by the
// embedded [driver] (see cross_distro_managers.go).

// =============================================================================
// flatpak Package Manager — shell-out primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports whether the named app exists on the flathub remote.
//
// Parameters:
//   - `name`: the application id to query.
//
// Returns:
//   - `bool`: true when `flatpak remote-info` resolves the app.
func (m *flatpakManager) available(name string) bool {
	return runShellCommand("flatpak remote-info flathub "+name, false).OK
}

// installRaw installs the named apps from flathub.
//
// Parameters:
//   - `names`: the application ids to install.
//   - `kwargs`: opaque native flags (unused by flatpak).
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *flatpakManager) installRaw(names []string, _ map[string]any) PlatformResult {
	return runShellCommand("flatpak install -y flathub "+strings.Join(names, " "), false)
}

// installed reports whether the named app is installed.
//
// Parameters:
//   - `name`: the application id to query.
//
// Returns:
//   - `bool`: true when `flatpak info` resolves the app.
func (m *flatpakManager) installed(name string) bool {
	return runShellCommand("flatpak info "+name, false).OK
}

// removeRaw uninstalls the named apps.
//
// Parameters:
//   - `names`: the application ids to uninstall.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *flatpakManager) removeRaw(names []string) PlatformResult {
	return runShellCommand("flatpak uninstall -y "+strings.Join(names, " "), false)
}

// searchRaw returns up to `limit` apps matching `query`.
//
// Parameters:
//   - `query`: the search term.
//   - `limit`: the maximum number of results; <= 0 means no limit.
//
// Returns:
//   - `[]SearchResult`: the matches, or nil on failure.
func (m *flatpakManager) searchRaw(query string, limit int) []SearchResult {
	result := runShellCommand("flatpak search "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	for _, line := range strings.Split(result.Stdout, "\n") {
		if line == "" {
			continue
		}
		// flatpak search output is tab-separated: Name Description ApplicationID Version Branch Remotes.
		fields := strings.Split(line, "\t")
		if len(fields) >= 1 && fields[0] != "" {
			sr := SearchResult{Name: strings.TrimSpace(fields[0])}
			if len(fields) >= 2 {
				sr.Description = strings.TrimSpace(fields[1])
			}
			if len(fields) >= 4 {
				sr.Version = strings.TrimSpace(fields[3])
			}
			results = append(results, sr)
			if limit > 0 && len(results) >= limit {
				return results
			}
		}
	}
	return results
}

// version returns the installed version of the named app, or "" when it is not installed.
//
// Parameters:
//   - `name`: the application id to query.
//
// Returns:
//   - `string`: the installed version, or "".
func (m *flatpakManager) version(name string) string {
	result := runShellCommand("flatpak info --show-version "+name, false)
	if !result.OK {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

// endregion

// endregion

// =============================================================================
// snap Package Manager — shell-out primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports whether the named snap exists in the store.
//
// Parameters:
//   - `name`: the snap name to query.
//
// Returns:
//   - `bool`: true when `snap info` resolves the snap.
func (m *snapManager) available(name string) bool {
	return runShellCommand("snap info "+name, false).OK
}

// installRaw installs the named snaps.
//
// Parameters:
//   - `names`: the snap names to install.
//   - `kwargs`: opaque native flags (unused by snap).
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *snapManager) installRaw(names []string, _ map[string]any) PlatformResult {
	return runShellCommand("snap install "+strings.Join(names, " "), true)
}

// installed reports whether the named snap is installed.
//
// Parameters:
//   - `name`: the snap name to query.
//
// Returns:
//   - `bool`: true when `snap list` resolves the snap.
func (m *snapManager) installed(name string) bool {
	return runShellCommand("snap list "+name, false).OK
}

// removeRaw uninstalls the named snaps.
//
// Parameters:
//   - `names`: the snap names to uninstall.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *snapManager) removeRaw(names []string) PlatformResult {
	return runShellCommand("snap remove "+strings.Join(names, " "), true)
}

// searchRaw returns up to `limit` snaps matching `query`.
//
// Parameters:
//   - `query`: the search term.
//   - `limit`: the maximum number of results; <= 0 means no limit.
//
// Returns:
//   - `[]SearchResult`: the matches, or nil on failure.
func (m *snapManager) searchRaw(query string, limit int) []SearchResult {
	result := runShellCommand("snap find "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	for i, line := range strings.Split(result.Stdout, "\n") {
		// First line is the "Name Version Publisher Notes Summary" header; skip it.
		if i == 0 || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			sr := SearchResult{Name: fields[0]}
			if len(fields) >= 2 {
				sr.Version = fields[1]
			}
			results = append(results, sr)
			if limit > 0 && len(results) >= limit {
				return results
			}
		}
	}
	return results
}

// version returns the installed version of the named snap, or "" when it is not installed.
//
// Parameters:
//   - `name`: the snap name to query.
//
// Returns:
//   - `string`: the installed version, or "".
func (m *snapManager) version(name string) string {
	result := runShellCommand("snap list "+name, false)
	if !result.OK {
		return ""
	}
	for i, line := range strings.Split(result.Stdout, "\n") {
		if i == 0 || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == name {
			return fields[1]
		}
	}
	return ""
}

// endregion

// endregion
