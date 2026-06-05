// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build windows

package platform

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Real shell-out primitives for the Windows managers (winget, sc.exe). The implicit _windows.go build constraint
// scopes this file to Windows hosts; non-Windows hosts get the stub primitives from windows_managers_other.go. The
// exported [PackageManager] surface is assembled from these primitives by the embedded [driver] (see
// windows_managers.go).

// =============================================================================
// Windows Service Manager — shell-out methods
// =============================================================================

// region EXPORTED METHODS

// region Behaviors

// Disable sets the named service to disabled start.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *windowsServiceManager) Disable(name string) PlatformResult {
	return runWindowsCommand("sc config "+name+" start= disabled", true)
}

// Enable sets the named service to automatic start.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *windowsServiceManager) Enable(name string) PlatformResult {
	return runWindowsCommand("sc config "+name+" start= auto", true)
}

// Exists reports whether the named service is registered.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `bool`: true when `sc query` resolves the service.
func (m *windowsServiceManager) Exists(name string) bool {
	return runWindowsCommand("sc query "+name, false).OK
}

// IsEnabled reports whether the named service is set to automatic start.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `bool`: true when `sc qc` reports AUTO_START.
func (m *windowsServiceManager) IsEnabled(name string) bool {
	result := runWindowsCommand("sc qc "+name, false)
	return result.OK && strings.Contains(result.Stdout, "AUTO_START")
}

// IsRunning reports whether the named service is currently running.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `bool`: true when `sc query` reports RUNNING.
func (m *windowsServiceManager) IsRunning(name string) bool {
	result := runWindowsCommand("sc query "+name, false)
	return result.OK && strings.Contains(result.Stdout, "RUNNING")
}

// Start starts the named service.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *windowsServiceManager) Start(name string) PlatformResult {
	return runWindowsCommand("sc start "+name, true)
}

// Status returns "running", "stopped", "not-found", or "unknown" for the named service.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `string`: the coarse service status.
func (m *windowsServiceManager) Status(name string) string {
	result := runWindowsCommand("sc query "+name, false)
	if !result.OK {
		return "not-found"
	}
	if strings.Contains(result.Stdout, "RUNNING") {
		return "running"
	}
	if strings.Contains(result.Stdout, "STOPPED") {
		return "stopped"
	}
	return "unknown"
}

// Stop stops the named service.
//
// Parameters:
//   - `name`: the service name.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *windowsServiceManager) Stop(name string) PlatformResult {
	return runWindowsCommand("sc stop "+name, true)
}

// endregion

// endregion

// =============================================================================
// winget Package Manager — shell-out primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports whether the named package exists in the winget catalog.
//
// Parameters:
//   - `name`: the winget id to query.
//
// Returns:
//   - `bool`: true when `winget show` resolves the package.
func (m *wingetManager) available(name string) bool {
	return runWindowsCommand("winget show --id "+name, false).OK
}

// installRaw installs the named packages by id, accepting source and package agreements.
//
// Parameters:
//   - `names`: the winget ids to install.
//   - `kwargs`: opaque native flags (unused by winget).
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *wingetManager) installRaw(names []string, _ map[string]any) PlatformResult {
	args := make([]string, len(names))
	for i, name := range names {
		args[i] = "--id " + name
	}
	return runWindowsCommand("winget install --accept-source-agreements --accept-package-agreements "+strings.Join(args, " "), false)
}

// installed reports whether the named package is installed.
//
// Parameters:
//   - `name`: the winget id to query.
//
// Returns:
//   - `bool`: true when `winget list` lists the id.
func (m *wingetManager) installed(name string) bool {
	result := runWindowsCommand("winget list --id "+name, false)
	return result.OK && strings.Contains(result.Stdout, name)
}

// removeRaw uninstalls the named packages by id.
//
// Parameters:
//   - `names`: the winget ids to uninstall.
//
// Returns:
//   - `PlatformResult`: the command result.
func (m *wingetManager) removeRaw(names []string) PlatformResult {
	args := make([]string, len(names))
	for i, name := range names {
		args[i] = "--id " + name
	}
	return runWindowsCommand("winget uninstall "+strings.Join(args, " "), false)
}

// searchRaw returns up to `limit` packages matching `query`.
//
// Parameters:
//   - `query`: the search term.
//   - `limit`: the maximum number of results; <= 0 means no limit.
//
// Returns:
//   - `[]SearchResult`: the matches, or nil on failure.
func (m *wingetManager) searchRaw(query string, limit int) []SearchResult {
	result := runWindowsCommand("winget search "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	inTable := false
	for _, line := range strings.Split(result.Stdout, "\n") {
		if strings.HasPrefix(line, "-") {
			inTable = true
			continue
		}
		if !inTable || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			sr := SearchResult{Name: fields[0]}
			if len(fields) >= 3 {
				sr.Version = fields[2]
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
//   - `name`: the winget id to query.
//
// Returns:
//   - `string`: the installed version, or "".
func (m *wingetManager) version(name string) string {
	result := runWindowsCommand("winget list --id "+name, false)
	if !result.OK {
		return ""
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		if strings.Contains(line, name) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[len(fields)-2]
			}
		}
	}
	return ""
}

// endregion

// endregion

// region HELPER FUNCTIONS

// runWindowsCommand executes a command via PowerShell or cmd, optionally elevated, capturing the result.
//
// It lives in the windows-tagged file because there is no useful semantics for it on non-Windows hosts
// (PowerShell and cmd.exe are absent). Used by both the winget and Service Control Manager primitives above.
//
// Parameters:
//   - `command`: the command line to run.
//   - `elevated`: when true, relaunch the command elevated via `Start-Process -Verb RunAs`.
//
// Returns:
//   - `PlatformResult`: the captured stdout/stderr/exit code.
func runWindowsCommand(command string, elevated bool) PlatformResult {

	var cmd *exec.Cmd

	if elevated {
		psCmd := "Start-Process -Wait -Verb RunAs -FilePath 'cmd.exe' -ArgumentList '/c " + command + "'"
		cmd = exec.CommandContext(context.Background(), "powershell", "-Command", psCmd) //nolint:gosec // G204: shell command from internal caller
	} else {
		cmd = exec.CommandContext(context.Background(), "cmd", "/c", command) //nolint:gosec // G204: shell command from internal caller
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}

	return PlatformResult{
		OK:     code == 0,
		Stdout: strings.TrimSpace(stdout.String()),
		Stderr: strings.TrimSpace(stderr.String()),
		Code:   code,
	}
}

// endregion
