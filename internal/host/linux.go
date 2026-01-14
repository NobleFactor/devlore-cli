// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

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
	switch h.platform.Distro {
	case "debian", "ubuntu", "linuxmint", "pop", "elementary":
		return &aptManager{}
	case "fedora", "rhel", "centos", "rocky", "alma":
		return &dnfManager{}
	case "arch", "manjaro", "endeavouros":
		return &pacmanManager{}
	case "opensuse", "suse":
		return &zypperManager{}
	}

	// Fallback: detect by binary
	if _, err := exec.LookPath("apt"); err == nil {
		return &aptManager{}
	}
	if _, err := exec.LookPath("dnf"); err == nil {
		return &dnfManager{}
	}
	if _, err := exec.LookPath("pacman"); err == nil {
		return &pacmanManager{}
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

// =============================================================================
// APT Package Manager (Debian, Ubuntu)
// =============================================================================

type aptManager struct{}

func (m *aptManager) Name() string { return "apt" }

func (m *aptManager) Installed(name string) bool {
	result := runShellCommand("dpkg-query -W "+name, false)
	return result.OK
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
// Pacman Package Manager (Arch Linux)
// =============================================================================

type pacmanManager struct{}

func (m *pacmanManager) Name() string { return "pacman" }

func (m *pacmanManager) Installed(name string) bool {
	result := runShellCommand("pacman -Q "+name, false)
	return result.OK
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

func (m *pacmanManager) Install(packages ...string) Result {
	return runShellCommand("pacman -S --noconfirm "+strings.Join(packages, " "), true)
}

func (m *pacmanManager) Remove(name string) Result {
	return runShellCommand("pacman -R --noconfirm "+name, true)
}

func (m *pacmanManager) Update() Result {
	return runShellCommand("pacman -Sy", true)
}

func (m *pacmanManager) AddRepo(url, keyURL, name string) Result {
	// Pacman repos are typically added via /etc/pacman.conf
	return Result{OK: false, Stderr: "Use /etc/pacman.conf for custom repositories"}
}

func (m *pacmanManager) NeedsSudo() bool { return true }

// =============================================================================
// Zypper Package Manager (openSUSE)
// =============================================================================

type zypperManager struct{}

func (m *zypperManager) Name() string { return "zypper" }

func (m *zypperManager) Installed(name string) bool {
	result := runShellCommand("rpm -q "+name, false)
	return result.OK
}

func (m *zypperManager) Version(name string) string {
	result := runShellCommand("rpm -q --queryformat '%{VERSION}' "+name, false)
	if !result.OK {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

func (m *zypperManager) Install(packages ...string) Result {
	return runShellCommand("zypper --non-interactive install "+strings.Join(packages, " "), true)
}

func (m *zypperManager) Remove(name string) Result {
	return runShellCommand("zypper --non-interactive remove "+name, true)
}

func (m *zypperManager) Update() Result {
	return runShellCommand("zypper refresh", true)
}

func (m *zypperManager) AddRepo(url, keyURL, name string) Result {
	return runShellCommand("zypper addrepo "+url+" "+name, true)
}

func (m *zypperManager) NeedsSudo() bool { return true }

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
