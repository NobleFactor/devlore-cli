// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"strings"
	"sync/atomic"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// nodeCounter provides unique node IDs across all plan bindings.
var nodeCounter uint64

// generateNodeID creates a unique node ID with the given prefix and components.
func generateNodeID(prefix string, components ...string) string {
	id := atomic.AddUint64(&nodeCounter, 1)
	if len(components) > 0 {
		return fmt.Sprintf("%s-%s-%d", prefix, strings.Join(components, "-"), id)
	}
	return fmt.Sprintf("%s-%d", prefix, id)
}

// =============================================================================
// Plan Bindings Implementation
// =============================================================================

// planBindings implements PlanBindings by building graph nodes.
type planBindings struct {
	graph   *execution.Graph
	host    host.Host
	project string // Package name for grouping
}

// NewPlanBindings creates a new PlanBindings for the given graph and host.
func NewPlanBindings(graph *execution.Graph, h host.Host, project string) PlanBindings {
	return &planBindings{
		graph:   graph,
		host:    h,
		project: project,
	}
}

// Graph returns the underlying execution graph.
func (p *planBindings) Graph() *execution.Graph {
	return p.graph
}

// Host returns the host abstraction.
func (p *planBindings) Host() host.Host {
	return p.host
}

// Project returns the project name for grouping nodes.
func (p *planBindings) Project() string {
	return p.project
}

// PackageInstall adds a package installation node.
func (p *planBindings) PackageInstall(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("package-install", packages...),
		Operations: []string{"package-install"},
		Project:    p.project,
	}
	node.SetSlotImmediate("packages", strings.Join(packages, ","))
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// PackageUpgrade adds a package upgrade node.
func (p *planBindings) PackageUpgrade(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("package-upgrade", packages...),
		Operations: []string{"package-upgrade"},
		Project:    p.project,
	}
	node.SetSlotImmediate("packages", strings.Join(packages, ","))
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// PackageRemove adds a package removal node.
func (p *planBindings) PackageRemove(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("package-remove", packages...),
		Operations: []string{"package-remove"},
		Project:    p.project,
	}
	node.SetSlotImmediate("packages", strings.Join(packages, ","))
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// PackageUpdate adds a package index update node.
func (p *planBindings) PackageUpdate() *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("package-update"),
		Operations: []string{"package-update"},
		Project:    p.project,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Configure adds a configuration file node (template expansion + copy).
func (p *planBindings) Configure(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("configure"),
		Operations: []string{"render", "copy"},
		Project:    p.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", p.host.ExpandPath(target))
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Link adds a symlink creation node.
func (p *planBindings) Link(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("link"),
		Operations: []string{"link"},
		Project:    p.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", p.host.ExpandPath(target))
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Copy adds a file copy node.
func (p *planBindings) Copy(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("copy"),
		Operations: []string{"copy"},
		Project:    p.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", p.host.ExpandPath(target))
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Write adds a file write node (write content directly to target).
func (p *planBindings) Write(target, content string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("write"),
		Operations: []string{"write"},
		Project:    p.project,
	}
	node.SetSlotImmediate("path", p.host.ExpandPath(target))
	node.SetSlotImmediate("content", content)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Remove adds a file/directory removal node.
func (p *planBindings) Remove(target string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("remove"),
		Operations: []string{"remove"},
		Project:    p.project,
	}
	node.SetSlotImmediate("path", p.host.ExpandPath(target))
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Download adds a file download node.
func (p *planBindings) Download(url, target string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("download"),
		Operations: []string{"download"},
		Project:    p.project,
	}
	node.SetSlotImmediate("url", url)
	node.SetSlotImmediate("path", p.host.ExpandPath(target))
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// ArchiveExtract adds an archive extraction node.
func (p *planBindings) ArchiveExtract(archive, target string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("archive-extract"),
		Operations: []string{"archive-extract"},
		Project:    p.project,
	}
	node.SetSlotImmediate("source", p.host.ExpandPath(archive))
	node.SetSlotImmediate("path", p.host.ExpandPath(target))
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// GitClone adds a git clone node.
func (p *planBindings) GitClone(url, target string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("git-clone"),
		Operations: []string{"git-clone"},
		Project:    p.project,
	}
	node.SetSlotImmediate("url", url)
	node.SetSlotImmediate("path", p.host.ExpandPath(target))
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// GitCheckout adds a git checkout node.
func (p *planBindings) GitCheckout(ref string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("git-checkout"),
		Operations: []string{"git-checkout"},
		Project:    p.project,
	}
	node.SetSlotImmediate("ref", ref)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// GitPull adds a git pull node.
func (p *planBindings) GitPull() *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("git-pull"),
		Operations: []string{"git-pull"},
		Project:    p.project,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Service adds a service management node.
func (p *planBindings) Service(name string, action ServiceAction) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("service", name, action.String()),
		Operations: []string{"service"},
		Project:    p.project,
	}
	node.SetSlotImmediate("name", name)
	node.SetSlotImmediate("action", action.String())
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Shell adds a shell command execution node.
func (p *planBindings) Shell(command string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("shell"),
		Operations: []string{"shell"},
		Project:    p.project,
	}
	node.SetSlotImmediate("command", command)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// DependsOn creates a dependency edge between nodes.
func (p *planBindings) DependsOn(from, to *execution.Node) {
	p.graph.Edges = append(p.graph.Edges, execution.Edge{
		From: to.ID,
		To:   from.ID,
	})
}

// =============================================================================
// Starlark Conversion
// =============================================================================

// StarlarkPlanBindings wraps PlanBindings for Starlark conversion.
type StarlarkPlanBindings struct {
	PlanBindings
}

// ToStarlark converts the plan bindings to a Starlark value.
// Exposed to phase scripts as the third argument.
//
// All namespaces use the Attr receiver pattern for consistency and
// static analysis support. Starlark API:
//
//	plan.package.install("pkg1", "pkg2", ...)  # Install packages
//	plan.package.upgrade("pkg1", ...)          # Upgrade packages
//	plan.package.remove("pkg1", ...)           # Remove packages
//	plan.package.update()                      # Update package index
//	plan.file.configure(source, target)        # Configure file (template + copy)
//	plan.file.link(source, target)             # Create symlink
//	plan.file.copy(source, target)             # Copy file
//	plan.file.write(target, content)           # Write content to file
//	plan.file.remove(target)                   # Remove file/directory
//	plan.archive.extract(archive, prefix)      # Extract archive
//	plan.git.clone(url, path)                  # Clone repository
//	plan.git.checkout(repo, ref)               # Checkout ref
//	plan.git.pull(repo)                        # Pull changes
//	plan.source(path)                          # Declare source file
//	plan.literal(content)                      # Inline content
//	plan.download(url)                         # Download file
//	plan.service(name, action)                 # Manage service
//	plan.shell(command)                        # Run shell command
//	plan.depends_on(consumer, producer)        # Create dependency
func (s *StarlarkPlanBindings) ToStarlark() starlark.Value {
	return NewPlanRoot(s.Graph(), s.Host(), s.Project())
}

// extractNodeID extracts the node ID from an argument that may be Output or struct.
func extractNodeID(arg starlark.Value, position string) (string, error) {
	// Check if it's an Output
	if output, ok := arg.(*Output); ok {
		return output.node.ID, nil
	}

	// Check if it's a struct with an id attribute
	if st, ok := arg.(*starlarkstruct.Struct); ok {
		idVal, err := st.Attr("id")
		if err != nil {
			return "", fmt.Errorf("%s argument has no id", position)
		}
		idStr, ok := starlark.AsString(idVal)
		if !ok {
			return "", fmt.Errorf("%s argument id is not a string", position)
		}
		return idStr, nil
	}

	return "", fmt.Errorf("%s argument must be an Output or node struct", position)
}

