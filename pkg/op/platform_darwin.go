// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build darwin

package op

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

func newDarwin() *Platform {
	hostname, _ := os.Hostname() //nolint:errcheck // best effort

	version := ""
	if out, err := exec.CommandContext(context.Background(), "sw_vers", "-productVersion").Output(); err == nil {
		version = strings.TrimSpace(string(out))
	}

	// Detect package managers
	var brew *brewManager
	var port *portManager
	if _, err := exec.LookPath("brew"); err == nil {
		brew = &brewManager{}
	}
	if _, err := exec.LookPath("port"); err == nil {
		port = &portManager{}
	}

	// Preferred package manager: port > brew.
	var preferred PackageManager
	packageManagers := make(map[string]PackageManager)
	if port != nil {
		preferred = port
		packageManagers["port"] = port
	}
	if brew != nil {
		if preferred == nil {
			preferred = brew
		}
		packageManagers["brew"] = brew
	}

	return &Platform{
		OS:              "darwin",
		Arch:            detectArch(),
		Distro:          "macos",
		Version:         version,
		Hostname:        hostname,
		PackageManager:  preferred,
		PackageManagers: packageManagers,
		ServiceManager:  &launchdManager{},
	}
}

// =============================================================================
// Homebrew Package Manager
// =============================================================================

type brewManager struct{}

func (m *brewManager) Name() string { return "brew" }

func (m *brewManager) Installed(name string) bool {
	if runShellCommand("brew list --formula "+name, false).OK {
		return true
	}
	return runShellCommand("brew list --cask "+name, false).OK
}

func (m *brewManager) Available(name string) bool {
	return runShellCommand("brew info "+name, false).OK
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
	parts := strings.Fields(result.Stdout)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func (m *brewManager) Install(packages ...string) PlatformResult {
	return runShellCommand("brew install "+strings.Join(packages, " "), false)
}

func (m *brewManager) Remove(name string) PlatformResult {
	return runShellCommand("brew uninstall "+name, false)
}

func (m *brewManager) Update() PlatformResult {
	return runShellCommand("brew update", false)
}

func (m *brewManager) AddRepo(url, keyURL, name string) PlatformResult {
	return runShellCommand("brew tap "+name, false)
}

func (m *brewManager) NeedsSudo() bool { return false }

// =============================================================================
// MacPorts Package Manager
// =============================================================================

type portManager struct{}

func (m *portManager) Name() string { return "port" }

func (m *portManager) Installed(name string) bool {
	return runShellCommand("port installed "+name+" | grep -q "+name, false).OK
}

func (m *portManager) Available(name string) bool {
	return runShellCommand("port info "+name, false).OK
}

func (m *portManager) Search(query string, limit int) []SearchResult {
	result := runShellCommand("port search --name "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
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

func (m *portManager) Install(packages ...string) PlatformResult {
	return runShellCommand("port install -N "+strings.Join(packages, " "), true)
}

func (m *portManager) Remove(name string) PlatformResult {
	return runShellCommand("port uninstall "+name, true)
}

func (m *portManager) Update() PlatformResult {
	return runShellCommand("port selfupdate", true)
}

func (m *portManager) AddRepo(_, _, _ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "MacPorts doesn't support dynamic repository addition"}
}

func (m *portManager) NeedsSudo() bool { return true }

// =============================================================================
// launchd Service Manager
// =============================================================================

type launchdManager struct{}

func (m *launchdManager) Exists(name string) bool {
	return runShellCommand("launchctl list | grep -q "+name, false).OK
}

func (m *launchdManager) IsRunning(name string) bool {
	out, err := exec.CommandContext(context.Background(), "launchctl", "list", name).Output() //nolint:gosec // G204: command built from provider inputs
	if err != nil {
		return false
	}
	fields := strings.Fields(string(out))
	return len(fields) > 0 && fields[0] != "-"
}

func (m *launchdManager) IsEnabled(_ string) bool {
	return false
}

func (m *launchdManager) Status(name string) string {
	if runShellCommand("launchctl list "+name, false).OK {
		return "running"
	}
	return "stopped"
}

func (m *launchdManager) Start(name string) PlatformResult {
	return runShellCommand("launchctl start "+name, false)
}

func (m *launchdManager) Stop(name string) PlatformResult {
	return runShellCommand("launchctl stop "+name, false)
}

func (m *launchdManager) Enable(name string) PlatformResult {
	userPlist := "~/Library/LaunchAgents/" + name + ".plist"
	systemPlist := "/Library/LaunchDaemons/" + name + ".plist"

	result := runShellCommand("launchctl load -w "+userPlist, false)
	if result.OK {
		return result
	}
	return runShellCommand("launchctl load -w "+systemPlist, true)
}

func (m *launchdManager) Disable(name string) PlatformResult {
	userPlist := "~/Library/LaunchAgents/" + name + ".plist"
	systemPlist := "/Library/LaunchDaemons/" + name + ".plist"

	result := runShellCommand("launchctl unload -w "+userPlist, false)
	if result.OK {
		return result
	}
	return runShellCommand("launchctl unload -w "+systemPlist, true)
}

func (m *launchdManager) NeedsSudo() bool { return false }
