// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package config

// WritConfig contains writ-specific configuration.
// Note: verbosity and dry_run are shared options at the Config root level.
type WritConfig struct {
	// Segments are the extra segments, beyond the built-in OS, DISTRO and ARCH, in the order a directory name carries
	// them (#944). Each declares the values it may take; a directory name carries one of them.
	Segments []SegmentDeclaration `yaml:"segments,omitempty" json:"segments,omitempty"`

	// Vars are template variables, and nothing else: segments live under Segments.
	// Example: {"USER_NAME": "John Doe"}
	Vars map[string]string `yaml:"vars,omitempty" json:"vars,omitempty"`
}

// SegmentDeclaration declares an extra segment: its name, the values it may take, and optionally this machine's value.
type SegmentDeclaration struct {
	// Name is the segment's name: ROLE. It may not be a built-in (OS, DISTRO, ARCH).
	Name string `yaml:"name" json:"name"`

	// Values are the values it may take: desktop, server. They are unique across every segment.
	Values []string `yaml:"values" json:"values"`

	// Value is this machine's value, one of Values. WRIT_SEGMENT_<NAME> and --segment NAME=value override it; unset,
	// the segment matches no directory name.
	Value string `yaml:"value,omitempty" json:"value,omitempty"`
}
