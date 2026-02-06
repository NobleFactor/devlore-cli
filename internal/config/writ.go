// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package config

// WritConfig contains writ-specific configuration.
// Note: verbosity and dry_run are shared options at the Config root level.
type WritConfig struct {
	// Segments are custom segment names beyond built-in OS, DISTRO, ARCH.
	// Example: ["ROLE", "SITE"]
	Segments []string `yaml:"segments,omitempty" json:"segments,omitempty"`

	// Vars are template variables and segment value overrides.
	// Example: {"USER_NAME": "John Doe", "ROLE": "desktop"}
	Vars map[string]string `yaml:"vars,omitempty" json:"vars,omitempty"`
}
