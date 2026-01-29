// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package platform provides platform-specific plan bindings for the Starlark runtime.
//
// Each platform (darwin, linux, windows) provides specialized implementations
// that understand the native package manager, service manager, and file system
// conventions for that platform.
//
// Build Tags:
//   - darwin.go: macOS with Homebrew and launchd
//   - linux.go: Linux with runtime distro detection (apt, dnf, pacman)
//   - windows.go: Windows with winget
//
// The correct implementation is selected via Go build tags at compile time,
// with Linux using runtime detection to select the appropriate package manager.
package platform

import (
	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/engine"
	"github.com/NobleFactor/devlore-cli/internal/host"
	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
)

// PlatformPlanBindings extends PlanBindings with platform-specific operations.
// Each platform implementation may add additional methods beyond the base interface.
type PlatformPlanBindings interface {
	loreStar.PlanBindings

	// ToStarlark returns the Starlark representation of the plan bindings.
	ToStarlark() starlark.Value

	// PlatformName returns the platform identifier (e.g., "darwin", "linux", "windows").
	PlatformName() string

	// PackageManagerName returns the package manager name (e.g., "brew", "apt", "winget").
	PackageManagerName() string
}

// basePlanBindings provides common functionality for all platform implementations.
type basePlanBindings struct {
	graph   *engine.Graph
	host    host.Host
	project string
}

// Graph returns the underlying execution graph.
func (b *basePlanBindings) Graph() *engine.Graph {
	return b.graph
}

// newBasePlanBindings creates a new base plan bindings.
func newBasePlanBindings(graph *engine.Graph, h host.Host, project string) *basePlanBindings {
	return &basePlanBindings{
		graph:   graph,
		host:    h,
		project: project,
	}
}
