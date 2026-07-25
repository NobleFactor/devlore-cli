// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package lore provides the runtime types and execution engine for the lore CLI.
package lore

// =============================================================================
// Phase Script Arguments
// =============================================================================
//
// Phase scripts receive two arguments: (package, phase)
// plan is a global, not an argument.
//
//   def install(package, phase):
//       phase.retry(max_attempts=3, backoff="exponential")
//       plan.package.install(package.name)
//
// - package: RuntimeEnvironment about the package being deployed
// - phase:   Phase context (name, action, retry)
// - plan:    Global — graph-building operations that add nodes to the execution graph

// =============================================================================
// Package RuntimeEnvironment
// =============================================================================

// PackageContext provides information about the package being deployed.
// Passed to phase scripts as the first argument.
type PackageContext struct {
	// Name is the package name being deployed.
	Name string

	// Version is the version being deployed.
	Version string

	// Features are the enabled feature flags for this deployment.
	Features []string

	// Settings are key-value configuration settings.
	Settings map[string]string

	// DryRun indicates this is a preview (no actual changes).
	DryRun bool

	// SourceRoot is the package source directory in the registry cache.
	SourceRoot string

	// TargetRoot is the deployment target directory (usually $HOME).
	TargetRoot string
}

// HasFeature checks if a feature is enabled.
func (p *PackageContext) HasFeature(name string) bool {
	for _, f := range p.Features {
		if f == name {
			return true
		}
	}
	return false
}

// Setting returns a setting value, or empty string if not set.
func (p *PackageContext) Setting(key string) string {
	if p.Settings == nil {
		return ""
	}
	return p.Settings[key]
}
