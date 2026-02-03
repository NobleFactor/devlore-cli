// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// =============================================================================
// System Bindings Implementation
// =============================================================================

// systemBindings implements SystemBindings by wrapping host.Host.
type systemBindings struct {
	host host.Host
}

// NewSystemBindings creates a new SystemBindings from a host.Host.
func NewSystemBindings(h host.Host) SystemBindings {
	return &systemBindings{host: h}
}

// Platform returns information about the current system.
func (s *systemBindings) Platform() host.Platform {
	return s.host.Platform()
}

// Package returns package manager queries.
func (s *systemBindings) Package() PackageQueries {
	return &packageQueries{pm: s.host.PackageManager()}
}

// Service returns service manager queries.
func (s *systemBindings) Service() ServiceQueries {
	return &serviceQueries{sm: s.host.ServiceManager()}
}

// ToStarlark converts the system bindings to a Starlark value.
// Exposed to phase scripts as the fourth argument.
//
// All namespaces use the Attr receiver pattern for consistency and
// static analysis support. Starlark API:
//
//	system.platform                        # Platform info struct
//	system.package.installed(name)         # Check if package installed
//	system.package.version(name)           # Get package version
//	system.package.manager()               # Get package manager name
//	system.service.exists(name)            # Check if service exists
//	system.service.running(name)           # Check if service is running
//	system.service.enabled(name)           # Check if service is enabled
//	system.git.installed()                 # Check if git is installed
//	system.git.version()                   # Get git version
//	system.git.repo_root()                 # Get current repo root
//	system.git.current_branch()            # Get current branch
//	system.git.is_clean()                  # Check if working dir is clean
//	system.file.exists(path)               # Check if path exists
//	system.file.is_dir(path)               # Check if path is a directory
//	system.file.which(name)                # Find executable in PATH
//	system.file.home()                     # Get user's home directory
func (s *systemBindings) ToStarlark() starlark.Value {
	return NewSystemRoot(s.host)
}

// =============================================================================
// Package Queries Implementation
// =============================================================================

type packageQueries struct {
	pm host.PackageManager
}

func (p *packageQueries) Installed(name string) bool {
	return p.pm.Installed(name)
}

func (p *packageQueries) Version(name string) string {
	return p.pm.Version(name)
}

// =============================================================================
// Service Queries Implementation
// =============================================================================

type serviceQueries struct {
	sm host.ServiceManager
}

func (s *serviceQueries) Exists(name string) bool {
	return s.sm.Exists(name)
}

func (s *serviceQueries) Running(name string) bool {
	return s.sm.Status(name) == "running"
}

func (s *serviceQueries) Enabled(name string) bool {
	status := s.sm.Status(name)
	return status == "enabled" || status == "running"
}
