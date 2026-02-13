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
func (p *Plan) Mkdir(path string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:        p.nextID("mkdir"),
		Operation: "mkdir",
		Project:   p.project,
		Mode:      0755,
	}
	node.SetSlotImmediate("path", path)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Link adds a symlink creation operation.
func (p *Plan) Link(source, path string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:        p.nextID("link"),
		Operation: "link",
		Project:   p.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", path)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Copy adds a file copy operation. Transforms (decrypt, render) create a chain
// of nodes connected by edges, with content flowing between them.
func (p *Plan) Copy(source, path string, transforms ...string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(transforms) == 0 {
		node := &Node{
			ID:        p.nextID("copy"),
			Operation: "copy",
			Project:   p.project,
			Mode:      0644,
		}
		node.SetSlotImmediate("source", source)
		node.SetSlotImmediate("path", path)
		p.graph.Nodes = append(p.graph.Nodes, node)
		return node
	}

	// Chain: transform1 → transform2 → ... → copy
	allOps := append(transforms, "copy")
	var prevNode *Node
	var lastNode *Node
	for i, op := range allOps {
		isLast := (i == len(allOps) - 1)
		node := &Node{
			ID:        p.nextID(op),
			Operation: op,
			Project:   p.project,
		}
		if i == 0 {
			node.SetSlotImmediate("source", source)
		}
		if isLast {
			node.SetSlotImmediate("path", path)
			node.Mode = 0644
		}
		p.graph.Nodes = append(p.graph.Nodes, node)
		if prevNode != nil {
			p.graph.Edges = append(p.graph.Edges, Edge{From: prevNode.ID, To: node.ID})
		}
		prevNode = node
		lastNode = node
	}
	return lastNode
}

// CopyWithMode adds a file copy operation with explicit permissions.
func (p *Plan) CopyWithMode(source, path string, mode os.FileMode, transforms ...string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(transforms) == 0 {
		node := &Node{
			ID:        p.nextID("copy"),
			Operation: "copy",
			Project:   p.project,
			Mode:      mode,
		}
		node.SetSlotImmediate("source", source)
		node.SetSlotImmediate("path", path)
		p.graph.Nodes = append(p.graph.Nodes, node)
		return node
	}

	// Chain: transform1 → transform2 → ... → copy
	allOps := append(transforms, "copy")
	var prevNode *Node
	var lastNode *Node
	for i, op := range allOps {
		isLast := (i == len(allOps) - 1)
		node := &Node{
			ID:        p.nextID(op),
			Operation: op,
			Project:   p.project,
		}
		if i == 0 {
			node.SetSlotImmediate("source", source)
		}
		if isLast {
			node.SetSlotImmediate("path", path)
			node.Mode = mode
		}
		p.graph.Nodes = append(p.graph.Nodes, node)
		if prevNode != nil {
			p.graph.Edges = append(p.graph.Edges, Edge{From: prevNode.ID, To: node.ID})
		}
		prevNode = node
		lastNode = node
	}
	return lastNode
}

// Remove adds a file/directory removal operation.
func (p *Plan) Remove(path string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:        p.nextID("remove"),
		Operation: "remove",
		Project:   p.project,
	}
	node.SetSlotImmediate("path", path)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Unlink adds a symlink removal operation.
func (p *Plan) Unlink(path string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:        p.nextID("unlink"),
		Operation: "unlink",
		Project:   p.project,
	}
	node.SetSlotImmediate("path", path)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Backup adds a backup operation for an existing file.
func (p *Plan) Backup(path string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:        p.nextID("backup"),
		Operation: "backup",
		Project:   p.project,
	}
	node.SetSlotImmediate("path", path)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Validate adds a precondition check operation.
func (p *Plan) Validate(check, message string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:        p.nextID("validate"),
		Operation: "validate",
		Project:   p.project,
	}
	node.SetSlotImmediate("check", check)
	node.SetSlotImmediate("message", message)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Rename adds a file/directory move operation (git mv when possible).
func (p *Plan) Rename(source, path string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:        p.nextID("move"),
		Operation: "move",
		Project:   p.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", path)
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
		From: from.ID,
		To:   to.ID,
	})
}

// Orders adds an ordering constraint between nodes.
func (p *Plan) Orders(from, to *Node) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.graph.Edges = append(p.graph.Edges, Edge{
		From: from.ID,
		To:   to.ID,
	})
}

// NOTE: There is no Delegates edge function. See comment above Delegate.
