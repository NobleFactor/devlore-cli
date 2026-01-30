// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build windows

package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// windowsHost implements Host for Windows.
//
// Design decisions:
//   - ADR-005: Windows Package Manager Choice
//     winget is the only supported package manager because:
//   - Ships with Windows 11 and recent Windows 10
//   - Microsoft-backed, long-term viability
//   - Store integration
type windowsHost struct {
	platform Platform
	pkgMgr   PackageManager
	svcMgr   ServiceManager
}

func newWindowsHost() Host {
	h := &windowsHost{}
	h.platform = h.detectPlatform()
	h.pkgMgr = h.detectPackageManager()
	h.svcMgr = &windowsServiceManager{}
	return h
}

func (h *windowsHost) Platform() Platform {
	return h.platform
}

func (h *windowsHost) detectPlatform() Platform {
	hostname, _ := os.Hostname()

	// Get Windows version
	version := ""
	if out, err := exec.Command("cmd", "/c", "ver").Output(); err == nil {
		version = strings.TrimSpace(string(out))
	}

	return Platform{
		OS:       "windows",
		Arch:     detectArch(),
		Distro:   "windows",
		Version:  version,
		Hostname: hostname,
	}
}

func (h *windowsHost) detectPackageManager() PackageManager {
	// ADR-005: winget is the only supported package manager
	return &wingetManager{}
}

func (h *windowsHost) PackageManager() PackageManager {
	return h.pkgMgr
}

func (h *windowsHost) ServiceManager() ServiceManager {
	return h.svcMgr
}

func (h *windowsHost) RunCommand(command string, sudo bool) Result {
	// Windows doesn't use sudo; admin operations use elevation
	// In a real implementation, we'd use runas or request UAC
	return runWindowsCommand(command, sudo)
}

func (h *windowsHost) ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(h.HomeDir(), path[2:])
	}
	// Expand Windows environment variables
	return os.ExpandEnv(path)
}

func (h *windowsHost) HomeDir() string {
	if home := os.Getenv("USERPROFILE"); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "C:\\Users\\" + os.Getenv("USERNAME")
}

// InstalledBy returns the package manager if the package is installed.
// Windows uses only winget, so this is trivial.
func (h *windowsHost) InstalledBy(name string) PackageManager {
	if h.pkgMgr != nil && h.pkgMgr.Installed(name) {
		return h.pkgMgr
	}
	return nil
}

// AllInstalledBy returns all package managers that have the package installed.
// Windows uses only winget, so this returns 0 or 1 items.
func (h *windowsHost) AllInstalledBy(name string) []PackageManager {
	if h.pkgMgr != nil && h.pkgMgr.Installed(name) {
		return []PackageManager{h.pkgMgr}
	}
	return nil
}

// GetPackageManager returns a specific package manager by name.
// Windows uses only winget, so this only returns the PM if name is "winget".
func (h *windowsHost) GetPackageManager(name string) PackageManager {
	if h.pkgMgr != nil && name == "winget" {
		return h.pkgMgr
	}
	return nil
}

// runWindowsCommand executes a command via PowerShell or cmd.
func runWindowsCommand(command string, elevated bool) Result {
	var cmd *exec.Cmd

	if elevated {
		// Use PowerShell with Start-Process for elevation
		psCmd := "Start-Process -Wait -Verb RunAs -FilePath 'cmd.exe' -ArgumentList '/c " + command + "'"
		cmd = exec.Command("powershell", "-Command", psCmd)
	} else {
		cmd = exec.Command("cmd", "/c", command)
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}

	return Result{
		OK:     code == 0,
		Stdout: strings.TrimSpace(stdout.String()),
		Stderr: strings.TrimSpace(stderr.String()),
		Code:   code,
	}
}

// =============================================================================
// winget Package Manager (Windows Package Manager)
// =============================================================================

type wingetManager struct{}

func (m *wingetManager) Name() string { return "winget" }

func (m *wingetManager) Installed(name string) bool {
	result := runWindowsCommand("winget list --id "+name, false)
	return result.OK && strings.Contains(result.Stdout, name)
}

func (m *wingetManager) Available(name string) bool {
	result := runWindowsCommand("winget show --id "+name, false)
	return result.OK
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
		// Skip header separator
		if strings.HasPrefix(line, "-") {
			inTable = true
			continue
		}
		if !inTable || line == "" {
			continue
		}
		// winget search output is columnar: Name  Id  Version  Source
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			sr := SearchResult{
				Name: fields[0],
			}
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
	// Parse version from winget list output
	// Format varies, but version is typically in a column
	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
		if strings.Contains(line, name) {
			fields := strings.Fields(line)
			// Version is typically the second-to-last or third field
			if len(fields) >= 2 {
				return fields[len(fields)-2]
			}
		}
	}
	return ""
}

func (m *wingetManager) Install(packages ...string) Result {
	args := make([]string, len(packages))
	for i, pkg := range packages {
		args[i] = "--id " + pkg
	}
	return runWindowsCommand("winget install --accept-source-agreements --accept-package-agreements "+strings.Join(args, " "), false)
}

func (m *wingetManager) Remove(name string) Result {
	return runWindowsCommand("winget uninstall --id "+name, false)
}

func (m *wingetManager) Update() Result {
	return runWindowsCommand("winget upgrade", false)
}

func (m *wingetManager) AddRepo(url, keyURL, name string) Result {
	return runWindowsCommand("winget source add --name "+name+" "+url, false)
}

func (m *wingetManager) NeedsSudo() bool { return false }

// =============================================================================
// Windows Service Manager (sc.exe)
// =============================================================================

type windowsServiceManager struct{}

func (m *windowsServiceManager) Exists(name string) bool {
	result := runWindowsCommand("sc query "+name, false)
	return result.OK
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

func (m *windowsServiceManager) Start(name string) Result {
	return runWindowsCommand("sc start "+name, true)
}

func (m *windowsServiceManager) Stop(name string) Result {
	return runWindowsCommand("sc stop "+name, true)
}

func (m *windowsServiceManager) Enable(name string) Result {
	return runWindowsCommand("sc config "+name+" start= auto", true)
}

func (m *windowsServiceManager) Disable(name string) Result {
	return runWindowsCommand("sc config "+name+" start= disabled", true)
}

func (m *windowsServiceManager) NeedsSudo() bool { return true }
