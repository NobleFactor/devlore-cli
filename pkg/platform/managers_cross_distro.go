// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"strings"
)

// =============================================================================
// snap Package Manager (Canonical's universal Linux store)
// =============================================================================
//
// Snap is the default secondary on Ubuntu (pre-installed since 16.04+) and Manjaro (via pamac). Available
// on most other distros via snapd installation but not pre-shipped.

type snapManager struct{}

func (m *snapManager) Name() string { return "snap" }

func (m *snapManager) ParsePURL(id string) PURL {

	name, version, _ := strings.Cut(id, "@")
	return PURL{Type: "snap", Name: name, Version: version}
}

func (m *snapManager) Installed(name string) bool {
	return runShellCommand("snap list "+name, false).OK
}

func (m *snapManager) Available(name string) bool {
	return runShellCommand("snap info "+name, false).OK
}

func (m *snapManager) Search(query string, limit int) []SearchResult {
	result := runShellCommand("snap find "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	lines := strings.Split(result.Stdout, "\n")
	for i, line := range lines {
		// First line is "Name Version Publisher Notes Summary" header; skip.
		if i == 0 || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			sr := SearchResult{Name: fields[0]}
			if len(fields) >= 2 {
				sr.Version = fields[1]
			}
			results = append(results, sr)
			if limit > 0 && len(results) >= limit {
				return results
			}
		}
	}
	return results
}

func (m *snapManager) Version(name string) string {
	result := runShellCommand("snap list "+name, false)
	if !result.OK {
		return ""
	}
	lines := strings.Split(result.Stdout, "\n")
	for i, line := range lines {
		if i == 0 || line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == name {
			return fields[1]
		}
	}
	return ""
}

func (m *snapManager) Install(packages ...string) PlatformResult {
	return runShellCommand("snap install "+strings.Join(packages, " "), true)
}

func (m *snapManager) Remove(name string) PlatformResult {
	return runShellCommand("snap remove "+name, true)
}

func (m *snapManager) Update() PlatformResult {
	return runShellCommand("snap refresh", true)
}

// AddRepo is a no-op for snap. The snap store is the canonical and only source; user-managed repositories
// are not part of the snap model.
func (m *snapManager) AddRepo(_, _, _ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "snap does not support user-managed repositories; the snap store is the only source"}
}

func (m *snapManager) NeedsSudo() bool { return true }

// =============================================================================
// flatpak Package Manager (Sandboxed desktop apps via Flathub or other remotes)
// =============================================================================
//
// Flatpak is the default secondary on Fedora Workstation, openSUSE, and Mint. Available cross-distro;
// remotes (Flathub being the canonical one) supply the app catalog.

type flatpakManager struct{}

func (m *flatpakManager) Name() string { return "flatpak" }

func (m *flatpakManager) ParsePURL(id string) PURL {

	name, version, _ := strings.Cut(id, "@")
	return PURL{Type: "flatpak", Name: name, Version: version}
}

func (m *flatpakManager) Installed(name string) bool {
	return runShellCommand("flatpak info "+name, false).OK
}

func (m *flatpakManager) Available(name string) bool {
	return runShellCommand("flatpak remote-info flathub "+name, false).OK
}

func (m *flatpakManager) Search(query string, limit int) []SearchResult {
	result := runShellCommand("flatpak search "+query, false)
	if !result.OK {
		return nil
	}

	var results []SearchResult
	lines := strings.Split(result.Stdout, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		// flatpak search output is tab-separated: Name Description ApplicationID Version Branch Remotes.
		fields := strings.Split(line, "\t")
		if len(fields) >= 1 && fields[0] != "" {
			sr := SearchResult{Name: strings.TrimSpace(fields[0])}
			if len(fields) >= 2 {
				sr.Description = strings.TrimSpace(fields[1])
			}
			if len(fields) >= 4 {
				sr.Version = strings.TrimSpace(fields[3])
			}
			results = append(results, sr)
			if limit > 0 && len(results) >= limit {
				return results
			}
		}
	}
	return results
}

func (m *flatpakManager) Version(name string) string {
	result := runShellCommand("flatpak info --show-version "+name, false)
	if !result.OK {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

func (m *flatpakManager) Install(packages ...string) PlatformResult {
	return runShellCommand("flatpak install -y flathub "+strings.Join(packages, " "), false)
}

func (m *flatpakManager) Remove(name string) PlatformResult {
	return runShellCommand("flatpak uninstall -y "+name, false)
}

func (m *flatpakManager) Update() PlatformResult {
	return runShellCommand("flatpak update -y", false)
}

// AddRepo registers a flatpak remote. name is the remote name (e.g., "flathub"); url is the remote URL
// (e.g., "https://flathub.org/repo/flathub.flatpakrepo"). keyURL is unused — flatpak remotes carry their
// own GPG signatures embedded in the .flatpakrepo file.
func (m *flatpakManager) AddRepo(url, _, name string) PlatformResult {
	return runShellCommand("flatpak remote-add --if-not-exists "+name+" "+url, false)
}

// NeedsSudo is false because flatpak defaults to user-level installs (~/.local/share/flatpak). System-wide
// installs (--system flag) would need sudo, but those are not the default path.
func (m *flatpakManager) NeedsSudo() bool { return false }
