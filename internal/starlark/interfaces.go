// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// =============================================================================
// Phase Script Arguments
// =============================================================================
//
// Phase scripts receive three arguments: (package, system, plan)
//
//   def install(package, system, plan):
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

// =============================================================================
// Plan Bindings (graph-building operations)
// =============================================================================

// PlanBindings provides operations that add nodes to the execution graph.
// Each method returns the created node for chaining or dependency specification.
//
// Note: Go method names are simple (Install, Remove, etc.) while Starlark
// uses nested structs (plan.package.install, plan.file.copy) and engine
// operations use namespaced names (package-install, package-remove).
type PlanBindings interface {
	// Graph returns the underlying execution graph being built.
	Graph() *execution.Graph

	// Host returns the host abstraction for path expansion, etc.
	Host() host.Host

	// Project returns the project name for grouping nodes.
	Project() string

	// Package operations - use platform's auto-detected package manager.
	// On Darwin, supports brew:pkg and port:pkg prefixes for manager override.

	// PackageInstall adds a package installation node.
	PackageInstall(packages ...string) *execution.Node

	// PackageUpgrade adds a package upgrade node.
	PackageUpgrade(packages ...string) *execution.Node

	// PackageRemove adds a package removal node.
	PackageRemove(packages ...string) *execution.Node

	// PackageUpdate adds a package index update node.
	PackageUpdate() *execution.Node

	// Template operations

	// Render adds a template rendering node.
	Render(source string) *execution.Node

	// Encryption operations

	// Decrypt adds a decryption node.
	Decrypt(source string) *execution.Node

	// File operations

	// Link adds a symlink creation node.
	Link(source, target string) *execution.Node

	// Copy adds a file copy node.
	Copy(source, target string) *execution.Node

	// Write adds a file write node (write content directly to target).
	Write(target, content string) *execution.Node

	// Remove adds a file/directory removal node.
	Remove(target string) *execution.Node

	// Download adds a file download node.
	Download(url, target string) *execution.Node

	// Archive operations

	// ArchiveExtract adds an archive extraction node.
	ArchiveExtract(archive, target string) *execution.Node

	// Git operations

	// GitClone adds a git clone node.
	GitClone(url, target string) *execution.Node

	// GitCheckout adds a git checkout node.
	GitCheckout(ref string) *execution.Node

	// GitPull adds a git pull node.
	GitPull() *execution.Node

	// System operations

	// Service adds a service management node.
	Service(name string, action ServiceAction) *execution.Node

	// Shell adds a shell command execution node.
	Shell(command string) *execution.Node

	// DependsOn creates a dependency edge between nodes.
	// The 'from' node will execute after the 'to' node completes.
	DependsOn(from, to *execution.Node)
}

// ServiceAction represents a service management action.
type ServiceAction int

const (
	// ServiceStart starts a service.
	ServiceStart ServiceAction = iota
	// ServiceStop stops a service.
	ServiceStop
	// ServiceRestart restarts a service.
	ServiceRestart
	// ServiceEnable enables a service at boot.
	ServiceEnable
	// ServiceDisable disables a service at boot.
	ServiceDisable
)

// String returns the action name.
func (a ServiceAction) String() string {
	switch a {
	case ServiceStart:
		return "start"
	case ServiceStop:
		return "stop"
	case ServiceRestart:
		return "restart"
	case ServiceEnable:
		return "enable"
	case ServiceDisable:
		return "disable"
	default:
		return "unknown"
	}
}
