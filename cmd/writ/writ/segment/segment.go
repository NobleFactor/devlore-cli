// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package segment provides segment-based directory matching for writ.
package segment

import (
	"strings"
)

// Segment represents a single segment value (e.g., OS="Darwin", ROLE="desktop").
type Segment struct {
	Name  string // e.g., "OS", "DISTRO", "ROLE"
	Value string // e.g., "Darwin", "debian", "desktop"
}

// Segments is an ordered list of segments for matching.
// Order matters: OS, DISTRO, ARCH, then custom segments.
type Segments []Segment

// OSFamily returns the OS family for a given OS.
// Unix matches both Darwin and Linux.
func OSFamily(os string) string {
	switch os {
	case "Darwin", "Linux":
		return "Unix"
	default:
		return ""
	}
}

// Get returns the value for a segment by name, or empty if not found.
func (s Segments) Get(name string) string {
	for _, seg := range s {
		if seg.Name == name {
			return seg.Value
		}
	}
	return ""
}

// Values returns all non-empty segment values in order.
func (s Segments) Values() []string {
	var values []string
	for _, seg := range s {
		if seg.Value != "" {
			values = append(values, seg.Value)
		}
	}
	return values
}

// AllValues returns all matchable values including OS family.
// For OS=Darwin, returns ["Darwin", "Unix"].
func (s Segments) AllValues() []string {
	var values []string
	for _, seg := range s {
		if seg.Value == "" {
			continue
		}
		values = append(values, seg.Value)
		// Add OS family if this is the OS segment
		if seg.Name == "OS" {
			if family := OSFamily(seg.Value); family != "" {
				values = append(values, family)
			}
		}
	}
	return values
}

// ParseDirName parses a directory name into project and suffixes.
// Example: "noblefactor.Darwin.arm64" → "noblefactor", ["Darwin", "arm64"]
func ParseDirName(dirname string) (project string, suffixes []string) {
	parts := strings.Split(dirname, ".")
	if len(parts) == 0 {
		return dirname, nil
	}
	return parts[0], parts[1:]
}

// Match checks if a directory name matches the given segments.
// Returns true if all suffixes match segment values (including OS family).
func (s Segments) Match(dirname string) bool {
	_, suffixes := ParseDirName(dirname)
	if len(suffixes) == 0 {
		// No suffixes means it always matches (base project)
		return true
	}

	matchable := s.AllValues()
	for _, suffix := range suffixes {
		if !contains(matchable, suffix) {
			return false
		}
	}
	return true
}

// contains checks if a string slice contains a value.
func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// MatchResult represents a matched directory.
type MatchResult struct {
	Path     string   // Full path to directory
	Project  string   // Project name (e.g., "noblefactor")
	Suffixes []string // Matched suffixes (e.g., ["Darwin", "arm64"])
}

// Specificity returns the number of matched suffixes.
// Higher specificity means more specific match.
func (m MatchResult) Specificity() int {
	return len(m.Suffixes)
}

// Set returns a new Segments with the named segment set to value.
// If the segment doesn't exist, it is appended.
func (s Segments) Set(name, value string) Segments {
	result := make(Segments, len(s))
	copy(result, s)

	for i := range result {
		if result[i].Name == name {
			result[i].Value = value
			return result
		}
	}
	// Not found, append
	return append(result, Segment{Name: name, Value: value})
}

// String returns a human-readable representation of segments.
func (s Segments) String() string {
	var parts []string
	for _, seg := range s {
		if seg.Value != "" {
			parts = append(parts, seg.Name+"="+seg.Value)
		}
	}
	return strings.Join(parts, ", ")
}
