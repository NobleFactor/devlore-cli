// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// =============================================================================
// Phase Script Arguments
// =============================================================================
//
// Phase scripts receive three arguments: (package, system, plan)
//
//   def forward(package, system, plan):
//       if system.package.installed(package.name):
//           return
//       plan.package.install(package.name)
//
// - package: Context about the package being deployed
// - system:  Read-only queries about the current platform state
// - plan:    Graph-building operations that add nodes to the execution graph

// =============================================================================
// System Bindings (read-only queries)
// =============================================================================

// SystemBindings provides read-only queries about the current platform state.
// Wraps host.Host to expose platform information to phase scripts.
type SystemBindings interface {
	// Platform returns information about the current system.
	Platform() host.Platform

	// Package provides package manager queries.
	Package() PackageQueries

	// Service provides service manager queries.
	Service() ServiceQueries

	// ToStarlark converts the system bindings to a Starlark value.
	ToStarlark() starlark.Value
}

// PackageQueries provides read-only package manager queries.
type PackageQueries interface {
	// Installed checks if a package is installed.
	Installed(name string) bool

	// Version returns the installed version of a package, or empty string if not installed.
	Version(name string) string
}

// ServiceQueries provides read-only service manager queries.
type ServiceQueries interface {
	// Exists checks if a service exists.
	Exists(name string) bool

	// Running checks if a service is currently running.
	Running(name string) bool

	// Enabled checks if a service is enabled at boot.
	Enabled(name string) bool
}

// =============================================================================
// Package Context
// =============================================================================

// PackageContext provides information about the package being deployed.
// Passed to phase scripts as the second argument.
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
