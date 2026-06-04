// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build linux

package platform

import "strings"

// Real shell-out implementations for the Linux managers (apt, dnf, pacman, systemd). The implicit
// _linux.go build constraint scopes this file to Linux hosts; non-Linux hosts get the stub
// implementations from linux_managers_other.go.

// =============================================================================
// APT Package Manager — shell-out methods
// =============================================================================

func (m *aptManager) Installed(name string) bool {
	return runShellCommand("dpkg-query -W "+name, false).OK
}

func (m *aptManager) Available(name string) bool {
	return runShellCommand("apt-cache show "+name, false).OK
}

func (m *aptManager) Search(query string, limit int) []SearchResult {
	result := runShellCommand("apt-cache search "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
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

func (m *aptManager) Version(name string) string {
	result := runShellCommand("dpkg-query -W -f='${Version}' "+name, false)
	if !result.OK {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

func (m *aptManager) Install(packages ...string) PlatformResult {
	return runShellCommand("apt-get install -y "+strings.Join(packages, " "), true)
}

func (m *aptManager) Remove(name string) PlatformResult {
	return runShellCommand("apt-get remove -y "+name, true)
}

func (m *aptManager) Update() PlatformResult {
	return runShellCommand("apt-get update", true)
}

func (m *aptManager) AddRepo(url, keyURL, name string) PlatformResult {
	if keyURL != "" {
		keyResult := runShellCommand("curl -fsSL "+keyURL+" | gpg --dearmor -o /etc/apt/keyrings/"+name+".gpg", true)
		if !keyResult.OK {
			return keyResult
		}
	}
	repoFile := "/etc/apt/sources.list.d/" + name + ".list"
	content := "deb [signed-by=/etc/apt/keyrings/" + name + ".gpg] " + url + " " + name + " main"
	return runShellCommand("echo '"+content+"' > "+repoFile, true)
}

// =============================================================================
// DNF Package Manager — shell-out methods
// =============================================================================

func (m *dnfManager) Installed(name string) bool {
	return runShellCommand("rpm -q "+name, false).OK
}

func (m *dnfManager) Available(name string) bool {
	return runShellCommand("dnf info "+name, false).OK
}

func (m *dnfManager) Search(query string, limit int) []SearchResult {
	result := runShellCommand("dnf search "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
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

func (m *dnfManager) Version(name string) string {
	result := runShellCommand("rpm -q --queryformat '%{VERSION}' "+name, false)
	if !result.OK {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

func (m *dnfManager) Install(packages ...string) PlatformResult {
	return runShellCommand("dnf install -y "+strings.Join(packages, " "), true)
}

func (m *dnfManager) Remove(name string) PlatformResult {
	return runShellCommand("dnf remove -y "+name, true)
}

func (m *dnfManager) Update() PlatformResult {
	return runShellCommand("dnf check-update || true", true)
}

func (m *dnfManager) AddRepo(url, keyURL, _ string) PlatformResult {
	if keyURL != "" {
		keyResult := runShellCommand("rpm --import "+keyURL, true)
		if !keyResult.OK {
			return keyResult
		}
	}
	return runShellCommand("dnf config-manager --add-repo "+url, true)
}

// =============================================================================
// Pacman Package Manager — shell-out methods
// =============================================================================

func (m *pacmanManager) Installed(name string) bool {
	return runShellCommand("pacman -Q "+name, false).OK
}

func (m *pacmanManager) Available(name string) bool {
	return runShellCommand("pacman -Si "+name, false).OK
}

func (m *pacmanManager) Search(query string, limit int) []SearchResult { //nolint:gocognit // parsing format requires nesting

	result := runShellCommand("pacman -Ss "+query, false)

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

func (m *pacmanManager) Version(name string) string {

	result := runShellCommand("pacman -Q "+name, false)

	if !result.OK {
		return ""
	}

	parts := strings.Fields(result.Stdout)

	if len(parts) >= 2 {
		return parts[1]
	}

	return ""
}

func (m *pacmanManager) Install(packages ...string) PlatformResult {

	return runShellCommand("pacman -S --noconfirm --needed "+strings.Join(packages, " "), true)
}

func (m *pacmanManager) Remove(name string) PlatformResult {

	return runShellCommand("pacman -R --noconfirm "+name, true)
}

func (m *pacmanManager) Update() PlatformResult {

	return runShellCommand("pacman -Sy", true)
}

func (m *pacmanManager) AddRepo(url, keyURL, name string) PlatformResult {
	if keyURL != "" {
		keyResult := runShellCommand("pacman-key --recv-keys "+keyURL+" && pacman-key --lsign-key "+keyURL, true)
		if !keyResult.OK {
			return keyResult
		}
	}
	repoEntry := "\n[" + name + "]\nServer = " + url + "\n"
	return runShellCommand("echo '"+repoEntry+"' >> /etc/pacman.conf", true)
}

// =============================================================================
// systemd Service Manager — shell-out methods
// =============================================================================

func (m *systemdManager) Exists(name string) bool {
	return runShellCommand("systemctl cat "+name, false).OK
}

func (m *systemdManager) IsRunning(name string) bool {
	return runShellCommand("systemctl is-active --quiet "+name, false).OK
}

func (m *systemdManager) IsEnabled(name string) bool {
	return runShellCommand("systemctl is-enabled --quiet "+name, false).OK
}

func (m *systemdManager) Status(name string) string {
	result := runShellCommand("systemctl is-active "+name, false)
	return strings.TrimSpace(result.Stdout)
}

func (m *systemdManager) Start(name string) PlatformResult {
	return runShellCommand("systemctl start "+name, true)
}

func (m *systemdManager) Stop(name string) PlatformResult {
	return runShellCommand("systemctl stop "+name, true)
}

func (m *systemdManager) Enable(name string) PlatformResult {
	return runShellCommand("systemctl enable "+name, true)
}

func (m *systemdManager) Disable(name string) PlatformResult {
	return runShellCommand("systemctl disable "+name, true)
}
