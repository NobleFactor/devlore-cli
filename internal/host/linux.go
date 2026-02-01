// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build linux

package host

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// linuxHost implements Host for Linux distributions.
type linuxHost struct {
	platform Platform
	pkgMgr   PackageManager
	svcMgr   ServiceManager
}

func newLinuxHost() Host {
	h := &linuxHost{}
	h.platform = h.detectPlatform()
	h.pkgMgr = h.detectPackageManager()
	h.svcMgr = &systemdManager{}
	return h
}

func (h *linuxHost) Platform() Platform {
	return h.platform
}

func (h *linuxHost) detectPlatform() Platform {
	hostname, _ := os.Hostname()

	distro, version := detectLinuxDistro()

	return Platform{
		OS:       "linux",
		Arch:     detectArch(),
		Distro:   distro,
		Version:  version,
		Hostname: hostname,
	}
}

// detectLinuxDistro reads /etc/os-release to determine distribution.
func detectLinuxDistro() (distro, version string) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return "linux", ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			distro = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		}
		if strings.HasPrefix(line, "VERSION_ID=") {
			version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		}
	}

	if distro == "" {
		distro = "linux"
	}
	return distro, version
}

func (h *linuxHost) detectPackageManager() PackageManager {
	// Detect based on distro or binary availability
	// Supported: apt (Debian/Ubuntu), dnf (Fedora/RHEL), pacman (Arch)
	switch h.platform.Distro {
	case "debian", "ubuntu", "linuxmint", "pop", "elementary":
		return &aptManager{}
	case "fedora", "rhel", "centos", "rocky", "alma":
		return &dnfManager{}
	case "arch", "manjaro", "endeavouros", "garuda", "artix":
		return &pacmanManager{}
	}

	// Fallback: detect by binary
	if _, err := exec.LookPath("pacman"); err == nil {
		return &pacmanManager{}
	}
	if _, err := exec.LookPath("apt"); err == nil {
		return &aptManager{}
	}
	if _, err := exec.LookPath("dnf"); err == nil {
		return &dnfManager{}
	}
	return &aptManager{} // Default
}

func (h *linuxHost) PackageManager() PackageManager {
	return h.pkgMgr
}

func (h *linuxHost) ServiceManager() ServiceManager {
	return h.svcMgr
}

func (h *linuxHost) RunCommand(command string, sudo bool) Result {
	return runShellCommand(command, sudo)
}

func (h *linuxHost) ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(h.HomeDir(), path[2:])
	}
	return path
}

func (h *linuxHost) HomeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/home/" + os.Getenv("USER")
}

// InstalledBy returns the package manager if the package is installed.
// Linux has a single PM per distribution, so this is trivial.
func (h *linuxHost) InstalledBy(name string) PackageManager {
	if h.pkgMgr != nil && h.pkgMgr.Installed(name) {
		return h.pkgMgr
	}
	return nil
}

// AllInstalledBy returns all package managers that have the package installed.
// Linux has a single PM per distribution, so this returns 0 or 1 items.
func (h *linuxHost) AllInstalledBy(name string) []PackageManager {
	if h.pkgMgr != nil && h.pkgMgr.Installed(name) {
		return []PackageManager{h.pkgMgr}
	}
	return nil
}

// GetPackageManager returns a specific package manager by name.
// Linux has a single PM per distribution, so this only returns the detected PM
// if the name matches (e.g., "apt" on Debian, "dnf" on Fedora).
func (h *linuxHost) GetPackageManager(name string) PackageManager {
	if h.pkgMgr != nil && h.pkgMgr.Name() == name {
		return h.pkgMgr
	}
	return nil
}

// =============================================================================
// APT Package Manager (Debian, Ubuntu)
// =============================================================================

type aptManager struct{}

func (m *aptManager) Name() string { return "apt" }

func (m *aptManager) Installed(name string) bool {
	result := runShellCommand("dpkg-query -W "+name, false)
	return result.OK
}

func (m *aptManager) Available(name string) bool {
	result := runShellCommand("apt-cache show "+name, false)
	return result.OK
}

func (m *aptManager) Search(query string, limit int) []SearchResult {
	result := runShellCommand("apt-cache search "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
		// apt-cache search output: "package - description"
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

func (m *aptManager) Install(packages ...string) Result {
	return runShellCommand("apt-get install -y "+strings.Join(packages, " "), true)
}

func (m *aptManager) Remove(name string) Result {
	return runShellCommand("apt-get remove -y "+name, true)
}

func (m *aptManager) Update() Result {
	return runShellCommand("apt-get update", true)
}

func (m *aptManager) AddRepo(url, keyURL, name string) Result {
	// Add GPG key if provided
	if keyURL != "" {
		keyResult := runShellCommand("curl -fsSL "+keyURL+" | gpg --dearmor -o /etc/apt/keyrings/"+name+".gpg", true)
		if !keyResult.OK {
			return keyResult
		}
	}
	// Add repository
	repoFile := "/etc/apt/sources.list.d/" + name + ".list"
	content := "deb [signed-by=/etc/apt/keyrings/" + name + ".gpg] " + url + " " + name + " main"
	return runShellCommand("echo '"+content+"' > "+repoFile, true)
}

func (m *aptManager) NeedsSudo() bool { return true }

// =============================================================================
// DNF Package Manager (Fedora, RHEL)
// =============================================================================

type dnfManager struct{}

func (m *dnfManager) Name() string { return "dnf" }

func (m *dnfManager) Installed(name string) bool {
	result := runShellCommand("rpm -q "+name, false)
	return result.OK
}

func (m *dnfManager) Available(name string) bool {
	result := runShellCommand("dnf info "+name, false)
	return result.OK
}

func (m *dnfManager) Search(query string, limit int) []SearchResult {
	result := runShellCommand("dnf search "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
		// Skip header lines
		if strings.HasPrefix(line, "=") || strings.HasPrefix(line, "Last metadata") || line == "" {
			continue
		}
		// dnf search output: "name.arch : description"
		parts := strings.SplitN(line, " : ", 2)
		if len(parts) >= 1 {
			namePart := strings.TrimSpace(parts[0])
			// Remove .arch suffix
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

func (m *dnfManager) Install(packages ...string) Result {
	return runShellCommand("dnf install -y "+strings.Join(packages, " "), true)
}

func (m *dnfManager) Remove(name string) Result {
	return runShellCommand("dnf remove -y "+name, true)
}

func (m *dnfManager) Update() Result {
	// dnf check-update returns 100 if updates available
	return runShellCommand("dnf check-update || true", true)
}

func (m *dnfManager) AddRepo(url, keyURL, name string) Result {
	// Import GPG key if provided
	if keyURL != "" {
		keyResult := runShellCommand("rpm --import "+keyURL, true)
		if !keyResult.OK {
			return keyResult
		}
	}
	// Add repository using dnf config-manager
	return runShellCommand("dnf config-manager --add-repo "+url, true)
}

func (m *dnfManager) NeedsSudo() bool { return true }

// =============================================================================
// Pacman Package Manager (Arch, Manjaro, EndeavourOS)
// =============================================================================

type pacmanManager struct{}

func (m *pacmanManager) Name() string { return "pacman" }

func (m *pacmanManager) Installed(name string) bool {
	result := runShellCommand("pacman -Q "+name, false)
	return result.OK
}

func (m *pacmanManager) Available(name string) bool {
	result := runShellCommand("pacman -Si "+name, false)
	return result.OK
}

func (m *pacmanManager) Search(query string, limit int) []SearchResult {
	result := runShellCommand("pacman -Ss "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	lines := strings.Split(result.Stdout, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		// pacman -Ss output format:
		// repo/package version
		//     Description text
		if strings.HasPrefix(line, " ") {
			// This is a description line, skip
			continue
		}
		// Parse "repo/package version" line
		parts := strings.Fields(line)
		if len(parts) >= 1 {
			repoAndName := parts[0]
			// Extract package name from "repo/name"
			if idx := strings.Index(repoAndName, "/"); idx >= 0 {
				pkgName := repoAndName[idx+1:]
				sr := SearchResult{Name: pkgName}
				// Get description from next line if available
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
	// Output format: "package version"
	parts := strings.Fields(result.Stdout)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func (m *pacmanManager) Install(packages ...string) Result {
	// --noconfirm for non-interactive install, --needed to skip already installed
	return runShellCommand("pacman -S --noconfirm --needed "+strings.Join(packages, " "), true)
}

func (m *pacmanManager) Remove(name string) Result {
	// --noconfirm for non-interactive removal
	return runShellCommand("pacman -R --noconfirm "+name, true)
}

func (m *pacmanManager) Update() Result {
	// -Sy syncs package database
	return runShellCommand("pacman -Sy", true)
}

func (m *pacmanManager) AddRepo(url, keyURL, name string) Result {
	// Arch uses /etc/pacman.conf for repository configuration
	// Adding a repo involves:
	// 1. Optionally import GPG key
	// 2. Add [repo] section to /etc/pacman.conf

	// Import GPG key if provided
	if keyURL != "" {
		keyResult := runShellCommand("pacman-key --recv-keys "+keyURL+" && pacman-key --lsign-key "+keyURL, true)
		if !keyResult.OK {
			return keyResult
		}
	}

	// Add repository to pacman.conf
	// Note: This is a simplified implementation. Real repos may need more config.
	repoEntry := "\n[" + name + "]\nServer = " + url + "\n"
	return runShellCommand("echo '"+repoEntry+"' >> /etc/pacman.conf", true)
}

func (m *pacmanManager) NeedsSudo() bool { return true }

// =============================================================================
// systemd Service Manager
// =============================================================================

type systemdManager struct{}

func (m *systemdManager) Exists(name string) bool {
	result := runShellCommand("systemctl cat "+name, false)
	return result.OK
}

func (m *systemdManager) Status(name string) string {
	result := runShellCommand("systemctl is-active "+name, false)
	return strings.TrimSpace(result.Stdout)
}

func (m *systemdManager) Start(name string) Result {
	return runShellCommand("systemctl start "+name, true)
}

func (m *systemdManager) Stop(name string) Result {
	return runShellCommand("systemctl stop "+name, true)
}

func (m *systemdManager) Enable(name string) Result {
	return runShellCommand("systemctl enable "+name, true)
}

func (m *systemdManager) Disable(name string) Result {
	return runShellCommand("systemctl disable "+name, true)
}

func (m *systemdManager) NeedsSudo() bool { return true }
