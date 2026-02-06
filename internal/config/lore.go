// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package config

// LoreConfig contains lore-specific configuration.
// Note: verbosity and dry_run are shared options at the Config root level.
type LoreConfig struct {
	Preferences Preferences `yaml:"preferences,omitempty" json:"preferences,omitempty"`
	Sources     Sources     `yaml:"sources,omitempty" json:"sources,omitempty"`
}

// Preferences controls AI-assisted behavior.
type Preferences struct {
	ConsultUserBeforeChanges bool   `yaml:"consult_user_before_changes,omitempty" json:"consult_user_before_changes,omitempty"`
	ValidateExistingPackages bool   `yaml:"validate_existing_packages,omitempty" json:"validate_existing_packages,omitempty"`
	SearchInternet           string `yaml:"search_internet,omitempty" json:"search_internet,omitempty"` // always, on-demand, never
}

// Sources configures documentation sources for AI-assisted features.
type Sources struct {
	Corporate      []CorporateSource `yaml:"corporate,omitempty" json:"corporate,omitempty"`
	TrustedDomains []string          `yaml:"trusted_domains,omitempty" json:"trusted_domains,omitempty"`
	BlockedDomains []string          `yaml:"blocked_domains,omitempty" json:"blocked_domains,omitempty"`
}

// CorporateSource represents an organization-specific documentation source.
type CorporateSource struct {
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
}
