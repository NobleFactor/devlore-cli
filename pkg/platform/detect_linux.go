// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build linux

package platform

import (
	"fmt"
	"os"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/selector"
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
// It inspects os-release (through [selector.ReadOSRelease]: /etc/os-release, then /usr/lib/os-release), the host's
// runtime.GOARCH, hostname, and the workstation/server variant signal. A distribution this package doesn't list takes
// its managers from the closest ancestor its os-release ID_LIKE names that the package does list (#944): Pop!_OS takes
// Ubuntu's. The workstation/server refinement strips desktop-only managers (flatpak) from the manager set when the
// host reports a server-flavored variant. The signal hierarchy is: os-release VARIANT_ID when present, falling back to
// `systemctl get-default` (graphical.target keeps workstation defaults; multi-user.target strips desktop-only
// managers).
//
// Returns:
//   - `*Spec`: the detected host spec.
//   - `error`: when os-release is missing or names no ID, or when neither the ID nor any ID_LIKE ancestor is a listed
//     distro.
func detectHost() (*Spec, error) {

	release, ok := selector.ReadOSRelease()
	if !ok {
		return nil, fmt.Errorf("platform: detect linux: no os-release at /etc/os-release or /usr/lib/os-release")
	}
	if release.ID == "" {
		return nil, fmt.Errorf("platform: detect linux: os-release names no ID")
	}

	id, ok := resolveLinuxDistro(release)
	if !ok {
		return nil, fmt.Errorf("platform: detect linux: unknown distro %q (from os-release ID, and none of ID_LIKE %v); expected one of debian, ubuntu, mint, rhel, fedora, centos-stream, almalinux, rocky, arch, manjaro", release.ID, release.IDLike)
	}

	spec := linuxSpecByDistro[id]().
		WithArch("").
		WithVersion(release.VersionID)

	if hostname, herr := os.Hostname(); herr == nil {
		spec.WithHostname(hostname)
	}

	if isServerVariant(release.VariantID) {
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

// resolveLinuxDistro finds the listed distro whose managers a host takes: its os-release ID when this package lists
// it, otherwise the closest ancestor its ID_LIKE names that this package lists.
//
// Parameters:
//   - `release`: the host's os-release.
//
// Returns:
//   - `string`: the distro, in this package's vocabulary (aliases applied: linuxmint is mint, centos is centos-stream).
//   - `bool`: false when neither the ID nor any ID_LIKE ancestor is listed.
func resolveLinuxDistro(release selector.OSRelease) (string, bool) {

	for _, id := range append([]string{release.ID}, release.IDLike...) {
		if alias, ok := linuxDistroAliases[id]; ok {
			id = alias
		}
		if _, ok := linuxSpecByDistro[id]; ok {
			return id, true
		}
	}
	return "", false
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
	result := runCommand([]string{"systemctl", "get-default"}, false)
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
