// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

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
//     winget is preferred over Chocolatey because:
//     - Ships with Windows 11 and recent Windows 10
//     - Microsoft-backed, long-term viability
//     - Store integration
//     - Chocolatey remains as legacy fallback
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
	// ADR-005: Prefer winget (modern) over choco (legacy)
	if _, err := exec.LookPath("winget"); err == nil {
		return &wingetManager{}
	}
	if _, err := exec.LookPath("choco"); err == nil {
		return &chocoManager{}
	}
	return &wingetManager{} // Default, will fail gracefully
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
// Chocolatey Package Manager (Legacy)
// =============================================================================

type chocoManager struct{}

func (m *chocoManager) Name() string { return "choco" }

func (m *chocoManager) Installed(name string) bool {
	result := runWindowsCommand("choco list --local-only "+name, false)
	return result.OK && strings.Contains(result.Stdout, name)
}

func (m *chocoManager) Version(name string) string {
	result := runWindowsCommand("choco list --local-only "+name, false)
	if !result.OK {
		return ""
	}
	// Parse version from choco list output: "package version"
	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, name+" ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return ""
}

func (m *chocoManager) Install(packages ...string) Result {
	return runWindowsCommand("choco install -y "+strings.Join(packages, " "), false)
}

func (m *chocoManager) Remove(name string) Result {
	return runWindowsCommand("choco uninstall -y "+name, false)
}

func (m *chocoManager) Update() Result {
	return runWindowsCommand("choco outdated", false)
}

func (m *chocoManager) AddRepo(url, keyURL, name string) Result {
	return runWindowsCommand("choco source add --name="+name+" --source="+url, false)
}

func (m *chocoManager) NeedsSudo() bool { return false }

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
