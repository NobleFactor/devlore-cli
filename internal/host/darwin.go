// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build darwin

package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// darwinHost implements Host for macOS.
type darwinHost struct {
	platform Platform
	pkgMgr   PackageManager
	svcMgr   ServiceManager
}

func newDarwinHost() Host {
	h := &darwinHost{}
	h.platform = h.detectPlatform()
	h.pkgMgr = h.detectPackageManager()
	h.svcMgr = &launchdManager{}
	return h
}

func (h *darwinHost) Platform() Platform {
	return h.platform
}

func (h *darwinHost) detectPlatform() Platform {
	hostname, _ := os.Hostname()

	// Get macOS version from sw_vers
	version := ""
	if out, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
		version = strings.TrimSpace(string(out))
	}

	return Platform{
		OS:       "darwin",
		Arch:     detectArch(),
		Distro:   "macos",
		Version:  version,
		Hostname: hostname,
	}
}

func (h *darwinHost) detectPackageManager() PackageManager {
	// MacPorts has priority over Homebrew
	if _, err := exec.LookPath("port"); err == nil {
		return &portManager{}
	}
	if _, err := exec.LookPath("brew"); err == nil {
		return &brewManager{}
	}
	return nil // No package manager installed
}

func (h *darwinHost) PackageManager() PackageManager {
	return h.pkgMgr
}

func (h *darwinHost) ServiceManager() ServiceManager {
	return h.svcMgr
}

func (h *darwinHost) RunCommand(command string, sudo bool) Result {
	return runShellCommand(command, sudo)
}

func (h *darwinHost) ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(h.HomeDir(), path[2:])
	}
	return path
}

func (h *darwinHost) HomeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/Users/" + os.Getenv("USER")
}

// =============================================================================
// Homebrew Package Manager
// =============================================================================

type brewManager struct{}

func (m *brewManager) Name() string { return "brew" }

func (m *brewManager) Installed(name string) bool {
	result := runShellCommand("brew list "+name, false)
	return result.OK
}

func (m *brewManager) Available(name string) bool {
	result := runShellCommand("brew info "+name, false)
	return result.OK
}

func (m *brewManager) Search(query string, limit int) []SearchResult {
	result := runShellCommand("brew search "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "==>") {
			continue
		}
		// brew search returns package names, one per line (or space-separated)
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

func (m *brewManager) Version(name string) string {
	result := runShellCommand("brew list --versions "+name, false)
	if !result.OK {
		return ""
	}
	// Output: "package 1.2.3"
	parts := strings.Fields(result.Stdout)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func (m *brewManager) Install(packages ...string) Result {
	return runShellCommand("brew install "+strings.Join(packages, " "), false)
}

func (m *brewManager) Remove(name string) Result {
	return runShellCommand("brew uninstall "+name, false)
}

func (m *brewManager) Update() Result {
	return runShellCommand("brew update", false)
}

func (m *brewManager) AddRepo(url, keyURL, name string) Result {
	// Homebrew uses taps
	return runShellCommand("brew tap "+name, false)
}

func (m *brewManager) NeedsSudo() bool { return false }

// =============================================================================
// MacPorts Package Manager
// =============================================================================

type portManager struct{}

func (m *portManager) Name() string { return "port" }

func (m *portManager) Installed(name string) bool {
	result := runShellCommand("port installed "+name+" | grep -q "+name, false)
	return result.OK
}

func (m *portManager) Available(name string) bool {
	result := runShellCommand("port info "+name, false)
	return result.OK
}

func (m *portManager) Search(query string, limit int) []SearchResult {
	result := runShellCommand("port search --name "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
		// port search output: "name @version (category)\n    description"
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

func (m *portManager) Version(name string) string {
	result := runShellCommand("port installed "+name, false)
	if !result.OK {
		return ""
	}
	// Parse version from output
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

func (m *portManager) Install(packages ...string) Result {
	return runShellCommand("port install -N "+strings.Join(packages, " "), true)
}

func (m *portManager) Remove(name string) Result {
	return runShellCommand("port uninstall "+name, true)
}

func (m *portManager) Update() Result {
	return runShellCommand("port selfupdate", true)
}

func (m *portManager) AddRepo(url, keyURL, name string) Result {
	// MacPorts doesn't have dynamic repo addition
	return Result{OK: false, Stderr: "MacPorts doesn't support dynamic repository addition"}
}

func (m *portManager) NeedsSudo() bool { return true }

// =============================================================================
// launchd Service Manager
// =============================================================================

type launchdManager struct{}

func (m *launchdManager) Exists(name string) bool {
	result := runShellCommand("launchctl list | grep -q "+name, false)
	return result.OK
}

func (m *launchdManager) Status(name string) string {
	result := runShellCommand("launchctl list "+name, false)
	if result.OK {
		return "running"
	}
	return "stopped"
}

func (m *launchdManager) Start(name string) Result {
	return runShellCommand("launchctl start "+name, false)
}

func (m *launchdManager) Stop(name string) Result {
	return runShellCommand("launchctl stop "+name, false)
}

func (m *launchdManager) Enable(name string) Result {
	// Try user agent first, then system daemon
	userPlist := "~/Library/LaunchAgents/" + name + ".plist"
	systemPlist := "/Library/LaunchDaemons/" + name + ".plist"

	result := runShellCommand("launchctl load -w "+userPlist, false)
	if result.OK {
		return result
	}
	return runShellCommand("launchctl load -w "+systemPlist, true)
}

func (m *launchdManager) Disable(name string) Result {
	userPlist := "~/Library/LaunchAgents/" + name + ".plist"
	systemPlist := "/Library/LaunchDaemons/" + name + ".plist"

	result := runShellCommand("launchctl unload -w "+userPlist, false)
	if result.OK {
		return result
	}
	return runShellCommand("launchctl unload -w "+systemPlist, true)
}

func (m *launchdManager) NeedsSudo() bool {
	// User agents don't need sudo, system daemons do
	return false // Default to user agent
}
