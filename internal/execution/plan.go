// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"os"
	"sync"
)

// Plan provides binding functions for building an execution graph.
// Graph producers (writ tree builder, lore pipeline executor, LLM graph builder)
// use Plan to add operations to the graph. Each method returns the created node
// for edge construction.
//
// In Starlark scripts, the plan object is passed to each phase function:
//
//	def install(system, package, plan):
//	    plan.mkdir("/usr/local/bin")
//	    plan.link("/usr/local/bin/foo", source="/path/to/foo")
type Plan struct {
	mu      sync.Mutex
	graph   *Graph
	project string // default project for new nodes
	nodeID  int    // auto-incrementing node ID
}

// NewPlan creates a new plan for building an execution graph.
func NewPlan(project string) *Plan {
	return &Plan{
		graph:   &Graph{Nodes: []*Node{}, Edges: []Edge{}},
		project: project,
	}
}

// Graph returns the built execution graph.
func (p *Plan) Graph() *Graph {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.graph
}

// nextID generates a unique node ID.
func (p *Plan) nextID(prefix string) string {
	p.nodeID++
	return prefix + "-" + itoa(p.nodeID)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}

// Mkdir adds a directory creation operation.
func (p *Plan) Mkdir(target string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:         p.nextID("mkdir"),
		Operations: []string{"mkdir"},
		Target:     target,
		Project:    p.project,
		Mode:       0755,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Link adds a symlink creation operation.
func (p *Plan) Link(source, target string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:         p.nextID("link"),
		Operations: []string{"link"},
		Source:     source,
		Target:     target,
		Project:    p.project,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Copy adds a file copy operation. Transforms (decrypt, expand) can be
// prepended to the pipeline.
func (p *Plan) Copy(source, target string, transforms ...string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	ops := append(transforms, "copy")
	node := &Node{
		ID:         p.nextID("copy"),
		Operations: ops,
		Source:     source,
		Target:     target,
		Project:    p.project,
		Mode:       0644,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// CopyWithMode adds a file copy operation with explicit permissions.
func (p *Plan) CopyWithMode(source, target string, mode os.FileMode, transforms ...string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	ops := append(transforms, "copy")
	node := &Node{
		ID:         p.nextID("copy"),
		Operations: ops,
		Source:     source,
		Target:     target,
		Project:    p.project,
		Mode:       mode,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Remove adds a file/directory removal operation.
func (p *Plan) Remove(target string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:         p.nextID("remove"),
		Operations: []string{"remove"},
		Target:     target,
		Project:    p.project,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Unlink adds a symlink removal operation.
func (p *Plan) Unlink(target string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:         p.nextID("unlink"),
		Operations: []string{"unlink"},
		Target:     target,
		Project:    p.project,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Backup adds a backup operation for an existing file.
func (p *Plan) Backup(target string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:         p.nextID("backup"),
		Operations: []string{"backup"},
		Target:     target,
		Project:    p.project,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Validate adds a precondition check operation.
func (p *Plan) Validate(check, message string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:         p.nextID("validate"),
		Operations: []string{"validate"},
		Project:    p.project,
		Metadata: map[string]string{
			"check":   check,
			"message": message,
		},
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Rename adds a file/directory rename operation (git mv when possible).
func (p *Plan) Rename(source, target string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:         p.nextID("rename"),
		Operations: []string{"rename"},
		Source:     source,
		Target:     target,
		Project:    p.project,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// NOTE: There is no Delegate function. writ and lore share the same execution
// execution. When writ encounters a packages-manifest.yaml, the Package Graph
// Builder (internal/lore/graph) adds package installation nodes to the
// execution graph. There is no delegation or handoff between tools.
//
// The Package Graph Builder is NOT YET IMPLEMENTED.

// DependsOn adds an ordering edge: from must complete before to begins.
func (p *Plan) DependsOn(from, to *Node) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.graph.Edges = append(p.graph.Edges, Edge{
		From:     from.ID,
		To:       to.ID,
		Relation: "depends_on",
	})
}

// Orders adds an ordering constraint without implying dependency.
func (p *Plan) Orders(from, to *Node) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.graph.Edges = append(p.graph.Edges, Edge{
		From:     from.ID,
		To:       to.ID,
		Relation: "orders",
	})
}

// NOTE: There is no Delegates edge function. See comment above Delegate.
