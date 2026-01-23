// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package segment

import (
	"bufio"
	"os"
	"runtime"
	"strings"
)

// DetectSegments returns segments for the current platform.
// Returns OS, DISTRO (Linux only), and ARCH segments.
func DetectSegments() Segments {
	return Segments{
		{Name: "OS", Value: capitalizeOS(runtime.GOOS)},
		{Name: "DISTRO", Value: detectDistro()},
		{Name: "ARCH", Value: runtime.GOARCH},
	}
}

// DetectSegmentsWithNames returns segments with custom segment names from config.
// Custom segment names (like ROLE, SITE) are appended after OS, DISTRO, ARCH.
// Values are empty until set via CLI --segment flags.
// Unassigned segments behave like DISTRO on macOS: directories with that suffix won't match.
func DetectSegmentsWithNames(names []string) Segments {
	segs := DetectSegments()
	for _, name := range names {
		segs = append(segs, Segment{Name: name, Value: ""})
	}
	return segs
}

// EnvVarPrefix is the prefix for segment environment variables.
// Segments are set via WRIT_SEGMENT_<NAME>=<value> (e.g., WRIT_SEGMENT_ROLE=server).
const EnvVarPrefix = "WRIT_SEGMENT_"

// LoadFromEnv reads segment values from environment variables.
// Only loads values for segments already defined in segs.
// Environment variable format: WRIT_SEGMENT_<NAME>=<value>
// Returns a new Segments with values populated from environment.
func (s Segments) LoadFromEnv() Segments {
	result := make(Segments, len(s))
	copy(result, s)

	for i := range result {
		envVar := EnvVarPrefix + result[i].Name
		if value := os.Getenv(envVar); value != "" {
			result[i].Value = value
		}
	}

	return result
}

// SetValues applies CLI --segment values to existing segments.
// Returns an error if a value references a segment name not defined in segs.
// Values format: map[name]value (e.g., {"ROLE": "server", "SITE": "aws"})
func (s Segments) SetValues(values map[string]string) (Segments, error) {
	result := make(Segments, len(s))
	copy(result, s)

	for name, value := range values {
		found := false
		for i := range result {
			if result[i].Name == name {
				result[i].Value = value
				found = true
				break
			}
		}
		if !found {
			return nil, &UndefinedSegmentError{Name: name}
		}
	}

	return result, nil
}

// UndefinedSegmentError indicates a CLI --segment referenced an undefined segment name.
type UndefinedSegmentError struct {
	Name string
}

func (e *UndefinedSegmentError) Error() string {
	return "undefined segment: " + e.Name + " (must be defined in config)"
}

// capitalizeOS converts runtime.GOOS to capitalized form.
// darwin → Darwin, linux → Linux, windows → Windows
func capitalizeOS(goos string) string {
	switch goos {
	case "darwin":
		return "Darwin"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	case "freebsd":
		return "FreeBSD"
	case "openbsd":
		return "OpenBSD"
	case "netbsd":
		return "NetBSD"
	default:
		// Capitalize first letter for unknown OS
		if len(goos) == 0 {
			return goos
		}
		return strings.ToUpper(goos[:1]) + goos[1:]
	}
}

// detectDistro returns the Linux distribution ID from /etc/os-release.
// Returns empty string on non-Linux or if detection fails.
func detectDistro() string {
	if runtime.GOOS != "linux" {
		return ""
	}

	file, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			id := strings.TrimPrefix(line, "ID=")
			id = strings.Trim(id, "\"")
			return capitalizeDistro(id)
		}
	}

	return ""
}

// capitalizeDistro converts distro ID to capitalized form.
func capitalizeDistro(id string) string {
	switch id {
	case "debian":
		return "Debian"
	case "ubuntu":
		return "Ubuntu"
	case "fedora":
		return "Fedora"
	case "centos":
		return "CentOS"
	case "rhel":
		return "RHEL"
	case "arch":
		return "Arch"
	case "alpine":
		return "Alpine"
	case "opensuse", "opensuse-leap", "opensuse-tumbleweed":
		return "OpenSUSE"
	default:
		// Capitalize first letter for unknown distro
		if len(id) == 0 {
			return id
		}
		return strings.ToUpper(id[:1]) + id[1:]
	}
}
