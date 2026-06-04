// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build linux

package op

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// newLinux builds the [*Platform] describing the host Linux system.
//
// Detects the distribution and version, architecture, package manager, and service manager, and seeds the
// package-manager registry from the detected manager.
//
// Returns:
//   - `*Platform`: the platform describing the host Linux system.
func newLinux() *Platform {
	hostname, _ := os.Hostname() //nolint:errcheck // best effort
	distro, version := detectLinuxDistro()

	packageManager := detectLinuxPackageManager(distro)
	packageManagers := make(map[string]PackageManager)
	if packageManager != nil {
		packageManagers[packageManager.Name()] = packageManager
	}

	return &Platform{
		OS:                 "linux",
		Arch:               detectArch(),
		Distro:             distro,
		Version:            version,
		Hostname:           hostname,
		DefaultConcurrency: 4 * runtime.NumCPU(),
		PackageManager:     packageManager,
		PackageManagers:    packageManagers,
		ServiceManager:     &systemdManager{},
	}
}

// aptManager drives the APT package manager (Debian, Ubuntu).
type aptManager struct{}

// region EXPORTED METHODS

// region Behaviors

// AddRepo registers an APT repository, importing its signing key first when one is supplied.
//
// Parameters:
//   - `url`: the repository URL.
//   - `keyURL`: the signing-key URL, or empty to skip key import.
//   - `name`: the repository name, used for the keyring and sources-list filenames.
//
// Returns:
//   - `PlatformResult`: the result of the repository-registration command.
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

// Available reports whether the named package exists in the APT cache.
//
// Parameters:
//   - `name`: the package name.
//
// Returns:
//   - `bool`: true when the package is available.
func (m *aptManager) Available(name string) bool {
	return runShellCommand("apt-cache show "+name, false).OK
}

// Install installs the named packages via apt-get.
//
// Parameters:
//   - `packages`: the package names to install.
//
// Returns:
//   - `PlatformResult`: the result of the install command.
func (m *aptManager) Install(packages ...string) PlatformResult {
	return runShellCommand("apt-get install -y "+strings.Join(packages, " "), true)
}

// Installed reports whether the named package is installed.
//
// Parameters:
//   - `name`: the package name.
//
// Returns:
//   - `bool`: true when the package is installed.
func (m *aptManager) Installed(name string) bool {
	return runShellCommand("dpkg-query -W "+name, false).OK
}

// Name returns the package manager's identifier.
//
// Returns:
//   - `string`: always "apt".
func (m *aptManager) Name() string { return "apt" }

// NeedsSudo reports whether the manager's mutating operations require elevated privileges.
//
// Returns:
//   - `bool`: always true.
func (m *aptManager) NeedsSudo() bool { return true }

// ParsePURL parses a package identifier into a [PURL] carrying the deb type.
//
// Parameters:
//   - `id`: the package identifier, optionally in "name@version" form.
//
// Returns:
//   - `PURL`: the parsed package URL.
func (m *aptManager) ParsePURL(id string) PURL {

	name, version, _ := strings.Cut(id, "@")
	return PURL{Type: "deb", Name: name, Version: version}
}

// Remove uninstalls the named package via apt-get.
//
// Parameters:
//   - `name`: the package name.
//
// Returns:
//   - `PlatformResult`: the result of the remove command.
func (m *aptManager) Remove(name string) PlatformResult {
	return runShellCommand("apt-get remove -y "+name, true)
}

// Search queries the APT cache and returns up to `limit` matches.
//
// Parameters:
//   - `query`: the search query.
//   - `limit`: the maximum number of results, or 0 for no limit.
//
// Returns:
//   - `[]SearchResult`: the matching packages, or nil on failure.
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

// Update refreshes the APT package index.
//
// Returns:
//   - `PlatformResult`: the result of the update command.
func (m *aptManager) Update() PlatformResult {
	return runShellCommand("apt-get update", true)
}

// Version returns the installed version of the named package, or empty when it is not installed.
//
// Parameters:
//   - `name`: the package name.
//
// Returns:
//   - `string`: the installed version, or the empty string.
func (m *aptManager) Version(name string) string {
	result := runShellCommand("dpkg-query -W -f='${Version}' "+name, false)
	if !result.OK {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

// endregion

// endregion

// dnfManager drives the DNF package manager (Fedora, RHEL).
type dnfManager struct{}

// region EXPORTED METHODS

// region Behaviors

// AddRepo registers a DNF repository, importing its signing key first when one is supplied.
//
// Parameters:
//   - `url`: the repository URL.
//   - `keyURL`: the signing-key URL, or empty to skip key import.
//
// Returns:
//   - `PlatformResult`: the result of the repository-registration command.
func (m *dnfManager) AddRepo(url, keyURL, _ string) PlatformResult {
	if keyURL != "" {
		keyResult := runShellCommand("rpm --import "+keyURL, true)
		if !keyResult.OK {
			return keyResult
		}
	}
	return runShellCommand("dnf config-manager --add-repo "+url, true)
}

// Available reports whether the named package is known to DNF.
//
// Parameters:
//   - `name`: the package name.
//
// Returns:
//   - `bool`: true when the package is available.
func (m *dnfManager) Available(name string) bool {
	return runShellCommand("dnf info "+name, false).OK
}

// Install installs the named packages via dnf.
//
// Parameters:
//   - `packages`: the package names to install.
//
// Returns:
//   - `PlatformResult`: the result of the install command.
func (m *dnfManager) Install(packages ...string) PlatformResult {
	return runShellCommand("dnf install -y "+strings.Join(packages, " "), true)
}

// Installed reports whether the named package is installed.
//
// Parameters:
//   - `name`: the package name.
//
// Returns:
//   - `bool`: true when the package is installed.
func (m *dnfManager) Installed(name string) bool {
	return runShellCommand("rpm -q "+name, false).OK
}

// Name returns the package manager's identifier.
//
// Returns:
//   - `string`: always "dnf".
func (m *dnfManager) Name() string { return "dnf" }

// NeedsSudo reports whether the manager's mutating operations require elevated privileges.
//
// Returns:
//   - `bool`: always true.
func (m *dnfManager) NeedsSudo() bool { return true }

// ParsePURL parses a package identifier into a [PURL] carrying the rpm type.
//
// Parameters:
//   - `id`: the package identifier, optionally in "name@version" form.
//
// Returns:
//   - `PURL`: the parsed package URL.
func (m *dnfManager) ParsePURL(id string) PURL {

	name, version, _ := strings.Cut(id, "@")
	return PURL{Type: "rpm", Name: name, Version: version}
}

// Remove uninstalls the named package via dnf.
//
// Parameters:
//   - `name`: the package name.
//
// Returns:
//   - `PlatformResult`: the result of the remove command.
func (m *dnfManager) Remove(name string) PlatformResult {
	return runShellCommand("dnf remove -y "+name, true)
}

// Search queries `dnf search` and returns up to `limit` matches.
//
// Parameters:
//   - `query`: the search query.
//   - `limit`: the maximum number of results, or 0 for no limit.
//
// Returns:
//   - `[]SearchResult`: the matching packages, or nil on failure.
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

// Update refreshes the DNF metadata cache.
//
// Returns:
//   - `PlatformResult`: the result of the update command.
func (m *dnfManager) Update() PlatformResult {
	return runShellCommand("dnf check-update || true", true)
}

// Version returns the installed version of the named package, or empty when it is not installed.
//
// Parameters:
//   - `name`: the package name.
//
// Returns:
//   - `string`: the installed version, or the empty string.
func (m *dnfManager) Version(name string) string {
	result := runShellCommand("rpm -q --queryformat '%{VERSION}' "+name, false)
	if !result.OK {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

// endregion

// endregion

// pacmanManager drives the Pacman package manager (Arch, Manjaro, EndeavourOS).
type pacmanManager struct{}

// region EXPORTED METHODS

// region Behaviors

// AddRepo registers a Pacman repository, receiving and locally signing its key first when one is supplied.
//
// Parameters:
//   - `url`: the repository server URL.
//   - `keyURL`: the key ID to receive and locally sign, or empty to skip.
//   - `name`: the repository name.
//
// Returns:
//   - `PlatformResult`: the result of the repository-registration command.
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

// Available reports whether the named package is known to Pacman.
//
// Parameters:
//   - `name`: the package name.
//
// Returns:
//   - `bool`: true when the package is available.
func (m *pacmanManager) Available(name string) bool {
	return runShellCommand("pacman -Si "+name, false).OK
}

// Install installs the named packages via pacman.
//
// Parameters:
//   - `packages`: the package names to install.
//
// Returns:
//   - `PlatformResult`: the result of the install command.
func (m *pacmanManager) Install(packages ...string) PlatformResult {
	return runShellCommand("pacman -S --noconfirm --needed "+strings.Join(packages, " "), true)
}

// Installed reports whether the named package is installed.
//
// Parameters:
//   - `name`: the package name.
//
// Returns:
//   - `bool`: true when the package is installed.
func (m *pacmanManager) Installed(name string) bool {
	return runShellCommand("pacman -Q "+name, false).OK
}

// Name returns the package manager's identifier.
//
// Returns:
//   - `string`: always "pacman".
func (m *pacmanManager) Name() string { return "pacman" }

// NeedsSudo reports whether the manager's mutating operations require elevated privileges.
//
// Returns:
//   - `bool`: always true.
func (m *pacmanManager) NeedsSudo() bool { return true }

// ParsePURL parses a package identifier into a [PURL] carrying the alpm type.
//
// Parameters:
//   - `id`: the package identifier, optionally in "name@version" form.
//
// Returns:
//   - `PURL`: the parsed package URL.
func (m *pacmanManager) ParsePURL(id string) PURL {

	name, version, _ := strings.Cut(id, "@")
	return PURL{Type: "alpm", Name: name, Version: version}
}

// Remove uninstalls the named package via pacman.
//
// Parameters:
//   - `name`: the package name.
//
// Returns:
//   - `PlatformResult`: the result of the remove command.
func (m *pacmanManager) Remove(name string) PlatformResult {
	return runShellCommand("pacman -R --noconfirm "+name, true)
}

// Search queries pacman and returns up to `limit` matches.
//
// Parameters:
//   - `query`: the search query.
//   - `limit`: the maximum number of results, or 0 for no limit.
//
// Returns:
//   - `[]SearchResult`: the matching packages, or nil on failure.
func (m *pacmanManager) Search(query string, limit int) []SearchResult { //nolint:gocognit // parsing format requires nesting

	result := runShellCommand("pacman -Ss "+query, false)

	if !result.OK {
		return nil
	}

	lines := strings.Split(result.Stdout, "\n")
	var results []SearchResult

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

// Update refreshes the Pacman package databases.
//
// Returns:
//   - `PlatformResult`: the result of the update command.
func (m *pacmanManager) Update() PlatformResult {

	return runShellCommand("pacman -Sy", true)
}

// Version returns the installed version of the named package, or empty when it is not installed.
//
// Parameters:
//   - `name`: the package name.
//
// Returns:
//   - `string`: the installed version, or the empty string.
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

// endregion

// endregion

// systemdManager drives the systemd service manager.
type systemdManager struct{}

// region EXPORTED METHODS

// region Behaviors

// Disable prevents the named unit from starting at boot.
//
// Parameters:
//   - `name`: the unit name.
//
// Returns:
//   - `PlatformResult`: the result of the disable command.
func (m *systemdManager) Disable(name string) PlatformResult {

	return runShellCommand("systemctl disable "+name, true)
}

// Enable configures the named unit to start at boot.
//
// Parameters:
//   - `name`: the unit name.
//
// Returns:
//   - `PlatformResult`: the result of the enable command.
func (m *systemdManager) Enable(name string) PlatformResult {

	return runShellCommand("systemctl enable "+name, true)
}

// Exists reports whether systemd knows the named unit.
//
// Parameters:
//   - `name`: the unit name.
//
// Returns:
//   - `bool`: true when the unit exists.
func (m *systemdManager) Exists(name string) bool {

	return runShellCommand("systemctl cat "+name, false).OK
}

// IsEnabled reports whether the named unit is enabled to start at boot.
//
// Parameters:
//   - `name`: the unit name.
//
// Returns:
//   - `bool`: true when the unit is enabled.
func (m *systemdManager) IsEnabled(name string) bool {

	return runShellCommand("systemctl is-enabled --quiet "+name, false).OK
}

// IsRunning reports whether the named unit is currently active.
//
// Parameters:
//   - `name`: the unit name.
//
// Returns:
//   - `bool`: true when the unit is active.
func (m *systemdManager) IsRunning(name string) bool {

	return runShellCommand("systemctl is-active --quiet "+name, false).OK
}

// NeedsSudo reports whether the manager's mutating operations require elevated privileges.
//
// Returns:
//   - `bool`: always true.
func (m *systemdManager) NeedsSudo() bool { return true }

// Start activates the named unit.
//
// Parameters:
//   - `name`: the unit name.
//
// Returns:
//   - `PlatformResult`: the result of the start command.
func (m *systemdManager) Start(name string) PlatformResult {

	return runShellCommand("systemctl start "+name, true)
}

// Status returns the named unit's active state.
//
// Parameters:
//   - `name`: the unit name.
//
// Returns:
//   - `string`: the active state (e.g. "active", "inactive", "failed").
func (m *systemdManager) Status(name string) string {

	result := runShellCommand("systemctl is-active "+name, false)
	return strings.TrimSpace(result.Stdout)
}

// Stop deactivates the named unit.
//
// Parameters:
//   - `name`: the unit name.
//
// Returns:
//   - `PlatformResult`: the result of the stop command.
func (m *systemdManager) Stop(name string) PlatformResult {

	return runShellCommand("systemctl stop "+name, true)
}

// endregion

// endregion

// region HELPERS

// detectLinuxDistro reads /etc/os-release to determine the host distribution and version.
//
// Returns:
//   - `distro`: the distribution ID (e.g. "ubuntu"), or "linux" when it cannot be determined.
//   - `version`: the VERSION_ID, or the empty string when absent.
func detectLinuxDistro() (distro, version string) {

	file, err := os.Open("/etc/os-release")
	if err != nil {
		return "linux", ""
	}
	defer func() { _ = file.Close() }()

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

// detectLinuxPackageManager selects the [PackageManager] for a distribution.
//
// Maps known distribution IDs to their package manager, then falls back to probing PATH for pacman, apt, and dnf.
//
// Parameters:
//   - `distro`: the distribution ID, as returned by [detectLinuxDistro].
//
// Returns:
//   - `PackageManager`: the selected manager; defaults to apt when nothing else matches.
func detectLinuxPackageManager(distro string) PackageManager { //nolint:ireturn // returns concrete behind interface

	switch distro {
	case "debian", "ubuntu", "linuxmint", "pop", "elementary":
		return &aptManager{}
	case "fedora", "rhel", "centos", "rocky", "alma":
		return &dnfManager{}
	case "arch", "manjaro", "endeavouros", "garuda", "artix":
		return &pacmanManager{}
	}

	if _, err := exec.LookPath("pacman"); err == nil {
		return &pacmanManager{}
	}

	if _, err := exec.LookPath("apt"); err == nil {
		return &aptManager{}
	}

	if _, err := exec.LookPath("dnf"); err == nil {
		return &dnfManager{}
	}

	return &aptManager{}
}

// endregion
