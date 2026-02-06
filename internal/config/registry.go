// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package config

// Default registry settings.
const (
	DefaultRegistryURL    = "https://github.com/NobleFactor/devlore-registry.git"
	DefaultRegistryBranch = "develop" // develop branch has AI assets; main is release-only
)

// RegistryConfig configures the devlore package registry.
// This is shared across lore and writ.
type RegistryConfig struct {
	// URL is the registry repository URL.
	// Default: https://github.com/NobleFactor/devlore-registry.git
	URL string `yaml:"url,omitempty" json:"url,omitempty"`

	// Branch is the git branch to use.
	// Default: develop (for demo phase; main for releases)
	Branch string `yaml:"branch,omitempty" json:"branch,omitempty"`

	// ForceTags forces tag resolution even on non-main branches.
	// When true, "latest" always resolves to the "latest" tag.
	// Default: false
	ForceTags bool `yaml:"force_tags,omitempty" json:"force_tags,omitempty"`
}

// WithDefaults returns a copy of the config with defaults applied.
func (c RegistryConfig) WithDefaults() RegistryConfig {
	cfg := c
	if cfg.URL == "" {
		cfg.URL = DefaultRegistryURL
	}
	if cfg.Branch == "" {
		cfg.Branch = DefaultRegistryBranch
	}
	return cfg
}
