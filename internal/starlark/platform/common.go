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
	"fmt"
	"strings"
	"sync/atomic"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
)

// baseNodeCounter provides unique node IDs for base plan bindings.
var baseNodeCounter uint64

func baseGenerateNodeID(prefix string, components ...string) string {
	id := atomic.AddUint64(&baseNodeCounter, 1)
	if len(components) > 0 {
		return fmt.Sprintf("%s-%s-%d", prefix, strings.Join(components, "-"), id)
	}
	return fmt.Sprintf("%s-%d", prefix, id)
}

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
	graph   *execution.Graph
	host    host.Host
	project string
}

// Graph returns the underlying execution graph.
func (b *basePlanBindings) Graph() *execution.Graph {
	return b.graph
}

// newBasePlanBindings creates a new base plan bindings.
func newBasePlanBindings(graph *execution.Graph, h host.Host, project string) *basePlanBindings {
	return &basePlanBindings{
		graph:   graph,
		host:    h,
		project: project,
	}
}

// Remove adds a file/directory removal node.
func (b *basePlanBindings) Remove(target string) *execution.Node {
	node := &execution.Node{
		ID:         baseGenerateNodeID("remove"),
		Operations: []string{"file-remove"},
		Target:     b.host.ExpandPath(target),
		Project:    b.project,
	}
	b.graph.Nodes = append(b.graph.Nodes, node)
	return node
}

// Download adds a file download node.
func (b *basePlanBindings) Download(url, target string) *execution.Node {
	node := &execution.Node{
		ID:         baseGenerateNodeID("download"),
		Operations: []string{"download"},
		Source:     url,
		Target:     b.host.ExpandPath(target),
		Project:    b.project,
	}
	b.graph.Nodes = append(b.graph.Nodes, node)
	return node
}

// ArchiveExtract adds an archive extraction node.
func (b *basePlanBindings) ArchiveExtract(archive, target string) *execution.Node {
	node := &execution.Node{
		ID:         baseGenerateNodeID("archive-extract"),
		Operations: []string{"archive-extract"},
		Source:     b.host.ExpandPath(archive),
		Target:     b.host.ExpandPath(target),
		Project:    b.project,
	}
	b.graph.Nodes = append(b.graph.Nodes, node)
	return node
}

// GitClone adds a git clone node.
func (b *basePlanBindings) GitClone(url, target string) *execution.Node {
	node := &execution.Node{
		ID:         baseGenerateNodeID("git-clone"),
		Operations: []string{"git-clone"},
		Source:     url,
		Target:     b.host.ExpandPath(target),
		Project:    b.project,
	}
	b.graph.Nodes = append(b.graph.Nodes, node)
	return node
}

// GitCheckout adds a git checkout node.
func (b *basePlanBindings) GitCheckout(ref string) *execution.Node {
	node := &execution.Node{
		ID:         baseGenerateNodeID("git-checkout"),
		Operations: []string{"git-checkout"},
		Project:    b.project,
		Metadata: map[string]string{
			"ref": ref,
		},
	}
	b.graph.Nodes = append(b.graph.Nodes, node)
	return node
}

// GitPull adds a git pull node.
func (b *basePlanBindings) GitPull() *execution.Node {
	node := &execution.Node{
		ID:         baseGenerateNodeID("git-pull"),
		Operations: []string{"git-pull"},
		Project:    b.project,
	}
	b.graph.Nodes = append(b.graph.Nodes, node)
	return node
}
