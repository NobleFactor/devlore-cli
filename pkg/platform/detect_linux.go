// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build linux

package platform

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// linuxDistroAliases maps freedesktop.org `os-release` ID values that don't match our internal distro
// vocabulary directly. Anything not in this map is taken at face value and looked up in
// [defaultLinuxPlatforms].
var linuxDistroAliases = map[string]string{
	"linuxmint": "mint",
	"centos":    "centos-stream", // CentOS Stream uses ID=centos; older CentOS is EOL.
}

// detectHost inspects /etc/os-release, the host's runtime.GOARCH, hostname, and the workstation/server
// variant signal, then constructs a [Platform] from [defaultLinuxPlatforms] for the detected distro.
//
// The workstation/server refinement strips desktop-only managers (flatpak) from the available set when
// the host reports a server-flavored variant. The signal hierarchy is:
//
//  1. /etc/os-release VARIANT_ID — definitive when present (Fedora's "workstation"/"server"/"silverblue").
//  2. `systemctl get-default` — graphical.target keeps workstation defaults; multi-user.target strips
//     desktop-only managers.
//
// Returns:
//   - Platform: the detected platform value.
//   - error: when the distro is not in the known set, or when /etc/os-release is missing/malformed.
func detectHost() (Platform, error) {

	id, versionID, variantID, err := readOSRelease()
	if err != nil {
		return nil, fmt.Errorf("platform: detect linux: %w", err)
	}

	if alias, ok := linuxDistroAliases[id]; ok {
		id = alias
	}

	factory, ok := defaultLinuxPlatforms[id]
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
		spec.WithAvailablePackageManagers(stripDesktopOnly(spec.availablePackageManagers))
		// If the default was flatpak (it isn't, in our table — but defensive), fall back to the
		// remaining default in the stripped map. Today, the default is always native (apt/dnf/pacman)
		// which survives the strip; this branch is a no-op.
	}

	return spec.Build()
}

// readOSRelease reads /etc/os-release and returns the ID, VERSION_ID, and VARIANT_ID fields. Empty
// strings are returned for any missing field.
func readOSRelease() (id, versionID, variantID string, err error) {

	file, err := os.Open("/etc/os-release")
	if err != nil {
		return "", "", "", fmt.Errorf("read /etc/os-release: %w", err)
	}
	defer func() { _ = file.Close() }()

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

// isServerVariant reports whether the host should be treated as a server-flavored install (no GUI),
// which strips desktop-only managers from the available-managers set.
//
// Falls back to `systemctl get-default` when VARIANT_ID is empty or non-definitive.
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

// stripDesktopOnly returns a copy of available with desktop-only managers (flatpak) removed.
//
// Used by [detectHost] when the host is a server-flavored install. snap is left in the set because it
// is genuinely cross-context (Ubuntu Server pre-installs snapd just like Ubuntu Desktop).
func stripDesktopOnly(available map[string]PackageManager) map[string]PackageManager {

	stripped := make(map[string]PackageManager, len(available))
	for name, manager := range available {
		if name == "flatpak" {
			continue
		}
		stripped[name] = manager
	}
	return stripped
}
