// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build linux

package platform

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
)

// linuxDistroAliases maps freedesktop.org `os-release` ID values that don't match our internal distro vocabulary.
//
// Anything not in this map is taken at face value and looked up in [linuxSpecByDistro].
var linuxDistroAliases = map[string]string{
	"linuxmint": "mint",
	"centos":    "centos-stream", // CentOS Stream uses ID=centos; older CentOS is EOL.
}

// detectHost returns a fresh host [*Spec] cloned from [linuxSpecByDistro] for the detected distro.
//
// It inspects /etc/os-release, the host's runtime.GOARCH, hostname, and the workstation/server variant signal. The
// workstation/server refinement strips desktop-only managers (flatpak) from the manager set when the host reports a
// server-flavored variant. The signal hierarchy is: /etc/os-release VARIANT_ID when present, falling back to
// `systemctl get-default` (graphical.target keeps workstation defaults; multi-user.target strips desktop-only
// managers).
//
// Returns:
//   - `*Spec`: the detected host spec.
//   - `error`: when the distro is not in the known set, or when /etc/os-release is missing or malformed.
func detectHost() (*Spec, error) {

	id, versionID, variantID, err := readOSRelease()
	if err != nil {
		return nil, fmt.Errorf("platform: detect linux: %w", err)
	}

	if alias, ok := linuxDistroAliases[id]; ok {
		id = alias
	}

	factory, ok := linuxSpecByDistro[id]
	if !ok {
		return nil, fmt.Errorf("platform: detect linux: unknown distro %q (from /etc/os-release ID); expected one of debian, ubuntu, mint, rhel, fedora, centos-stream, almalinux, rocky, arch, manjaro", id)
	}

	spec := factory().
		WithArch("").
		WithVersion(versionID)

	if hostname, herr := os.Hostname(); herr == nil {
		spec.WithHostname(hostname)
	}

	if isServerVariant(variantID) {
		spec.managers = stripDesktopOnly(spec.managers)
	}

	spec.serviceManager = detectInit()

	return spec, nil
}

// detectInit probes the active init system and returns the matching service manager.
//
// systemd publishes /run/systemd/system when it is PID 1; its absence — containers, WSL, minimal/CI boxes — selects
// the SysVinit `service` path. This runs on the live host, so it reflects the actual init, not the distro's declared
// default (which the named factories set to systemd).
//
// Returns:
//   - `ServiceManager`: &systemdManager{} when systemd is the active init, else &sysVinitManager{}.
func detectInit() ServiceManager {

	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return &systemdManager{}
	}
	return &sysVinitManager{}
}

// readOSRelease reads /etc/os-release and returns its ID, VERSION_ID, and VARIANT_ID fields.
//
// Returns:
//   - `id`: the distro ID; empty when absent (an error).
//   - `versionID`: the VERSION_ID; empty when absent.
//   - `variantID`: the VARIANT_ID; empty when absent.
//   - `err`: non-nil when the file cannot be read or lacks an ID field.
func readOSRelease() (id, versionID, variantID string, err error) {

	file, err := os.Open("/etc/os-release")
	if err != nil {
		return "", "", "", fmt.Errorf("read /etc/os-release: %w", err)
	}

	defer iox.Close(&err, file)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "ID="):
			id = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		case strings.HasPrefix(line, "VERSION_ID="):
			versionID = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		case strings.HasPrefix(line, "VARIANT_ID="):
			variantID = strings.Trim(strings.TrimPrefix(line, "VARIANT_ID="), "\"")
		}
	}

	if id == "" {
		return "", "", "", fmt.Errorf("/etc/os-release missing ID field")
	}

	return id, versionID, variantID, nil
}

// isServerVariant reports whether the host should be treated as a server-flavored install (no GUI).
//
// A server-flavored install strips desktop-only managers from the manager set. Falls back to `systemctl
// get-default` when VARIANT_ID is empty or non-definitive.
//
// Parameters:
//   - `variantID`: the /etc/os-release VARIANT_ID value (may be empty).
//
// Returns:
//   - `bool`: true when the host is server-flavored.
func isServerVariant(variantID string) bool {

	switch variantID {
	case "workstation", "silverblue", "kinoite", "iot", "cloud":
		return false
	case "server", "coreos":
		return true
	}

	// VARIANT_ID absent or unrecognized — fall back to systemd's default-target signal.
	result := runShellCommand("systemctl get-default", false)
	if !result.OK {
		return false
	}
	return strings.TrimSpace(result.Stdout) == "multi-user.target"
}

// stripDesktopOnly returns a copy of `managers` with desktop-only managers (flatpak) removed.
//
// snap is left in because it is genuinely cross-context (Ubuntu Server pre-installs snapd just like Ubuntu
// Desktop). The default native manager (apt/dnf/pacman) always survives the strip.
//
// Parameters:
//   - `managers`: the platform's leaf set.
//
// Returns:
//   - `[]leaf`: the managers with flatpak removed.
func stripDesktopOnly(managers []leaf) []leaf {

	stripped := make([]leaf, 0, len(managers))
	for _, manager := range managers {
		if manager.name() == "flatpak" {
			continue
		}
		stripped = append(stripped, manager)
	}
	return stripped
}
