// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// runWindowsCommand executes a command via PowerShell or cmd, optionally elevated, and captures
// stdout/stderr/exit code into a [PlatformResult].
//
// Used by the [wingetManager] and [windowsServiceManager]. The command string is passed to `cmd /c`
// (non-elevated) or wrapped in `Start-Process -Verb RunAs` via PowerShell (elevated).
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

func (m *wingetManager) Installed(name string) bool {
	result := runWindowsCommand("winget list --id "+name, false)
	return result.OK && strings.Contains(result.Stdout, name)
}

func (m *wingetManager) Available(name string) bool {
	return runWindowsCommand("winget show --id "+name, false).OK
}

func (m *wingetManager) Search(query string, limit int) []SearchResult {
	result := runWindowsCommand("winget search "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	lines := strings.Split(result.Stdout, "\n")
	inTable := false
	for _, line := range lines {
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

func (m *wingetManager) Version(name string) string {
	result := runWindowsCommand("winget list --id "+name, false)
	if !result.OK {
		return ""
	}
	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
		if strings.Contains(line, name) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[len(fields)-2]
			}
		}
	}
	return ""
}

func (m *wingetManager) Install(packages ...string) PlatformResult {
	args := make([]string, len(packages))
	for i, pkg := range packages {
		args[i] = "--id " + pkg
	}
	return runWindowsCommand("winget install --accept-source-agreements --accept-package-agreements "+strings.Join(args, " "), false)
}

func (m *wingetManager) Remove(name string) PlatformResult {
	return runWindowsCommand("winget uninstall --id "+name, false)
}

func (m *wingetManager) Update() PlatformResult {
	return runWindowsCommand("winget upgrade", false)
}

func (m *wingetManager) AddRepo(url, _, name string) PlatformResult {
	return runWindowsCommand("winget source add --name "+name+" "+url, false)
}

func (m *wingetManager) NeedsSudo() bool { return false }

// =============================================================================
// Windows Service Manager (sc.exe)
// =============================================================================

type windowsServiceManager struct{}

func (m *windowsServiceManager) Exists(name string) bool {
	return runWindowsCommand("sc query "+name, false).OK
}

func (m *windowsServiceManager) IsRunning(name string) bool {
	result := runWindowsCommand("sc query "+name, false)
	return result.OK && strings.Contains(result.Stdout, "RUNNING")
}

func (m *windowsServiceManager) IsEnabled(name string) bool {
	result := runWindowsCommand("sc qc "+name, false)
	return result.OK && strings.Contains(result.Stdout, "AUTO_START")
}

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

func (m *windowsServiceManager) Start(name string) PlatformResult {
	return runWindowsCommand("sc start "+name, true)
}

func (m *windowsServiceManager) Stop(name string) PlatformResult {
	return runWindowsCommand("sc stop "+name, true)
}

func (m *windowsServiceManager) Enable(name string) PlatformResult {
	return runWindowsCommand("sc config "+name+" start= auto", true)
}

func (m *windowsServiceManager) Disable(name string) PlatformResult {
	return runWindowsCommand("sc config "+name+" start= disabled", true)
}

func (m *windowsServiceManager) NeedsSudo() bool { return true }
